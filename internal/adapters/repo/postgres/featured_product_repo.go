package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/phenrril/tienda3d/internal/domain"
)

type FeaturedProductRepo struct{ db *gorm.DB }

func NewFeaturedProductRepo(db *gorm.DB) *FeaturedProductRepo {
	return &FeaturedProductRepo{db: db}
}

// List devuelve todos los productos destacados ordenados por DisplayOrder
func (r *FeaturedProductRepo) List(ctx context.Context) ([]domain.FeaturedProduct, error) {
	var list []domain.FeaturedProduct
	if err := r.db.WithContext(ctx).Order("display_order asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// Save guarda o actualiza un producto destacado
func (r *FeaturedProductRepo) Save(ctx context.Context, productID uuid.UUID, order int) error {
	fp := domain.FeaturedProduct{
		ID:           uuid.New(),
		ProductID:    productID,
		DisplayOrder: order,
		CreatedAt:    time.Now(),
	}
	
	// Usar Upsert: si ya existe el ProductID, actualizar el DisplayOrder
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Primero verificar si ya existe
		var existing domain.FeaturedProduct
		err := tx.Where("product_id = ?", productID).First(&existing).Error
		
		if err == nil {
			// Ya existe, actualizar DisplayOrder
			return tx.Model(&existing).Update("display_order", order).Error
		} else if err == gorm.ErrRecordNotFound {
			// No existe, crear nuevo
			return tx.Create(&fp).Error
		}
		return err
	})
}

// Delete elimina un producto destacado por su ID
func (r *FeaturedProductRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&domain.FeaturedProduct{}, "id = ?", id).Error
}

// Clear elimina todos los productos destacados
func (r *FeaturedProductRepo) Clear(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("1 = 1").Delete(&domain.FeaturedProduct{}).Error
}

// GetWithProducts devuelve los productos completos de los productos destacados, ordenados por DisplayOrder
// Solo devuelve productos activos
func (r *FeaturedProductRepo) GetWithProducts(ctx context.Context) ([]domain.Product, error) {
	var products []domain.Product
	
	err := r.db.WithContext(ctx).
		Table("products").
		Select("products.*").
		Joins("INNER JOIN featured_products ON products.id = featured_products.product_id").
		Where("products.active = ?", true).
		Order("featured_products.display_order asc").
		Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at asc")
		}).
		Find(&products).Error
	
	if err != nil {
		return nil, err
	}
	
	return products, nil
}

