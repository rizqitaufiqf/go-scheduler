package worker

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rizqitaufiqf/go-scheduler/dto"
	"github.com/rizqitaufiqf/go-scheduler/repository"
)

// --- Concrete Task Processor Implementations for PRODUCT ---

// ProductCreateProcessor handles the creation of a product.
type ProductCreateProcessor struct {
	repo repository.ProductRepository
}

// Process unmarshals the payload and calls the repository to create a product.
func (p *ProductCreateProcessor) Process(task *dto.TaskScheduler) error {
	var product dto.Product
	if err := json.Unmarshal(task.Payload, &product); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	product.ID = uuid.New() // Ensure a new UUID is generated for creation
	time.Sleep(20 * time.Second)
	return p.repo.Create(&product)
}

// ProductUpdateProcessor handles the update of a product.
type ProductUpdateProcessor struct {
	repo repository.ProductRepository
}

// Process unmarshals the payload and calls the repository to update a product.
func (p *ProductUpdateProcessor) Process(task *dto.TaskScheduler) error {
	var product dto.Product
	if err := json.Unmarshal(task.Payload, &product); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	return p.repo.Update(&product)
}

// ProductDeleteProcessor handles the deletion of a product.
type ProductDeleteProcessor struct {
	repo repository.ProductRepository
}

// Process unmarshals the payload and calls the repository to delete a product.
func (p *ProductDeleteProcessor) Process(task *dto.TaskScheduler) error {
	var payload struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	return p.repo.Delete(payload.ID)
}
