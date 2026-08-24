package inventory

import (
	"net/http"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"

	"github.com/labstack/echo/v4"
)

// MovementStrategy 封装一种库存业务的方向和关联对象校验。
// 新增业务类型时注册新策略即可，无需修改库存过账主流程。
type MovementStrategy interface {
	Type() string
	Direction() string
	Validate(context MovementValidationContext, request itemMovementRequest, itemType string, itemID uint) error
}

// MovementValidationContext 隔离策略对数据库实现的依赖，便于替换存储和单元测试。
type MovementValidationContext interface {
	RequireSupplier(id uint) error
	RequireCustomer(id uint) error
	RequireDepartment(id uint) error
	ValidateOriginalDocument(id uint, itemType string, itemID uint, customerID, departmentID *uint) error
	RequirePermission(code string) error
}

type movementStrategy struct {
	businessType string
	direction    string
	validate     func(MovementValidationContext, itemMovementRequest, string, uint) error
}

func (s movementStrategy) Type() string      { return s.businessType }
func (s movementStrategy) Direction() string { return s.direction }
func (s movementStrategy) Validate(context MovementValidationContext, request itemMovementRequest, itemType string, itemID uint) error {
	return s.validate(context, request, itemType, itemID)
}

func DefaultMovementStrategies() map[string]MovementStrategy {
	items := []MovementStrategy{
		movementStrategy{businessType: businessPurchaseInbound, direction: typeInbound, validate: validatePurchaseInbound},
		movementStrategy{businessType: businessReturnReworkInbound, direction: typeInbound, validate: validateReturnReworkInbound},
		movementStrategy{businessType: businessCustomerOutbound, direction: typeOutbound, validate: validateCustomerOutbound},
		movementStrategy{businessType: businessDepartmentOutbound, direction: typeOutbound, validate: validateDepartmentOutbound},
	}
	result := make(map[string]MovementStrategy, len(items))
	for _, item := range items {
		result[item.Type()] = item
	}
	return result
}

func validatePurchaseInbound(context MovementValidationContext, req itemMovementRequest, _ string, _ uint) error {
	if req.SupplierID == nil || req.CustomerID != nil || req.DepartmentID != nil || req.OriginalDocumentID != nil {
		return invalidMovement("采购入库必须且只能关联供应商")
	}
	if err := context.RequirePermission("suppliers:read"); err != nil {
		return err
	}
	if req.UnitCost > 0 {
		if err := context.RequirePermission("cost:view"); err != nil {
			return err
		}
	}
	return context.RequireSupplier(*req.SupplierID)
}

func validateReturnReworkInbound(context MovementValidationContext, req itemMovementRequest, itemType string, itemID uint) error {
	if (req.CustomerID == nil) == (req.DepartmentID == nil) || req.SupplierID != nil {
		return invalidMovement("退货返工必须且只能选择客户或部门作为来源")
	}
	if req.CustomerID != nil {
		if err := context.RequirePermission("customers:read"); err != nil {
			return err
		}
		if err := context.RequireCustomer(*req.CustomerID); err != nil {
			return err
		}
	} else {
		if err := context.RequirePermission("system:departments:read"); err != nil {
			return err
		}
		if err := context.RequireDepartment(*req.DepartmentID); err != nil {
			return err
		}
	}
	if req.OriginalDocumentID != nil {
		return context.ValidateOriginalDocument(*req.OriginalDocumentID, itemType, itemID, req.CustomerID, req.DepartmentID)
	}
	return nil
}

func validateCustomerOutbound(context MovementValidationContext, req itemMovementRequest, _ string, _ uint) error {
	if req.CustomerID == nil || req.SupplierID != nil || req.DepartmentID != nil || req.OriginalDocumentID != nil {
		return invalidMovement("客户出库必须且只能关联客户")
	}
	if err := context.RequirePermission("customers:read"); err != nil {
		return err
	}
	return context.RequireCustomer(*req.CustomerID)
}

func validateDepartmentOutbound(context MovementValidationContext, req itemMovementRequest, _ string, _ uint) error {
	if req.DepartmentID == nil || req.SupplierID != nil || req.CustomerID != nil || req.OriginalDocumentID != nil {
		return invalidMovement("部门出库必须且只能关联目标部门")
	}
	if err := context.RequirePermission("system:departments:read"); err != nil {
		return err
	}
	return context.RequireDepartment(*req.DepartmentID)
}

func invalidMovement(message string) error {
	return echo.NewHTTPError(http.StatusBadRequest, message)
}

type requestMovementValidationContext struct {
	handler *Handler
	user    *auth.CurrentUser
}

func (context requestMovementValidationContext) RequireSupplier(id uint) error {
	return context.handler.requireRecord(&model.Supplier{}, id, "供应商不存在")
}

func (context requestMovementValidationContext) RequireCustomer(id uint) error {
	return context.handler.requireRecord(&model.Customer{}, id, "客户不存在")
}

func (context requestMovementValidationContext) RequireDepartment(id uint) error {
	return context.handler.requireRecord(&model.Department{}, id, "部门不存在")
}

func (context requestMovementValidationContext) ValidateOriginalDocument(id uint, itemType string, itemID uint, customerID, departmentID *uint) error {
	return context.handler.validateOriginalDocument(id, itemType, itemID, customerID, departmentID)
}

func (context requestMovementValidationContext) RequirePermission(code string) error {
	if context.user != nil {
		for _, permission := range context.user.Permissions {
			if permission == code {
				return nil
			}
		}
	}
	return echo.NewHTTPError(http.StatusForbidden, "缺少办理该业务所需的关联资料权限")
}
