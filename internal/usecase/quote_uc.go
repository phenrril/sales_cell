package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/phenrril/tienda3d/internal/domain"
)

type QuoteUC struct {
	Models  domain.UploadedModelRepo
	Quotes  domain.QuoteRepo
	Pricing domain.PricingService
	Clock   domain.Clock
}

func (uc *QuoteUC) CreateFromModel(ctx context.Context, model *domain.UploadedModel, cfg domain.QuoteConfig) (*domain.Quote, error) {
	if model.ID == uuid.Nil {
		return nil, errors.New("model sin ID")
	}
	price, _ := uc.Pricing.Price(model.VolumeCM3, model.EstimatedTimeMin, cfg.Material, cfg.Quality, cfg.InfillPct, cfg.LayerHeightMM)
	q := &domain.Quote{
		ID:              uuid.New(),
		UploadedModelID: model.ID,
		Material:        cfg.Material,
		LayerHeightMM:   cfg.LayerHeightMM,
		InfillPct:       cfg.InfillPct,
		Quality:         cfg.Quality,
		Price:           price,
		Currency:        "ARS",
		ExpireAt:        uc.Clock.Now().Add(24 * time.Hour),
		CreatedAt:       uc.Clock.Now(),
	}
	if err := uc.Quotes.Save(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

func (uc *QuoteUC) Reprice(ctx context.Context, quoteID uuid.UUID, cfg domain.QuoteConfig) (*domain.Quote, error) {
	q, err := uc.Quotes.FindByID(ctx, quoteID)
	if err != nil {
		return nil, err
	}
	model, err := uc.Models.FindByID(ctx, q.UploadedModelID)
	if err != nil {
		return nil, err
	}
	price, _ := uc.Pricing.Price(model.VolumeCM3, model.EstimatedTimeMin, cfg.Material, cfg.Quality, cfg.InfillPct, cfg.LayerHeightMM)
	q.Material = cfg.Material
	q.LayerHeightMM = cfg.LayerHeightMM
	q.InfillPct = cfg.InfillPct
	q.Quality = cfg.Quality
	q.Price = price
	q.ExpireAt = uc.Clock.Now().Add(24 * time.Hour)
	if err := uc.Quotes.Save(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}
