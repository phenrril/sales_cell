package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/phenrril/tienda3d/internal/domain"
)

type OrderRepo struct{ db *gorm.DB }

func NewOrderRepo(db *gorm.DB) *OrderRepo { return &OrderRepo{db: db} }

func (r *OrderRepo) Save(ctx context.Context, o *domain.Order) error {

	if o == nil {
		return errors.New("order nil")
	}

	var count int64
	if err := r.db.WithContext(ctx).Model(&domain.Order{}).Where("id = ?", o.ID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {

		core := domain.Order{ID: o.ID, Status: o.Status, Email: o.Email, Name: o.Name, Phone: o.Phone, DNI: o.DNI, Address: o.Address, PostalCode: o.PostalCode, Province: o.Province, MPPreferenceID: o.MPPreferenceID, MPStatus: o.MPStatus, Total: o.Total, ShippingMethod: o.ShippingMethod, ShippingCost: o.ShippingCost, Notified: o.Notified}
		if err := r.db.WithContext(ctx).Create(&core).Error; err != nil {
			return err
		}

		if len(o.Items) > 0 {
			for i := range o.Items {
				o.Items[i].OrderID = o.ID
				if o.Items[i].ID == uuid.Nil {
					o.Items[i].ID = uuid.New()
				}
			}
			if err := r.db.WithContext(ctx).Create(&o.Items).Error; err != nil {
				return err
			}
		}
		return nil
	}

	return r.db.WithContext(ctx).Model(&domain.Order{}).Where("id = ?", o.ID).Updates(map[string]any{
		"status":           o.Status,
		"email":            o.Email,
		"name":             o.Name,
		"phone":            o.Phone,
		"dni":              o.DNI,
		"address":          o.Address,
		"postal_code":      o.PostalCode,
		"province":         o.Province,
		"mp_preference_id": o.MPPreferenceID,
		"mp_status":        o.MPStatus,
		"total":            o.Total,
		"shipping_method":  o.ShippingMethod,
		"shipping_cost":    o.ShippingCost,
		"notified":         o.Notified,
	}).Error
}

func (r *OrderRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	var o domain.Order
	if err := r.db.WithContext(ctx).Preload("Items").First(&o, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &o, nil
}

func (r *OrderRepo) FindByPreferenceID(ctx context.Context, prefID string) (*domain.Order, error) {
	var o domain.Order
	if err := r.db.WithContext(ctx).Preload("Items").First(&o, "mp_preference_id = ?", prefID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &o, nil
}

func (r *OrderRepo) UpdateStatus(ctx context.Context, id uuid.UUID, st domain.OrderStatus) error {
	return r.db.WithContext(ctx).Model(&domain.Order{}).Where("id = ?", id).Update("status", st).Error
}

func (r *OrderRepo) List(ctx context.Context, status *domain.OrderStatus, mpStatus *string, page, pageSize int) ([]domain.Order, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&domain.Order{})
	if status != nil {
		q = q.Where("status = ?", *status)
	}
	if mpStatus != nil && *mpStatus != "" {
		q = q.Where("mp_status = ?", *mpStatus)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []domain.Order
	if err := q.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Preload("Items").Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *OrderRepo) ListInRange(ctx context.Context, from, to time.Time) ([]domain.Order, error) {

	if to.Before(from) {
		from, to = to, from
	}

	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	to = time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), to.Location())
	var list []domain.Order
	if err := r.db.WithContext(ctx).Where("created_at BETWEEN ? AND ?", from, to).Order("created_at asc").Preload("Items").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
