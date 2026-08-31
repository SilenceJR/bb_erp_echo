// Package customer 提供全新的客户编码与客户资料接口。
package customer

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

type ErrorResponse = response.ErrorBody
type CodeRequest struct {
	Code string `json:"code"`
}

type Handler struct {
	Service *Service
	DB      *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler { return &Handler{Service: NewService(db), DB: db} }

func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	codes := v1.Group("/customer-codes", audit)
	codes.GET("", h.listCodes, require("/api/v1/customers", "read"))
	codes.GET("/next", h.nextCode, require("/api/v1/customers", "read"))
	codes.POST("", h.createCode, require("/api/v1/customers", "write"))
	codes.PATCH("/:id", h.updateCode, require("/api/v1/customers", "write"))
	codes.DELETE("/:id", h.deleteCode, require("/api/v1/customers", "write"))
	customers := v1.Group("/customers", audit)
	customers.GET("", h.listProfiles, require("/api/v1/customers", "read"))
	customers.GET("/options", h.options, require("/api/v1/customers", "read"))
	h.registerExcelRoutes(customers, require)
	customers.POST("", h.createProfile, require("/api/v1/customers", "write"))
	customers.GET("/:id", h.getProfile, require("/api/v1/customers", "read"))
	customers.PATCH("/:id", h.updateProfile, require("/api/v1/customers", "write"))
	customers.PUT("/:id/default", h.setDefault, require("/api/v1/customers", "write"))
	customers.DELETE("/:id", h.deleteProfile, require("/api/v1/customers", "write"))
}

// @Summary 客户编码分组列表
// @Tags 客户
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页条数"
// @Param q query string false "编码或资料关键词"
// @Param filter query string false "all、multiple 或 empty"
// @Success 200 {object} CodePage
// @Router /api/v1/customer-codes [get]
func (h *Handler) listCodes(c *echo.Context) error {
	result, err := h.Service.ListCodes(pagination.FromEcho(c), strings.TrimSpace(c.QueryParam("filter")))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

// @Summary 查询下一个客户编码建议值
// @Tags 客户
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]string
// @Router /api/v1/customer-codes/next [get]
func (h *Handler) nextCode(c *echo.Context) error {
	code, err := h.Service.NextCode()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"code": code})
}

// @Summary 创建客户编码
// @Tags 客户
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body CodeRequest false "编码留空时自动生成"
// @Success 201 {object} CodeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /api/v1/customer-codes [post]
func (h *Handler) createCode(c *echo.Context) error {
	var body CodeRequest
	if c.Request().ContentLength != 0 {
		if err := request.BindAndValidate(c, &body); err != nil {
			return err
		}
	}
	item, err := h.Service.CreateCode(body.Code)
	if err != nil {
		return customerHTTPError(err)
	}
	return c.JSON(http.StatusCreated, item)
}

// @Summary 修改客户编码
// @Tags 客户
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "客户编码 ID"
// @Param body body CodeRequest true "客户编码"
// @Success 200 {object} CodeResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /api/v1/customer-codes/{id} [patch]
func (h *Handler) updateCode(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var body CodeRequest
	if err = request.BindAndValidate(c, &body); err != nil {
		return err
	}
	item, err := h.Service.UpdateCode(id, body.Code)
	if err != nil {
		return customerHTTPError(err)
	}
	return c.JSON(http.StatusOK, item)
}

