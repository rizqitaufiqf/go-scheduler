package repository

import (
	"fmt"

	dto "github.com/rizqitaufiqf/go-scheduler/dto"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProductRepository defines the interface for product database operations.
type ProductRepository interface {
	Create(product *dto.Product) error
	Update(product *dto.Product) error
	Delete(id uuid.UUID) error
	FindAll() ([]dto.Product, error)
}

type productRepository struct {
	db *gorm.DB
}

// NewProductRepository creates a new instance of ProductRepository.
func NewProductRepository(db *gorm.DB) ProductRepository {
	return &productRepository{db: db}
}

// Create adds a new product to the database.
func (r *productRepository) Create(product *dto.Product) error {
	return r.db.Create(product).Error
}

// FindAll retrieves all products from the database.
func (r *productRepository) FindAll() ([]dto.Product, error) {
	var products []dto.Product
	err := r.db.Find(&products).Error
	return products, err
}

// Update modifies an existing product in the database.
func (r *productRepository) Update(product *dto.Product) error {
	if product.ID == uuid.Nil {
		return fmt.Errorf("product ID is missing for update")
	}

	tx := r.db.Model(&dto.Product{}).Where("id = ?", product.ID).Updates(product)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Delete removes a product from the database by its ID.
func (r *productRepository) Delete(id uuid.UUID) error {
	if id == uuid.Nil {
		return fmt.Errorf("product ID is missing for delete")
	}

	tx := r.db.Delete(&dto.Product{}, id)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
