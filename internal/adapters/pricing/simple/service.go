package simple

import "github.com/phenrril/tienda3d/internal/domain"

type Service struct{}

func NewPricingService() *Service { return &Service{} }

func (s *Service) Price(volumeCM3 float64, timeMin int, material domain.Material, quality domain.PrintQuality, infillPct int, layerMM float64) (float64, map[string]float64) {
	coefMaterial := map[domain.Material]float64{domain.MaterialPLA: 2.2, domain.MaterialPETG: 2.8, domain.MaterialTPU: 3.1}[material]
	if coefMaterial == 0 {
		coefMaterial = 2.5
	}
	rateQuality := map[domain.PrintQuality]float64{domain.QualityDraft: 12, domain.QualityStandard: 18, domain.QualityHigh: 25}[quality]
	if rateQuality == 0 {
		rateQuality = 18
	}
	layerFactor := 0.0
	if layerMM > 0 {
		layerFactor = (0.28 - layerMM) * 10
	}
	baseVol := volumeCM3 * coefMaterial
	baseTime := float64(timeMin) * rateQuality / 60.0
	infillFactor := 1.0 + float64(infillPct)/200.0
	raw := (baseVol + baseTime) * infillFactor
	adj := raw + layerFactor
	margin := adj * 0.20
	price := adj + margin
	bd := map[string]float64{
		"volume":        baseVol,
		"time":          baseTime,
		"infill_factor": infillFactor,
		"layer_adj":     layerFactor,
		"margin":        margin,
		"total":         price,
	}
	return price, bd
}
