package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/phenrril/tienda3d/internal/domain"
)

type StarProductRepo struct{ db *gorm.DB }

func NewStarProductRepo(db *gorm.DB) *StarProductRepo {
	return &StarProductRepo{db: db}
}

// Get devuelve el producto estrella actual con sus imágenes precargadas.
// Devuelve (nil, nil) si no hay ninguno o si el producto referenciado fue eliminado/inactivado.
func (r *StarProductRepo) Get(ctx context.Context) (*domain.Product, error) {
	var sp domain.StarProduct
	if err := r.db.WithContext(ctx).First(&sp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var p domain.Product
	err := r.db.WithContext(ctx).
		Where("id = ? AND active = ?", sp.ProductID, true).
		Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at asc")
		}).
		First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// Set reemplaza el producto estrella por el indicado. Garantiza singleton.
func (r *StarProductRepo) Set(ctx context.Context, productID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&domain.StarProduct{}).Error; err != nil {
			return err
		}
		sp := domain.StarProduct{
			ID:        uuid.New(),
			ProductID: productID,
			UpdatedAt: time.Now(),
		}
		return tx.Create(&sp).Error
	})
}

// Clear elimina el producto estrella si existiese.
func (r *StarProductRepo) Clear(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("1 = 1").Delete(&domain.StarProduct{}).Error
}
