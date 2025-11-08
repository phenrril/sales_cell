package domain

import (
	"time"

	"github.com/google/uuid"
)

type Customer struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	Email        string    `gorm:"size:140;uniqueIndex"`
	Name         string    `gorm:"size:140"`
	Phone        string    `gorm:"size:60"`
	TaxID        string    `gorm:"size:30"`
	TaxCondition string    `gorm:"size:4"` // RI, MT, EX, CF
	PriceList    string    `gorm:"size:40"`
	CreatedAt    time.Time
}
