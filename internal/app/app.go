package app

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gorm.io/gorm"

	"github.com/phenrril/tienda3d/internal/adapters/httpserver"
	"github.com/phenrril/tienda3d/internal/adapters/payments/mercadopago"
	"github.com/phenrril/tienda3d/internal/adapters/repo/postgres"
	"github.com/phenrril/tienda3d/internal/adapters/storage/localfs"
	"github.com/phenrril/tienda3d/internal/domain"
	"github.com/phenrril/tienda3d/internal/usecase"
	"github.com/phenrril/tienda3d/internal/views"
)

type App struct {
	DB             *gorm.DB
	Tmpl           *template.Template
	ProductUC      *usecase.ProductUC
	QuoteUC        *usecase.QuoteUC
	OrderUC        *usecase.OrderUC
	PaymentUC      *usecase.PaymentUC
	ModelRepo      domain.UploadedModelRepo
	ShippingMethod string  `gorm:"size:30"`
	ShippingCost   float64 `gorm:"type:decimal(12,2)"`
	Storage        domain.FileStorage
	Customers      domain.CustomerRepo
	OAuthConfig    *oauth2.Config
}

func NewApp(db *gorm.DB) (*App, error) {

	prodRepo := postgres.NewProductRepo(db)
	orderRepo := postgres.NewOrderRepo(db)
	modelRepo := postgres.NewUploadedModelRepo(db)
	custRepo := postgres.NewCustomerRepo(db)
	storageDir := os.Getenv("STORAGE_DIR")
	if storageDir == "" {
		storageDir = "uploads"
	}
	_ = os.MkdirAll(storageDir, 0755)
	storage := localfs.New(storageDir)

	token := os.Getenv("MP_ACCESS_TOKEN")
	appEnv := strings.ToLower(os.Getenv("APP_ENV"))
	if appEnv == "production" || appEnv == "prod" {
		if prodTok := os.Getenv("PROD_ACCESS_TOKEN"); prodTok != "" {
			log.Info().Msg("usando credenciales MP producción")
			token = prodTok
		} else {
			log.Warn().Msg("APP_ENV=production pero falta PROD_ACCESS_TOKEN, usando MP_ACCESS_TOKEN")
		}
	} else {
		if strings.HasPrefix(token, "TEST-") {
			log.Info().Msg("modo sandbox MercadoPago (token TEST-)")
		} else {
			log.Info().Msg("APP_ENV no es production; usando token definido")
		}
	}
	
	// Validar que el token esté presente
	if token == "" {
		log.Error().Msg("MP_ACCESS_TOKEN no está configurado. Las preferencias de pago no funcionarán.")
	} else {
		// Log del prefijo del token para debugging (sin exponer el token completo)
		tokenPrefix := token
		if len(tokenPrefix) > 10 {
			tokenPrefix = tokenPrefix[:10] + "..."
		}
		log.Info().Str("token_prefix", tokenPrefix).Msg("token de MercadoPago configurado")
	}

	payment := mercadopago.NewGateway(token)

	var oauthCfg *oauth2.Config
	googleID := os.Getenv("GOOGLE_CLIENT_ID")
	googleSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	if googleID != "" && googleSecret != "" {
		oauthCfg = &oauth2.Config{
			ClientID:     googleID,
			ClientSecret: googleSecret,
			RedirectURL:  baseURL + "/auth/google/callback",
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		}
		log.Info().Msg("OAuth Google habilitado")
	} else {
		log.Warn().Msg("Google OAuth no configurado (faltan GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET)")
	}

	app := &App{}
	app.ProductUC = &usecase.ProductUC{Products: prodRepo}
	app.OrderUC = &usecase.OrderUC{Orders: orderRepo, Products: prodRepo}
	app.PaymentUC = &usecase.PaymentUC{Orders: orderRepo, Gateway: payment}
	app.DB = db
	app.ModelRepo = modelRepo
	app.Storage = storage
	app.Customers = custRepo
	app.OAuthConfig = oauthCfg

	// helpers de plantillas (UI)
	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"div": func(a, b float64) float64 { return a / b },
		"mul": func(a, b float64) float64 { return a * b },
		// ars: formatea un número como ARS sin decimales con separadores de miles
		"ars": func(v float64) string {
			// redondeo a entero
			s := fmt.Sprintf("%.0f", v)
			// insertar separador de miles '.'
			n := len(s)
			neg := false
			if n > 0 && s[0] == '-' {
				neg = true
				s = s[1:]
				n--
			}
			if n <= 3 {
				if neg {
					return "ARS -" + s
				}
				return "ARS " + s
			}
			rem := n % 3
			if rem == 0 {
				rem = 3
			}
			out := s[:rem]
			for i := rem; i < n; i += 3 {
				out += "." + s[i:i+3]
			}
			if neg {
				out = "-" + out
			}
			return "ARS " + out
		},
		// percent aplica un porcentaje a un valor
		"percent": func(v float64, pct float64) float64 { return v * (1.0 + pct/100.0) },
		// gain: calcula ganancia bruta segun precio bruto y margen
		"gain": func(gross float64, pct float64) float64 { return gross * (pct / 100.0) },
		// colorhex: convierte un nombre genérico (es) o cualquier string a un hex de swatch
		"colorhex": func(s string) string {
			v := strings.TrimSpace(strings.ToLower(s))
			if v == "" {
				return "#334155"
			}
			if strings.HasPrefix(v, "#") {
				return v
			}
			m := map[string]string{
				"negro":       "#111827",
				"blanco":      "#ffffff",
				"azul":        "#3b82f6",
				"verde":       "#10b981",
				"amarillo":    "#f59e0b",
				"rojo":        "#ef4444",
				"violeta":     "#6366f1",
				"lila":        "#8b5cf6",
				"rosa":        "#ec4899",
				"turquesa":    "#14b8a6",
				"lima":        "#a3e635",
				"gris":        "#64748b",
				"gris oscuro": "#334155",
			}
			if hex, ok := m[v]; ok {
				return hex
			}
			return "#334155"
		},
		// img: normaliza URLs de imagen (agrega / si falta y codifica espacios)
		"img": func(u string) string {
			s := strings.TrimSpace(u)
			if s == "" {
				return s
			}
			if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "/") {
				s = "/" + s
			}
			// codificar espacios para atributos src/srcset
			s = strings.ReplaceAll(s, " ", "%20")
			return s
		},
		// imgw: agrega parámetro ?w= para variantes responsivas
		"imgw": func(u string, w int) string {
			base := strings.TrimSpace(u)
			if base == "" {
				return base
			}
			if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") && !strings.HasPrefix(base, "/") {
				base = "/" + base
			}
			base = strings.ReplaceAll(base, " ", "%20")
			return fmt.Sprintf("%s?w=%d", base, w)
		},
	}
	
	// En desarrollo, cargar templates desde filesystem para hot reload
	isDev := appEnv == "" || appEnv == "development" || appEnv == "dev"
	
	var tmpl *template.Template
	var err error
	
	if isDev {
		// Desarrollo: cargar desde filesystem para ver cambios sin rebuild
		tmpl, err = template.New("layout").Funcs(funcMap).ParseGlob("internal/views/*.html")
		if err != nil {
			return nil, err
		}
		_, err = tmpl.ParseGlob("internal/views/admin/*.html")
		if err != nil {
			return nil, err
		}
	} else {
		// Producción: usar archivos embebidos
		tmpl, err = template.New("layout").Funcs(funcMap).ParseFS(views.FS, "*.html", "admin/*.html")
		if err != nil {
			return nil, err
		}
	}
	
	app.Tmpl = tmpl

	return app, nil
}

