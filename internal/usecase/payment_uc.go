package usecase

import (
	"context"

	"github.com/phenrril/tienda3d/internal/domain"
)

type PaymentUC struct {
	Orders  domain.OrderRepo
	Gateway domain.PaymentGateway
}

func (uc *PaymentUC) CreatePreference(ctx context.Context, order *domain.Order) (string, error) {
	url, err := uc.Gateway.CreatePreference(ctx, order)
	if err != nil {
		return "", err
	}
	return url, nil
}
