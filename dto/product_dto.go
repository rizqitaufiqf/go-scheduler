package dto

import (
	"time"

	"github.com/google/uuid"
)

// Product represents the products table
type Product struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name      string    `gorm:"type:varchar(255);not null" json:"name"`
	Price     float64   `gorm:"type:numeric(10,2);not null" json:"price"`
	Stock     int       `gorm:"not null" json:"stock"`
	CreatedAt time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:now()" json:"updated_at"`
}
