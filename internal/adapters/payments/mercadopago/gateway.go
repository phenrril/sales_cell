package mercadopago

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/phenrril/tienda3d/internal/domain"
)

type Gateway struct {
	token      string
	httpClient *http.Client
}

func NewGateway(token string) *Gateway {
	return &Gateway{token: token, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

type mpItem struct {
	Title      string  `json:"title"`
	Quantity   int     `json:"quantity"`
	UnitPrice  float64 `json:"unit_price"`
	CurrencyID string  `json:"currency_id"`
}

type mpPrefReq struct {
	Items               []mpItem          `json:"items"`
	Payer               map[string]string `json:"payer,omitempty"`
	BackURLs            map[string]string `json:"back_urls,omitempty"`
	AutoReturn          string            `json:"auto_return,omitempty"`
	NotificationURL     string            `json:"notification_url,omitempty"`
	StatementDescriptor string            `json:"statement_descriptor,omitempty"`
	Shipments           *struct {
		Cost float64 `json:"cost"`
		Mode string  `json:"mode"`
	} `json:"shipments,omitempty"`
}

type mpPrefResp struct {
	ID               string `json:"id"`
	InitPoint        string `json:"init_point"`
	SandboxInitPoint string `json:"sandbox_init_point"`
}

type mpPaymentResp struct {
	ID                int64  `json:"id"`
	Status            string `json:"status"`
	ExternalReference string `json:"external_reference"`
}

func signExternal(orderID string) string {
	key := os.Getenv("SECRET_KEY")
	if key == "" {
		key = "dev"
	}
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(orderID))
	return hex.EncodeToString(h.Sum(nil))[:24]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (g *Gateway) CreatePreference(ctx context.Context, o *domain.Order) (string, error) {
	if g.token == "" {
		return "", errors.New("MP token faltante (MP_ACCESS_TOKEN)")
	}
	if o == nil {
		return "", errors.New("orden nil")
	}

	// Validar formato básico del token
	if len(g.token) < 10 {
		return "", errors.New("token de MercadoPago inválido (muy corto)")
	}
	items := make([]mpItem, 0, len(o.Items)+1)
	subtotal := 0.0
	for _, it := range o.Items {
		items = append(items, mpItem{Title: it.Title, Quantity: it.Qty, UnitPrice: it.UnitPrice, CurrencyID: "ARS"})
		subtotal += it.UnitPrice * float64(it.Qty)
	}
	if o.ShippingCost > 0 {
		label := "Envío"
		if o.ShippingMethod == "cadete" {
			label = "Cadete (Rosario)"
		}
		items = append(items, mpItem{Title: label, Quantity: 1, UnitPrice: o.ShippingCost, CurrencyID: "ARS"})
	}
	// Agregar descuento como item negativo si existe
	if o.DiscountAmount > 0 {
		items = append(items, mpItem{
			Title:      "Descuento",
			Quantity:   1,
			UnitPrice:  -o.DiscountAmount,
			CurrencyID: "ARS",
		})
	}
	calcTotal := subtotal + o.ShippingCost - o.DiscountAmount
	// Usar el total de la orden si está bien calculado, sino usar el calculado
	if o.Total > 0 && (o.Total-calcTotal) <= 0.01 && (calcTotal-o.Total) <= 0.01 {
		// El total ya está correcto
	} else {
		o.Total = calcTotal
	}
	baseURL := os.Getenv("PUBLIC_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	extRef := fmt.Sprintf("%s|%s", o.ID.String(), signExternal(o.ID.String()))

	// MercadoPago con credenciales de PRODUCCIÓN rechaza localhost con auto_return
	// Si usamos token de producción con localhost, NO enviar auto_return
	useAutoReturn := true
	if !strings.HasPrefix(g.token, "TEST-") && strings.Contains(baseURL, "localhost") {
		useAutoReturn = false
	}

	autoReturnValue := ""
	if useAutoReturn {
		autoReturnValue = "approved"
	}

	reqBody := mpPrefReq{
		Items: items,
		Payer: map[string]string{"email": o.Email},
		BackURLs: map[string]string{
			"success": baseURL + "/pay/" + o.ID.String(),
			"pending": baseURL + "/pay/" + o.ID.String(),
			"failure": baseURL + "/pay/" + o.ID.String(),
		},
		AutoReturn:          autoReturnValue,
		NotificationURL:     baseURL + "/webhooks/mp",
		StatementDescriptor: "NEWMOBILE",
	}

	// Construir el payload manualmente para asegurar que todos los campos estén presentes
	type mpPreferenceRequest struct {
		Items               []mpItem          `json:"items"`
		Payer               map[string]string `json:"payer,omitempty"`
		BackURLs            map[string]string `json:"back_urls,omitempty"`
		AutoReturn          string            `json:"auto_return,omitempty"`
		NotificationURL     string            `json:"notification_url,omitempty"`
		StatementDescriptor string            `json:"statement_descriptor,omitempty"`
		ExternalReference   string            `json:"external_reference,omitempty"`
	}

	payload := mpPreferenceRequest{
		Items:               reqBody.Items,
		Payer:               reqBody.Payer,
		BackURLs:            reqBody.BackURLs,
		AutoReturn:          reqBody.AutoReturn,
		NotificationURL:     reqBody.NotificationURL,
		StatementDescriptor: reqBody.StatementDescriptor,
		ExternalReference:   extRef,
	}

	buf, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("error serializando payload MP: %w", err)
	}
	if os.Getenv("MP_DEBUG") == "1" {
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mercadopago.com/checkout/preferences", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+g.token)
	httpReq.Header.Set("Content-Type", "application/json")
	res, err := g.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("error de conexión con MercadoPago: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)

		// Intentar parsear el error de MercadoPago para un mensaje más claro
		var mpError struct {
			Message string `json:"message"`
			Status  int    `json:"status"`
			Code    string `json:"code"`
			Error   string `json:"error"`
		}
		if err := json.Unmarshal(body, &mpError); err == nil && mpError.Message != "" {

			// Mensajes más específicos según el código de error
			if res.StatusCode == 401 || res.StatusCode == 403 {
				return "", fmt.Errorf("credenciales de MercadoPago inválidas o sin permisos (status %d): %s. Verificá que MP_ACCESS_TOKEN sea válido y tenga permisos para crear preferencias", res.StatusCode, mpError.Message)
			}
			return "", fmt.Errorf("error de MercadoPago (status %d): %s", res.StatusCode, mpError.Message)
		}

		if res.StatusCode == 401 || res.StatusCode == 403 {
			return "", fmt.Errorf("credenciales de MercadoPago inválidas o sin permisos (status %d). Verificá que MP_ACCESS_TOKEN sea válido", res.StatusCode)
		}
		return "", fmt.Errorf("mp pref status %d: %s", res.StatusCode, string(body))
	}
	var pref mpPrefResp
	if err := json.NewDecoder(res.Body).Decode(&pref); err != nil {
		return "", err
	}
	if pref.ID == "" {
		return "", errors.New("respuesta MP incompleta")
	}
	initPoint := pref.InitPoint
	appEnv := strings.ToLower(os.Getenv("APP_ENV"))
	if strings.HasPrefix(g.token, "TEST-") && appEnv != "production" && appEnv != "prod" && pref.SandboxInitPoint != "" {
		initPoint = pref.SandboxInitPoint
	} else {
	}
	o.MPPreferenceID = pref.ID
	return initPoint, nil
}

func (g *Gateway) PaymentInfo(ctx context.Context, paymentID string) (string, string, error) {
	if g.token == "" || paymentID == "" {
		return "", "", errors.New("params")
	}
	url := "https://api.mercadopago.com/v1/payments/" + paymentID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	res, err := g.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		return "", "", fmt.Errorf("mp payment status %d: %s", res.StatusCode, string(b))
	}
	var pr mpPaymentResp
	if err := json.NewDecoder(res.Body).Decode(&pr); err != nil {
		return "", "", err
	}
	return pr.Status, pr.ExternalReference, nil
}

func (g *Gateway) VerifyWebhook(signature string, body []byte) (interface{}, error) {
	if signature == "" {
		return nil, errors.New("signature vacía")
	}
	return map[string]any{"status": "received", "len": len(body)}, nil
}

func VerifyExternalRef(ext string) (string, bool) {
	parts := strings.Split(ext, "|")
	if len(parts) != 2 {
		return "", false
	}
	orderID, sig := parts[0], parts[1]
	calc := signExternal(orderID)
	return orderID, calc == sig
}

func init() {

}
