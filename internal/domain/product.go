package domain

import (
	"time"

	"github.com/google/uuid"
)

type Product struct {
	ID             uuid.UUID         `gorm:"type:uuid;primaryKey"`
	Slug           string            `gorm:"uniqueIndex;size:140"`
	Name           string            `gorm:"size:180"`
	BasePrice      float64           `gorm:"type:decimal(12,2)"`
	GrossPrice     float64           `gorm:"type:decimal(12,2);default:0"`
	MarginPct      float64           `gorm:"type:decimal(6,2);default:0"`
	Category       string            `gorm:"size:100"`
	ShortDesc      string            `gorm:"type:text"`
	ReadyToShip    bool              `gorm:"default:true"`
	Active         bool              `gorm:"default:true;index"`
	WidthMM        float64           `gorm:"type:decimal(8,2);default:0"`
	HeightMM       float64           `gorm:"type:decimal(8,2);default:0"`
	DepthMM        float64           `gorm:"type:decimal(8,2);default:0"`
	Brand          string            `gorm:"size:100"`
	Model          string            `gorm:"size:140"`
	Attributes     map[string]string `gorm:"type:jsonb;serializer:json"`
	Specifications map[string]string `gorm:"type:jsonb;serializer:json"`
	Images         []Image
	Variants       []Variant
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Variant struct {
	ID            uuid.UUID         `gorm:"type:uuid;primaryKey"`
	ProductID     uuid.UUID         `gorm:"type:uuid;index"`
	Material      Material          `gorm:"type:varchar(10);not null"`
	Color         string            `gorm:"size:60"`
	LayerHeightMM float64           `gorm:"type:decimal(4,2)"`
	InfillPct     int               `gorm:"type:int"`
	SKU           string            `gorm:"size:100;index"`
	EAN           string            `gorm:"size:20;index"`
	Attributes    map[string]string `gorm:"type:jsonb;serializer:json"`
	Price         float64           `gorm:"type:decimal(12,2);default:0"`
	Cost          float64           `gorm:"type:decimal(12,2);default:0"`
	Stock         int               `gorm:"type:int;default:0"`
	ImageURL      string            `gorm:"size:255"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Image struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	ProductID uuid.UUID `gorm:"type:uuid;index"`
	URL       string    `gorm:"size:255"`
	Alt       string    `gorm:"size:140"`
	CreatedAt time.Time
}
