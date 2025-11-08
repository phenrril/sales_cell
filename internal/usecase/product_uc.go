package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/phenrril/tienda3d/internal/domain"
)

type ProductUC struct {
	Products domain.ProductRepo
}

func (uc *ProductUC) List(ctx context.Context, f domain.ProductFilter) ([]domain.Product, int64, error) {
	if f.PageSize == 0 {
		f.PageSize = 20
	}
	return uc.Products.List(ctx, f)
}

func (uc *ProductUC) GetBySlug(ctx context.Context, slug string) (*domain.Product, error) {
	if slug == "" {
		return nil, errors.New("slug vacío")
	}
	return uc.Products.FindBySlug(ctx, slug)
}

func (uc *ProductUC) Create(ctx context.Context, p *domain.Product) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	p.Slug = strings.ToLower(strings.ReplaceAll(p.Name, " ", "-"))
	return uc.Products.Save(ctx, p)
}

func (uc *ProductUC) AddImages(ctx context.Context, productID uuid.UUID, imgs []domain.Image) error {
	return uc.Products.AddImages(ctx, productID, imgs)
}

func (uc *ProductUC) DeleteBySlug(ctx context.Context, slug string) error {
	if slug == "" {
		return errors.New("slug vacío")
	}

	if repo, ok := uc.Products.(interface {
		DeleteBySlug(context.Context, string) error
	}); ok {
		return repo.DeleteBySlug(ctx, slug)
	}
	return errors.New("repo no soporta delete")
}

func (uc *ProductUC) DeleteFullBySlug(ctx context.Context, slug string) ([]string, error) {
	if slug == "" {
		return nil, errors.New("slug vacío")
	}
	if repo, ok := uc.Products.(interface {
		DeleteFullBySlug(context.Context, string) ([]string, error)
	}); ok {
		return repo.DeleteFullBySlug(ctx, slug)
	}

	return nil, uc.DeleteBySlug(ctx, slug)
}

func (uc *ProductUC) Categories(ctx context.Context) ([]string, error) {
	if repo, ok := uc.Products.(interface {
		DistinctCategories(context.Context) ([]string, error)
	}); ok {
		return repo.DistinctCategories(ctx)
	}
	return []string{}, nil
}

// --- Variantes ---

func (uc *ProductUC) CreateVariant(ctx context.Context, v *domain.Variant) error {
	if v == nil {
		return errors.New("variant nil")
	}
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	return uc.Products.SaveVariant(ctx, v)
}

func (uc *ProductUC) UpdateVariant(ctx context.Context, v *domain.Variant) error {
	if v == nil || v.ID == uuid.Nil {
		return errors.New("variant id")
	}
	return uc.Products.SaveVariant(ctx, v)
}

func (uc *ProductUC) DeleteVariant(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return errors.New("variant id")
	}
	if repo, ok := uc.Products.(interface {
		DeleteVariant(context.Context, uuid.UUID) error
	}); ok {
		return repo.DeleteVariant(ctx, id)
	}
	// fallback: hard delete
	if repo, ok := uc.Products.(interface{ RawDB() any }); ok && repo != nil {
		// sin acceso directo al DB aquí; requerir método en repo concreto si fuera necesario
		return errors.New("repo no soporta DeleteVariant")
	}
	return errors.New("repo no soporta DeleteVariant")
}

func (uc *ProductUC) ListVariants(ctx context.Context, productID uuid.UUID) ([]domain.Variant, error) {
	if productID == uuid.Nil {
		return nil, errors.New("product id")
	}
	return uc.Products.ListVariants(ctx, productID)
}

func (uc *ProductUC) SearchByEAN(ctx context.Context, ean string) (*domain.Product, *domain.Variant, error) {
	e := strings.TrimSpace(ean)
	if e == "" {
		return nil, nil, errors.New("ean vacío")
	}
	return uc.Products.FindVariantByEAN(ctx, e)
}

func (uc *ProductUC) SearchBySKU(ctx context.Context, sku string) (*domain.Product, *domain.Variant, error) {
	s := strings.TrimSpace(sku)
	if s == "" {
		return nil, nil, errors.New("sku vacío")
	}
	return uc.Products.FindVariantBySKU(ctx, s)
}
