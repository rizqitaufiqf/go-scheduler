package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/rizqitaufiqf/go-scheduler/dto"
	"github.com/rizqitaufiqf/go-scheduler/repository"
)

// ProductProcessor handles task processing for the Product domain.
type ProductProcessor struct {
	productRepo repository.ProductRepository
}

// NewProductProcessor creates a new task processor for products.
func NewProductProcessor(productRepo repository.ProductRepository) *ProductProcessor {
	return &ProductProcessor{
		productRepo: productRepo,
	}
}

// ProcessProductCreate handles product creation tasks
func (p *ProductProcessor) ProcessProductCreate(ctx context.Context, t *asynq.Task) error {
	var payload dto.ProductCreatePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	log.Printf("[ProductCreate] Processing: %+v", payload)
	// Simulate some work
	time.Sleep(1 * time.Second)

	// Example: Save to database, call external API, etc.
	product := &dto.Product{
		ID:    uuid.New(), // Generate a new UUID for the product
		Name:  payload.Name,
		Price: payload.Price,
		Stock: payload.Stock,
	}

	if err := p.productRepo.Create(product); err != nil {
		return fmt.Errorf("failed to create product in db: %w", err)
	}

	log.Printf("[ProductCreate] Successfully created product with ID: %s", product.ID)

	return nil
}

// ProcessProductUpdate handles product update tasks
func (p *ProductProcessor) ProcessProductUpdate(ctx context.Context, t *asynq.Task) error {
	var payload dto.ProductUpdatePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	log.Printf("[ProductUpdate] Processing: %+v", payload)
	// Simulate some work
	time.Sleep(1 * time.Second)

	productID, err := uuid.Parse(payload.ID)
	if err != nil {
		return fmt.Errorf("invalid UUID format in payload: %w", err)
	}

	product := &dto.Product{
		ID:    productID,
		Name:  payload.Name,
		Price: payload.Price,
	}

	if err := p.productRepo.Update(product); err != nil {
		return fmt.Errorf("failed to update product in db: %w", err)
	}

	log.Printf("[ProductUpdate] Successfully updated product with ID: %s", payload.ID)

	return nil
}

// ProcessProductDelete handles product deletion tasks
func (p *ProductProcessor) ProcessProductDelete(ctx context.Context, t *asynq.Task) error {
	var payload dto.ProductDeletePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	log.Printf("[ProductDelete] Processing: %+v", payload)

	productID, err := uuid.Parse(payload.ID)
	if err != nil {
		return fmt.Errorf("invalid UUID format in payload: %w", err)
	}

	log.Printf("[ProductDelete] Deleted product ID: %v", productID)

	return p.productRepo.Delete(productID)
}
