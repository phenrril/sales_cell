package smtp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/rs/zerolog/log"
	"gopkg.in/gomail.v2"

	"github.com/phenrril/tienda3d/internal/domain"
)

type SMTPService struct {
	host     string
	port     int
	user     string
	password string
	from     string
	enabled  bool
}

func NewSMTPService() *SMTPService {
	host := os.Getenv("SMTP_HOST")
	portStr := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	password := os.Getenv("SMTP_PASS")
	from := os.Getenv("SMTP_FROM")

	// Valores por defecto
	if host == "" {
		host = "smtp.gmail.com"
	}
	if portStr == "" {
		portStr = "587"
	}
	if from == "" {
		from = user
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		port = 587
	}

	enabled := host != "" && user != "" && password != ""

	// Logging detallado para debug
	if enabled {
		log.Info().
			Str("host", host).
			Int("port", port).
			Str("user", user).
			Str("from", from).
			Msg("✅ SMTP configurado correctamente")
	} else {
		log.Warn().
			Str("host", host).
			Str("user", user).
			Bool("has_password", password != "").
			Msg("⚠️ SMTP no configurado - los emails de confirmación no se enviarán")
	}

	return &SMTPService{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		from:     from,
		enabled:  enabled,
	}
}

func (s *SMTPService) SendOrderConfirmation(ctx context.Context, order *domain.Order) error {
	if !s.enabled {
		log.Warn().Str("order_id", order.ID.String()).Msg("⚠️ SMTP no configurado - no se envió email para orden")
		return nil
	}

	if order == nil {
		return fmt.Errorf("orden es nil")
	}

	if order.Email == "" {
		log.Warn().Str("order_id", order.ID.String()).Msg("⚠️ Email vacío - no se envió email de confirmación")
		return nil
	}

	// Generar HTML
	html, err := s.generateOrderHTML(order)
	if err != nil {
		log.Error().Err(err).Str("order_id", order.ID.String()).Msg("error generando HTML del email")
		return err
	}

	// Crear mensaje
	orderNumber := order.ID.String()[:8]
	subject := fmt.Sprintf("✅ Confirmación de tu pedido #%s", orderNumber)

	m := gomail.NewMessage()
	m.SetHeader("From", s.from)
	m.SetHeader("To", order.Email)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", html)

	// Enviar
	d := gomail.NewDialer(s.host, s.port, s.user, s.password)
	
	// Para puerto 465 (Gmail SSL), habilitar SSL
	if s.port == 465 {
		d.SSL = true
	}
	// Para puerto 587, gomail usa STARTTLS automáticamente
	
	if err := d.DialAndSend(m); err != nil {
		log.Error().
			Err(err).
			Str("order_id", order.ID.String()).
			Str("email", order.Email).
			Str("host", s.host).
			Int("port", s.port).
			Str("from", s.from).
			Msg("❌ Error enviando email de confirmación")
		return err
	}

	log.Info().
		Str("order_id", order.ID.String()).
		Str("email", order.Email).
		Msg("📧 Email de confirmación enviado")
	return nil
}

type ItemData struct {
	Title    string
	Color    string
	Qty      int
	Subtotal float64
}

type EmailData struct {
	Name           string
	OrderNumber    string
	PaymentMethod  string
	Items          []ItemData
	ShippingCost   float64
	DiscountAmount float64
	Total          float64
	Address        string
	PostalCode     string
	Province       string
}

