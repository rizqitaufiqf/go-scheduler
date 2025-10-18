package dto

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents the user model in the database.
type User struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	Username  string         `gorm:"type:varchar(100);unique;not null" json:"username"`
	Email     string         `gorm:"type:varchar(255);unique;not null" json:"email"`
	FullName  string         `gorm:"type:varchar(255)" json:"full_name,omitempty"`
	IsActive  bool           `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time      `gorm:"default:now()" json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at,omitempty"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"` // Use json:"-" to hide from API responses
}

// TableName specifies the table name for the User model.
func (User) TableName() string {
	return "public.users"
}
