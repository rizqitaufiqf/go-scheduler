package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/rizqitaufiqf/go-scheduler/dto"
	repo "github.com/rizqitaufiqf/go-scheduler/repository"
)

type ProductHandler struct {
	repo repo.ProductRepository
}

func NewProductHandler(repo repo.ProductRepository) *ProductHandler {
	return &ProductHandler{repo: repo}
}

// CreateProduct handles direct creation of a product.
// @Summary      Create a new product directly
// @Description  Adds a new product to the database immediately.
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        product  body      dto.Product  true  "Product to create"
// @Success      201      {object}  dto.Product
// @Failure      400      {object}  dto.ErrorResponse
// @Failure      500      {object}  dto.ErrorResponse
// @Router       /products [post]
func (h *ProductHandler) CreateProduct(c *gin.Context) {
	var product dto.Product
	if err := c.ShouldBindJSON(&product); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.repo.Create(&product); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to create product"})
		return
	}

	c.JSON(http.StatusCreated, product)
}

// GetProducts lists all products.
// @Summary      List all products
// @Description  Gets a list of all products in the database.
// @Tags         Products
// @Produce      json
// @Success      200  {array}   dto.Product
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /products [get]
func (h *ProductHandler) GetProducts(c *gin.Context) {
	products, err := h.repo.FindAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to retrieve products"})
		return
	}
	c.JSON(http.StatusOK, products)
}

// UpdateProduct handles direct update of a product.
// @Summary      Update an existing product
// @Description  Updates a product's details by its UUID.
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        id       path      string       true  "Product ID (UUID)" Format(uuid)
// @Param        product  body      dto.Product  true  "Product data to update"
// @Success      200      {object}  dto.MessageResponse
// @Failure      400      {object}  dto.ErrorResponse
// @Failure      404      {object}  dto.ErrorResponse
// @Failure      500      {object}  dto.ErrorResponse
// @Router       /products/{id} [put]
func (h *ProductHandler) UpdateProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid product ID"})
		return
	}

	var product dto.Product
	if err := c.ShouldBindJSON(&product); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	product.ID = id
	if err := h.repo.Update(&product); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to update product"})
		return
	}

	c.JSON(http.StatusOK, dto.MessageResponse{Message: "Product updated successfully"})
}

// DeleteProduct handles direct deletion of a product.
// @Summary      Delete a product
// @Description  Deletes a product by its UUID.
// @Tags         Products
// @Produce      json
// @Param        id  path      string  true  "Product ID (UUID)" Format(uuid)
// @Success      204 "No Content"
// @Failure      400 {object}  dto.ErrorResponse
// @Failure      500 {object}  dto.ErrorResponse
// @Router       /products/{id} [delete]
func (h *ProductHandler) DeleteProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid product ID"})
		return
	}

	if err := h.repo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to delete product"})
		return
	}

	c.Status(http.StatusNoContent)
}
