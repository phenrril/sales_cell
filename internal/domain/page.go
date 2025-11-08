package domain

import (
	"time"
)

type Page struct {
	Slug      string `gorm:"primaryKey;size:80"`
	Title     string `gorm:"size:180"`
	BodyMD    string `gorm:"type:text"`
	UpdatedAt time.Time
}
