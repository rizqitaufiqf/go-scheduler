package dto

import (
	"time"

	"github.com/google/uuid"
)

// Product represents a product in the database.
type Product struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;" json:"id"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	Price     float64   `json:"price"`
	Stock     int       `json:"stock"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProductCreatePayload for product creation tasks
type ProductCreatePayload struct {
	Name        string  `json:"name"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
	Description string  `json:"description,omitempty"`
}

// ProductUpdatePayload for product update tasks
type ProductUpdatePayload struct {
	ID          string  `json:"id"`
	Name        string  `json:"name,omitempty"`
	Price       float64 `json:"price,omitempty"`
	Stock       int     `json:"stock,omitempty"`
	Description string  `json:"description,omitempty"`
}

// ProductDeletePayload for product deletion tasks
type ProductDeletePayload struct {
	ID string `json:"id"`
}