func (a *App) HTTPHandler() http.Handler {
	return httpserver.New(a.Tmpl, a.ProductUC, a.QuoteUC, a.OrderUC, a.PaymentUC, a.ModelRepo, a.Storage, a.Customers, a.OAuthConfig)
}

func (a *App) MigrateAndSeed() error {
	if err := a.DB.AutoMigrate(
		&domain.Product{}, &domain.Variant{}, &domain.Image{}, &domain.Order{}, &domain.OrderItem{}, &domain.UploadedModel{}, &domain.Quote{}, &domain.Page{}, &domain.Customer{},
	); err != nil {
		return err
	}

	// Migraciones manuales para columnas que AutoMigrate puede no agregar si la tabla ya existe
	// Tabla orders: agregar columnas para checkout multi-paso
	_ = a.DB.Exec("ALTER TABLE orders ADD COLUMN IF NOT EXISTS payment_method VARCHAR(30)").Error
	_ = a.DB.Exec("ALTER TABLE orders ADD COLUMN IF NOT EXISTS discount_amount DECIMAL(12,2) DEFAULT 0").Error
	_ = a.DB.Exec("ALTER TABLE orders ADD COLUMN IF NOT EXISTS customer_id UUID").Error
	_ = a.DB.Exec("ALTER TABLE orders ADD COLUMN IF NOT EXISTS shipping_method VARCHAR(30)").Error
	_ = a.DB.Exec("ALTER TABLE orders ADD COLUMN IF NOT EXISTS shipping_cost DECIMAL(12,2) DEFAULT 0").Error
	_ = a.DB.Exec("ALTER TABLE orders ADD COLUMN IF NOT EXISTS subtotal_net DECIMAL(12,2) DEFAULT 0").Error
	_ = a.DB.Exec("ALTER TABLE orders ADD COLUMN IF NOT EXISTS vat_amount DECIMAL(12,2) DEFAULT 0").Error
	
	// Índices para las nuevas columnas
	_ = a.DB.Exec("CREATE INDEX IF NOT EXISTS idx_orders_payment_method ON orders(payment_method)").Error
	_ = a.DB.Exec("CREATE INDEX IF NOT EXISTS idx_orders_customer_id ON orders(customer_id)").Error
	
	// Tabla order_items: agregar columnas para variantes e impuestos
	_ = a.DB.Exec("ALTER TABLE order_items ADD COLUMN IF NOT EXISTS variant_id UUID").Error
	_ = a.DB.Exec("ALTER TABLE order_items ADD COLUMN IF NOT EXISTS sku VARCHAR(120)").Error
	_ = a.DB.Exec("ALTER TABLE order_items ADD COLUMN IF NOT EXISTS ean VARCHAR(20)").Error
	_ = a.DB.Exec("ALTER TABLE order_items ADD COLUMN IF NOT EXISTS unit_price_net DECIMAL(12,2) DEFAULT 0").Error
	_ = a.DB.Exec("ALTER TABLE order_items ADD COLUMN IF NOT EXISTS vat_rate DECIMAL(5,2) DEFAULT 21.00").Error
	_ = a.DB.Exec("ALTER TABLE order_items ADD COLUMN IF NOT EXISTS vat_amount DECIMAL(12,2) DEFAULT 0").Error
	_ = a.DB.Exec("ALTER TABLE order_items ADD COLUMN IF NOT EXISTS unit_price_gross DECIMAL(12,2) DEFAULT 0").Error
	
	// Índices para order_items
	_ = a.DB.Exec("CREATE INDEX IF NOT EXISTS idx_order_items_variant_id ON order_items(variant_id)").Error
	
	// Tabla customers: agregar columnas para datos fiscales
	_ = a.DB.Exec("ALTER TABLE customers ADD COLUMN IF NOT EXISTS tax_id VARCHAR(30)").Error
	_ = a.DB.Exec("ALTER TABLE customers ADD COLUMN IF NOT EXISTS tax_condition VARCHAR(4)").Error
	_ = a.DB.Exec("ALTER TABLE customers ADD COLUMN IF NOT EXISTS price_list VARCHAR(40)").Error

	if err := backfillSlugs(a.DB); err != nil {
		return err
	}

	// Índices adicionales y constraints sugeridos
	_ = a.DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_variants_sku_unique ON variants (sku) WHERE sku IS NOT NULL AND sku <> ''").Error
	_ = a.DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_variants_ean_unique ON variants (ean) WHERE ean IS NOT NULL AND ean <> ''").Error
	_ = a.DB.Exec("CREATE INDEX IF NOT EXISTS idx_variants_product_id ON variants (product_id)").Error
	_ = a.DB.Exec("CREATE INDEX IF NOT EXISTS idx_variants_attributes_gin ON variants USING gin (attributes)").Error

	return nil
}

