package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type ProductRepo interface {
	Save(ctx context.Context, p *Product) error
	FindBySlug(ctx context.Context, slug string) (*Product, error)
	List(ctx context.Context, filter ProductFilter) ([]Product, int64, error)
	AddImages(ctx context.Context, productID uuid.UUID, imgs []Image) error
	DistinctCategories(ctx context.Context) ([]string, error)
	// Variantes
	SaveVariant(ctx context.Context, v *Variant) error
	ListVariants(ctx context.Context, productID uuid.UUID) ([]Variant, error)
	FindVariantByEAN(ctx context.Context, ean string) (*Product, *Variant, error)
	FindVariantBySKU(ctx context.Context, sku string) (*Product, *Variant, error)
	UpdateVariantStock(ctx context.Context, variantID uuid.UUID, delta int) error
	DeleteVariant(ctx context.Context, variantID uuid.UUID) error
	// Imágenes
	ClearImages(ctx context.Context, productID uuid.UUID) ([]string, error)
	// Gestión de productos activos/inactivos
	MarkAllInactive(ctx context.Context) error
	GetInactiveSlugs(ctx context.Context) ([]string, error)
}

type CustomerRepo interface {
	FindByEmail(ctx context.Context, email string) (*Customer, error)
	Save(ctx context.Context, c *Customer) error
	// Opcionales
	FindByTaxID(ctx context.Context, taxID string) (*Customer, error)
}

type ProductFilter struct {
	Category        string
	ReadyToShip     *bool
	IncludeInactive *bool
	Sort            string
	Page            int
	PageSize        int
	Query           string
}

type OrderRepo interface {
	Save(ctx context.Context, o *Order) error
	FindByID(ctx context.Context, id uuid.UUID) (*Order, error)
	FindByPreferenceID(ctx context.Context, prefID string) (*Order, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, st OrderStatus) error
	List(ctx context.Context, status *OrderStatus, mpStatus *string, page, pageSize int) ([]Order, int64, error)
	ListInRange(ctx context.Context, from, to time.Time) ([]Order, error)
}

type QuoteRepo interface {
	Save(ctx context.Context, q *Quote) error
	FindByID(ctx context.Context, id uuid.UUID) (*Quote, error)
}

type UploadedModelRepo interface {
	Save(ctx context.Context, m *UploadedModel) error
	FindByID(ctx context.Context, id uuid.UUID) (*UploadedModel, error)
}

type PageRepo interface {
	FindBySlug(ctx context.Context, slug string) (*Page, error)
	Save(ctx context.Context, p *Page) error
}

type FeaturedProductRepo interface {
	List(ctx context.Context) ([]FeaturedProduct, error)
	Save(ctx context.Context, productID uuid.UUID, order int) error
	Delete(ctx context.Context, id uuid.UUID) error
	Clear(ctx context.Context) error
	GetWithProducts(ctx context.Context) ([]Product, error)
}

type QuoteService interface {
	EstimateFromModel(ctx context.Context, modelID uuid.UUID, cfg QuoteConfig) (*Quote, error)
}

type PricingService interface {
	Price(volumeCM3 float64, timeMin int, material Material, quality PrintQuality, infillPct int, layerMM float64) (float64, map[string]float64)
}

type PaymentGateway interface {
	CreatePreference(ctx context.Context, o *Order) (initPoint string, err error)
	VerifyWebhook(signature string, body []byte) (event interface{}, err error)
	PaymentInfo(ctx context.Context, paymentID string) (status string, externalRef string, err error)
}

type FileStorage interface {
	SaveModel(ctx context.Context, filename string, data []byte) (string, error)
	SaveImage(ctx context.Context, filename string, data []byte) (string, error)
}

type Clock interface{ Now() time.Time }

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }
