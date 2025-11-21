package domain

import (
	"time"

	"github.com/google/uuid"
)

type OrderStatus string

const (
	OrderStatusPendingQuote OrderStatus = "pending_quote"
	OrderStatusQuoted       OrderStatus = "quoted"
	OrderStatusAwaitingPay  OrderStatus = "awaiting_payment"
	OrderStatusInPrint      OrderStatus = "in_print"
	OrderStatusFinished     OrderStatus = "finished"
	OrderStatusShipped      OrderStatus = "shipped"
	OrderStatusCancelled    OrderStatus = "cancelled"
)

type Order struct {
	ID             uuid.UUID   `gorm:"type:uuid;primaryKey"`
	Status         OrderStatus `gorm:"type:varchar(30);index"`
	Items          []OrderItem
	Email          string     `gorm:"size:140"`
	Name           string     `gorm:"size:140"`
	Phone          string     `gorm:"size:50"`
	DNI            string     `gorm:"size:30"`
	Address        string     `gorm:"size:255"`
	PostalCode     string     `gorm:"size:20"`
	Province       string     `gorm:"size:80"`
	DeliveryNotes  string     `gorm:"type:text"`
	MPPreferenceID string     `gorm:"size:140"`
	MPStatus       string     `gorm:"size:60"`
	CustomerID     *uuid.UUID `gorm:"type:uuid;index"`
	SubtotalNet    float64    `gorm:"type:decimal(12,2);default:0"`
	VATAmount      float64    `gorm:"type:decimal(12,2);default:0"`
	Total          float64    `gorm:"type:decimal(12,2)"`
	ShippingMethod string     `gorm:"size:30"`
	ShippingCost   float64    `gorm:"type:decimal(12,2)"`
	PaymentMethod  string     `gorm:"size:30;index"`
	DiscountAmount float64    `gorm:"type:decimal(12,2)"`
	Notified       bool       `gorm:"not null;default:false"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

type OrderItem struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey"`
	OrderID        uuid.UUID  `gorm:"type:uuid;index"`
	ProductID      *uuid.UUID `gorm:"type:uuid;index"`
	VariantID      *uuid.UUID `gorm:"type:uuid;index"`
	QuoteID        *uuid.UUID `gorm:"type:uuid;index"`
	Title          string     `gorm:"size:180"`
	Color          string     `gorm:"size:60"`
	Qty            int        `gorm:"not null"`
	SKU            string     `gorm:"size:120"`
	EAN            string     `gorm:"size:20"`
	UnitPrice      float64    `gorm:"type:decimal(12,2)"`
	UnitPriceNet   float64    `gorm:"type:decimal(12,2);default:0"`
	VATRate        float64    `gorm:"type:decimal(5,2);default:21.00"`
	VATAmount      float64    `gorm:"type:decimal(12,2);default:0"`
	UnitPriceGross float64    `gorm:"type:decimal(12,2);default:0"`
}