func backfillSlugs(db *gorm.DB) error {
	var products []domain.Product
	if err := db.Where("slug IS NULL OR slug = ''").Find(&products).Error; err != nil {
		return err
	}
	for _, p := range products {
		base := strings.ToLower(strings.TrimSpace(p.Name))
		base = strings.ReplaceAll(base, " ", "-")
		if base == "" {
			base = p.ID.String()[:8]
		}
		slug := base

		var count int64
		i := 1
		for {
			if err := db.Model(&domain.Product{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				break
			}
			i++
			slug = fmt.Sprintf("%s-%d", base, i)
		}
		if err := db.Model(&domain.Product{}).Where("id = ?", p.ID).Update("slug", slug).Error; err != nil {
			return err
		}
	}

	if err := db.Exec("UPDATE products SET name = 'Producto' WHERE name IS NULL OR name = ''").Error; err != nil {
		return err
	}
	if err := db.Exec("UPDATE products SET base_price = 0 WHERE base_price IS NULL").Error; err != nil {
		return err
	}

	_ = db.Exec("ALTER TABLE products ALTER COLUMN slug SET NOT NULL").Error
	_ = db.Exec("ALTER TABLE products ALTER COLUMN name SET NOT NULL").Error
	_ = db.Exec("ALTER TABLE products ALTER COLUMN base_price SET NOT NULL").Error
	return nil
}

func seedProducts(db *gorm.DB) {
	prods := []domain.Product{
		{ID: uuid.New(), Slug: "llavero-logo", Name: "Llavero Logo", BasePrice: 1200, Category: "accesorios", ShortDesc: "Llavero impreso", ReadyToShip: true},
		{ID: uuid.New(), Slug: "lampara-luna", Name: "Lámpara Luna", BasePrice: 8500, Category: "iluminacion", ShortDesc: "Lámpara decorativa"},
		{ID: uuid.New(), Slug: "soporte-celular", Name: "Soporte Celular", BasePrice: 2500, Category: "hogar", ShortDesc: "Stand universal"},
		{ID: uuid.New(), Slug: "organizador-cables", Name: "Organizador Cables", BasePrice: 1800, Category: "oficina", ShortDesc: "Ordená tus cables"},
		{ID: uuid.New(), Slug: "maceta-geometrica", Name: "Maceta Geométrica", BasePrice: 3000, Category: "jardin"},
		{ID: uuid.New(), Slug: "clip-bolsa", Name: "Clip Bolsa", BasePrice: 600, Category: "cocina", ReadyToShip: true},
	}
	for _, p := range prods {
		db.Create(&p)
	}
}

func seedPages(db *gorm.DB) {
	pages := []domain.Page{{Slug: "about", Title: "Sobre NewMobile", BodyMD: "Somos una tienda especializada en celulares y accesorios."}, {Slug: "contact", Title: "Contacto", BodyMD: "Escribinos a ventas@newmobile.com.ar"}}
	for _, p := range pages {
		db.Create(&p)
	}
}
