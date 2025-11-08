package usecase

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/phenrril/tienda3d/internal/domain"
)

type OrderUC struct {
	Orders   domain.OrderRepo
	Quotes   domain.QuoteRepo
	Products domain.ProductRepo
	Clock    domain.Clock
}

func (uc *OrderUC) CreateFromQuote(ctx context.Context, quote *domain.Quote, email string) (*domain.Order, error) {
	if quote == nil {
		return nil, errors.New("quote nil")
	}
	o := &domain.Order{
		ID:     uuid.New(),
		Status: domain.OrderStatusQuoted,
		Email:  email,
		Items:  []domain.OrderItem{{ID: uuid.New(), QuoteID: &quote.ID, Qty: 1, UnitPrice: quote.Price}},
		Total:  quote.Price,
	}
	if err := uc.Orders.Save(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

func (uc *OrderUC) UpdateStatus(ctx context.Context, id uuid.UUID, st domain.OrderStatus) error {
	return uc.Orders.UpdateStatus(ctx, id, st)
}
