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
	"github.com/rs/zerolog/log"
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

func (g *Gateway) CreatePreference(ctx context.Context, o *domain.Order) (string, error) {
	if g.token == "" {
		return "", errors.New("MP token faltante (MP_ACCESS_TOKEN)")
	}
	if o == nil {
		return "", errors.New("orden nil")
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
	calcTotal := subtotal + o.ShippingCost
	if o.Total == 0 || (o.Total-calcTotal) > 0.01 || (calcTotal-o.Total) > 0.01 {
		o.Total = calcTotal
	}
	log.Debug().Str("order", o.ID.String()).Float64("subtotal", subtotal).Float64("shipping", o.ShippingCost).Float64("total", o.Total).Int("items", len(items)).Msg("MP preference build (shipping as item)")
	baseURL := os.Getenv("PUBLIC_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	extRef := fmt.Sprintf("%s|%s", o.ID.String(), signExternal(o.ID.String()))
	reqBody := mpPrefReq{
		Items: items,
		Payer: map[string]string{"email": o.Email},
		BackURLs: map[string]string{
			"success": baseURL + "/pay/" + o.ID.String(),
			"pending": baseURL + "/pay/" + o.ID.String(),
			"failure": baseURL + "/pay/" + o.ID.String(),
		},
		AutoReturn:          "approved",
		NotificationURL:     baseURL + "/webhooks/mp",
		StatementDescriptor: "CHROMA3D",
	}

	type reqExt struct {
		mpPrefReq
		ExternalReference string `json:"external_reference"`
	}
	payload := reqExt{mpPrefReq: reqBody, ExternalReference: extRef}
	buf, _ := json.Marshal(payload)
	if os.Getenv("MP_DEBUG") == "1" {
		log.Debug().RawJSON("mp_pref_payload", buf).Msg("MP preference payload")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mercadopago.com/checkout/preferences", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+g.token)
	httpReq.Header.Set("Content-Type", "application/json")
	res, err := g.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
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
		log.Debug().Str("pref_id", pref.ID).Str("url", initPoint).Msg("MP preference sandbox")
	} else {
		log.Info().Str("pref_id", pref.ID).Str("url", initPoint).Msg("MP preference prod")
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
