package domain

import (
	"time"

	"github.com/google/uuid"
)

type UploadedModel struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey"`
	OwnerEmail       string    `gorm:"size:140"`
	Filename         string    `gorm:"size:255"`
	Path             string    `gorm:"size:400"`
	VolumeCM3        float64   `gorm:"type:decimal(10,3)"`
	EstimatedTimeMin int       `gorm:"type:int"`
	Hash             string    `gorm:"size:120;index"`
	CreatedAt        time.Time
}
