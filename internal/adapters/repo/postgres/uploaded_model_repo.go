package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/phenrril/tienda3d/internal/domain"
)

type UploadedModelRepo struct{ db *gorm.DB }

func NewUploadedModelRepo(db *gorm.DB) *UploadedModelRepo { return &UploadedModelRepo{db: db} }

func (r *UploadedModelRepo) Save(ctx context.Context, m *domain.UploadedModel) error {
	return r.db.WithContext(ctx).Save(m).Error
}

func (r *UploadedModelRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.UploadedModel, error) {
	var m domain.UploadedModel
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}
