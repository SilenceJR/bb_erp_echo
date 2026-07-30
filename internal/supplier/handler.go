// Package supplier 负责采购供应商档案。
package supplier

import (
	"errors"
	"net/http"

	"bb_erp_echo/internal/shared/request"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

type Handler struct {
	Service Service
}

type supplierRequest struct {
	Name    string `json:"name" validate:"required"`
	Code    string `json:"code" validate:"required"`
	Contact string `json:"contact"`
	Phone   string `json:"phone"`
	Address string `json:"address"`
	Status  string `json:"status"`
}

func NewHandler(db *gorm.DB) *Handler {
	return NewHandlerWithService(NewService(db))
}

func NewHandlerWithService(service Service) *Handler {
	return &Handler{Service: service}
}

func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	group := v1.Group("/suppliers", audit)
	group.GET("", h.List, require("/api/v1/suppliers", "read"))
	group.POST("", h.Create, require("/api/v1/suppliers", "write"))
	group.PATCH("/:id", h.Update, require("/api/v1/suppliers", "write"))
}

func (h *Handler) List(c *echo.Context) error {
	items, err := h.Service.List()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

func (h *Handler) Create(c *echo.Context) error {
	var req supplierRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	item, err := h.Service.Create(req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

func (h *Handler) Update(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req supplierRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	item, err := h.Service.Update(id, req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "供应商不存在")
		}
		return err
	}
	return c.JSON(http.StatusOK, item)
}