func (s *SMTPService) generateOrderHTML(order *domain.Order) (string, error) {
	// Preparar datos
	orderNumber := order.ID.String()[:8]
	paymentMethod := s.mapPaymentMethod(order.PaymentMethod)

	items := make([]ItemData, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, ItemData{
			Title:    item.Title,
			Color:    item.Color,
			Qty:      item.Qty,
			Subtotal: item.UnitPrice * float64(item.Qty),
		})
	}

	data := EmailData{
		Name:           order.Name,
		OrderNumber:    orderNumber,
		PaymentMethod:  paymentMethod,
		Items:          items,
		ShippingCost:   order.ShippingCost,
		DiscountAmount: order.DiscountAmount,
		Total:          order.Total,
		Address:        order.Address,
		PostalCode:     order.PostalCode,
		Province:       order.Province,
	}

	// Template HTML
	tmpl := `<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Confirmación de Pedido</title>
</head>
<body style="margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; background-color: #f3f4f6;">
    <table role="presentation" style="width: 100%; border-collapse: collapse; background-color: #f3f4f6; padding: 20px 0;">
        <tr>
            <td align="center">
                <table role="presentation" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); overflow: hidden;">
                    <!-- Header con gradiente -->
                    <tr>
                        <td style="background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); padding: 40px 30px; text-align: center;">
                            <h1 style="margin: 0; color: #ffffff; font-size: 28px; font-weight: 600;">¡Gracias por tu compra!</h1>
                        </td>
                    </tr>
                    
                    <!-- Contenido principal -->
                    <tr>
                        <td style="padding: 30px;">
                            <!-- Saludo -->
                            <p style="margin: 0 0 20px 0; color: #111827; font-size: 16px; line-height: 1.6;">
                                Hola <strong>{{.Name}}</strong>,
                            </p>
                            <p style="margin: 0 0 30px 0; color: #374151; font-size: 16px; line-height: 1.6;">
                                Hemos recibido tu pedido y estamos procesándolo. A continuación encontrarás todos los detalles.
                            </p>
                            
                            <!-- Información del pedido -->
                            <table role="presentation" style="width: 100%; border-collapse: collapse; background-color: #f9fafb; border-radius: 6px; padding: 20px; margin-bottom: 30px;">
                                <tr>
                                    <td style="padding: 0;">
                                        <p style="margin: 0 0 8px 0; color: #6b7280; font-size: 14px; font-weight: 500; text-transform: uppercase; letter-spacing: 0.5px;">Número de Pedido</p>
                                        <p style="margin: 0 0 16px 0; color: #111827; font-size: 20px; font-weight: 600;">#{{.OrderNumber}}</p>
                                        <p style="margin: 0; color: #6b7280; font-size: 14px; font-weight: 500; text-transform: uppercase; letter-spacing: 0.5px;">Método de Pago</p>
                                        <p style="margin: 0; color: #111827; font-size: 16px; font-weight: 500;">{{.PaymentMethod}}</p>
                                    </td>
                                </tr>
                            </table>
                            
                            <!-- Tabla de productos -->
                            <h2 style="margin: 0 0 20px 0; color: #111827; font-size: 20px; font-weight: 600;">Productos</h2>
                            <table role="presentation" style="width: 100%; border-collapse: collapse; margin-bottom: 30px;">
                                <thead>
                                    <tr style="background-color: #f9fafb; border-bottom: 2px solid #e5e7eb;">
                                        <th style="padding: 12px; text-align: left; color: #374151; font-size: 14px; font-weight: 600;">Producto</th>
                                        <th style="padding: 12px; text-align: center; color: #374151; font-size: 14px; font-weight: 600;">Cantidad</th>
                                        <th style="padding: 12px; text-align: right; color: #374151; font-size: 14px; font-weight: 600;">Precio</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {{range .Items}}
                                    <tr style="border-bottom: 1px solid #e5e7eb;">
                                        <td style="padding: 16px 12px;">
                                            <p style="margin: 0 0 4px 0; color: #111827; font-size: 15px; font-weight: 500;">{{.Title}}</p>
                                            {{if .Color}}
                                            <p style="margin: 0; color: #6b7280; font-size: 13px;">Color: {{.Color}}</p>
                                            {{end}}
                                        </td>
                                        <td style="padding: 16px 12px; text-align: center; color: #374151; font-size: 15px;">{{.Qty}}</td>
                                        <td style="padding: 16px 12px; text-align: right; color: #111827; font-size: 15px; font-weight: 500;">${{printf "%.2f" .Subtotal}}</td>
                                    </tr>
                                    {{end}}
                                </tbody>
                            </table>
                            
                            <!-- Totales -->
                            <table role="presentation" style="width: 100%; border-collapse: collapse; margin-bottom: 30px;">
                                <tr>
                                    <td style="padding: 8px 0; text-align: right; color: #6b7280; font-size: 15px;">Subtotal:</td>
                                    <td style="padding: 8px 0; padding-left: 20px; text-align: right; color: #111827; font-size: 15px; width: 120px;">${{printf "%.2f" (subtotal .Items .ShippingCost)}}</td>
                                </tr>
                                {{if gt .ShippingCost 0.0}}
                                <tr>
                                    <td style="padding: 8px 0; text-align: right; color: #6b7280; font-size: 15px;">Envío:</td>
                                    <td style="padding: 8px 0; padding-left: 20px; text-align: right; color: #111827; font-size: 15px;">${{printf "%.2f" .ShippingCost}}</td>
                                </tr>
                                {{end}}
                                {{if gt .DiscountAmount 0.0}}
                                <tr>
                                    <td style="padding: 8px 0; text-align: right; color: #059669; font-size: 15px; font-weight: 500;">Descuento:</td>
                                    <td style="padding: 8px 0; padding-left: 20px; text-align: right; color: #059669; font-size: 15px; font-weight: 500;">-${{printf "%.2f" .DiscountAmount}}</td>
                                </tr>
                                {{end}}
                                <tr>
                                    <td style="padding: 12px 0; text-align: right; color: #111827; font-size: 18px; font-weight: 600; border-top: 2px solid #e5e7eb;">Total:</td>
                                    <td style="padding: 12px 0; padding-left: 20px; text-align: right; color: #667eea; font-size: 18px; font-weight: 600; border-top: 2px solid #e5e7eb;">${{printf "%.2f" .Total}}</td>
                                </tr>
                            </table>
                            
                            {{if .Address}}
                            <!-- Dirección de envío -->
                            <table role="presentation" style="width: 100%; border-collapse: collapse; background-color: #f9fafb; border-radius: 6px; padding: 20px; margin-bottom: 30px;">
                                <tr>
                                    <td style="padding: 0;">
                                        <p style="margin: 0 0 8px 0; color: #6b7280; font-size: 14px; font-weight: 500; text-transform: uppercase; letter-spacing: 0.5px;">Dirección de Envío</p>
                                        <p style="margin: 0; color: #111827; font-size: 15px; line-height: 1.6;">
                                            {{.Address}}<br>
                                            {{if .PostalCode}}{{.PostalCode}}{{end}}{{if and .PostalCode .Province}}, {{end}}{{if .Province}}{{.Province}}{{end}}
                                        </p>
                                    </td>
                                </tr>
                            </table>
                            {{end}}
                            
                            <!-- Próximos pasos -->
                            <table role="presentation" style="width: 100%; border-collapse: collapse; border: 2px solid #667eea; border-radius: 6px; padding: 20px; margin-bottom: 30px; background-color: #f0f4ff;">
                                <tr>
                                    <td style="padding: 0;">
                                        <h3 style="margin: 0 0 12px 0; color: #667eea; font-size: 18px; font-weight: 600;">Próximos pasos</h3>
                                        <ul style="margin: 0; padding-left: 20px; color: #374151; font-size: 15px; line-height: 1.8;">
                                            <li>Recibirás una notificación cuando tu pedido sea procesado</li>
                                            <li>Te contactaremos si necesitamos información adicional</li>
                                            <li>Puedes consultar el estado de tu pedido en cualquier momento</li>
                                        </ul>
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>
                    
                    <!-- Footer -->
                    <tr>
                        <td style="padding: 30px; background-color: #f9fafb; text-align: center; border-top: 1px solid #e5e7eb;">
                            <p style="margin: 0 0 8px 0; color: #6b7280; font-size: 14px;">Gracias por confiar en nosotros</p>
                            <p style="margin: 0; color: #9ca3af; font-size: 12px;">Si tienes alguna pregunta, no dudes en contactarnos</p>
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`

	// Funciones del template
	funcMap := template.FuncMap{
		"subtotal": func(items []ItemData, shipping float64) float64 {
			total := 0.0
			for _, item := range items {
				total += item.Subtotal
			}
			// El subtotal no incluye el shipping
			return total
		},
	}

	t, err := template.New("email").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("error parseando template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error ejecutando template: %w", err)
	}

	return buf.String(), nil
}

func (s *SMTPService) mapPaymentMethod(method string) string {
	method = strings.ToLower(strings.TrimSpace(method))
	switch method {
	case "mercadopago":
		return "MercadoPago"
	case "efectivo":
		return "Efectivo"
	case "transferencia", "transfer":
		return "Transferencia Bancaria"
	default:
		if method == "" {
			return "No especificado"
		}
		// Capitalizar primera letra
		if len(method) > 0 {
			return strings.ToUpper(method[:1]) + method[1:]
		}
		return method
	}
}