// @Summary 删除空客户编码
// @Tags 客户
// @Security BearerAuth
// @Param id path int true "客户编码 ID"
// @Success 204
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /api/v1/customer-codes/{id} [delete]
func (h *Handler) deleteCode(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	if err = h.Service.DeleteCode(id); err != nil {
		return customerHTTPError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// @Summary 查询客户资料
// @Tags 客户
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页条数"
// @Param q query string false "客户关键词"
// @Success 200 {object} ProfilePage
// @Router /api/v1/customers [get]
func (h *Handler) listProfiles(c *echo.Context) error {
	result, err := h.Service.ListProfiles(pagination.FromEcho(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

// @Summary 查询客户资料选择项
// @Tags 客户
// @Security BearerAuth
// @Produce json
// @Param q query string false "编码、简称或名称"
// @Success 200 {array} OptionResponse
// @Router /api/v1/customers/options [get]
func (h *Handler) options(c *echo.Context) error {
	items, err := h.Service.Options(strings.TrimSpace(c.QueryParam("q")))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// @Summary 创建客户资料
// @Tags 客户
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body ProfileInput true "客户资料"
// @Success 201 {object} ProfileResponse
// @Router /api/v1/customers [post]
func (h *Handler) createProfile(c *echo.Context) error {
	var body ProfileInput
	if err := request.BindAndValidate(c, &body); err != nil {
		return err
	}
	item, err := h.Service.CreateProfile(body)
	if err != nil {
		return customerHTTPError(err)
	}
	return c.JSON(http.StatusCreated, item)
}

// @Summary 查询客户资料详情
// @Tags 客户
// @Security BearerAuth
// @Produce json
// @Param id path int true "客户资料 ID"
// @Success 200 {object} ProfileResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/customers/{id} [get]
func (h *Handler) getProfile(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	item, err := h.Service.GetProfile(id)
	if err != nil {
		return customerHTTPError(err)
	}
	return c.JSON(http.StatusOK, item)
}

// @Summary 修改客户资料
// @Tags 客户
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "客户资料 ID"
// @Param body body ProfileUpdate true "客户资料；不能更换所属编码"
// @Success 200 {object} ProfileResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/customers/{id} [patch]
func (h *Handler) updateProfile(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var body ProfileUpdate
	if err = request.BindAndValidate(c, &body); err != nil {
		return err
	}
	item, err := h.Service.UpdateProfile(id, body)
	if err != nil {
		return customerHTTPError(err)
	}
	return c.JSON(http.StatusOK, item)
}

// @Summary 设为默认客户资料
// @Tags 客户
// @Security BearerAuth
// @Produce json
// @Param id path int true "客户资料 ID"
// @Success 200 {object} ProfileResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/customers/{id}/default [put]
func (h *Handler) setDefault(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	item, err := h.Service.SetDefault(id)
	if err != nil {
		return customerHTTPError(err)
	}
	return c.JSON(http.StatusOK, item)
}

// @Summary 物理删除客户资料
// @Description 默认资料仍有同码其他资料时必须通过 replacement_id 指定替代默认资料；被业务引用时返回 409。
// @Tags 客户
// @Security BearerAuth
// @Param id path int true "客户资料 ID"
// @Param replacement_id query int false "同编码替代默认资料 ID"
// @Success 204
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /api/v1/customers/{id} [delete]
func (h *Handler) deleteProfile(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var replacement uint64
	if raw := strings.TrimSpace(c.QueryParam("replacement_id")); raw != "" {
		replacement, err = strconv.ParseUint(raw, 10, 64)
		if err != nil || replacement == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "replacement_id 无效")
		}
	}
	if err = h.Service.DeleteProfile(id, uint(replacement)); err != nil {
		return customerHTTPError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func customerHTTPError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "客户编码或客户资料不存在")
	case errors.Is(err, ErrCodeConflict):
		return echo.NewHTTPError(http.StatusConflict, "客户编码已存在")
	case errors.Is(err, ErrCodeHasProfiles):
		return echo.NewHTTPError(http.StatusConflict, "客户编码仍有关联资料，不能删除")
	case errors.Is(err, ErrProfileReferenced):
		return echo.NewHTTPError(http.StatusConflict, "客户资料已被业务记录引用，不能删除")
	case errors.Is(err, ErrReplacementNeeded):
		return echo.NewHTTPError(http.StatusConflict, "删除默认资料前必须选择替代默认资料")
	case errors.Is(err, ErrInvalidReplacement):
		return echo.NewHTTPError(http.StatusBadRequest, "替代默认资料无效")
	default:
		if strings.Contains(err.Error(), "客户编码") {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return err
	}
}
