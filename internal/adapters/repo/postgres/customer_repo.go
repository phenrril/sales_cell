package postgres

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/phenrril/tienda3d/internal/domain"
)

type CustomerRepo struct{ db *gorm.DB }

func NewCustomerRepo(db *gorm.DB) *CustomerRepo { return &CustomerRepo{db: db} }

func (r *CustomerRepo) FindByEmail(ctx context.Context, email string) (*domain.Customer, error) {
	var c domain.Customer
	e := strings.ToLower(strings.TrimSpace(email))
	if e == "" {
		return nil, errors.New("email vacío")
	}
	if err := r.db.WithContext(ctx).First(&c, "LOWER(email) = ?", e).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *CustomerRepo) Save(ctx context.Context, c *domain.Customer) error {
	if c.Email != "" {
		c.Email = strings.ToLower(c.Email)
	}
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *CustomerRepo) FindByTaxID(ctx context.Context, taxID string) (*domain.Customer, error) {
	var c domain.Customer
	t := strings.TrimSpace(taxID)
	if t == "" {
		return nil, errors.New("tax id vacío")
	}
	if err := r.db.WithContext(ctx).First(&c, "tax_id = ?", t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}
