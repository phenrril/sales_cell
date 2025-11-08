package domain

import (
	"time"

	"github.com/google/uuid"
)

type PrintQuality string

const (
	QualityDraft    PrintQuality = "draft"
	QualityStandard PrintQuality = "standard"
	QualityHigh     PrintQuality = "quality"
)

type Quote struct {
	ID              uuid.UUID    `gorm:"type:uuid;primaryKey"`
	UploadedModelID uuid.UUID    `gorm:"type:uuid;index"`
	Material        Material     `gorm:"type:varchar(10)"`
	LayerHeightMM   float64      `gorm:"type:decimal(4,2)"`
	InfillPct       int          `gorm:"type:int"`
	Quality         PrintQuality `gorm:"type:varchar(12)"`
	Price           float64      `gorm:"type:decimal(12,2)"`
	Currency        string       `gorm:"size:10"`
	ExpireAt        time.Time
	CreatedAt       time.Time
}

type QuoteConfig struct {
	Material      Material
	LayerHeightMM float64
	InfillPct     int
	Quality       PrintQuality
}
