package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

const (
	// idempotencyScopeCreateDocument and idempotencyScopeCreateItemMovement are
	// intentionally different: the same client key must not cross API actions.
	idempotencyScopeCreateDocument     = "inventory_documents.create"
	idempotencyScopeCreateItemMovement = "inventory_item_movements.create"
)

type idempotencyMetadata struct {
	scope          string
	accountID      uint
	organizationID uint
	requestHash    string
}

func newIdempotencyMetadata(c *echo.Context, scope, requestHash string) idempotencyMetadata {
	metadata := idempotencyMetadata{scope: scope, requestHash: requestHash}
	if current := auth.GetCurrentUser(c); current != nil {
		metadata.accountID = current.ID
		metadata.organizationID = current.OrganizationID
	}
	return metadata
}

func (metadata idempotencyMetadata) matches(document model.InventoryDocument) bool {
	return document.IdempotencyScope == metadata.scope &&
		document.IdempotencyAccountID == metadata.accountID &&
		document.IdempotencyOrganizationID == metadata.organizationID &&
		document.IdempotencyRequestHash == metadata.requestHash
}

// findIdempotentDocument returns an existing result only when every part of
// the idempotency identity matches. A key collision with any different scope,
// account, organization, or business request is always a 409 conflict.
func findIdempotentDocument(db *gorm.DB, key string, metadata idempotencyMetadata) (model.InventoryDocument, bool, error) {
	if key == "" {
		return model.InventoryDocument{}, false, nil
	}
	var existing model.InventoryDocument
	err := db.Preload("Lines").Where("idempotency_key = ?", key).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.InventoryDocument{}, false, nil
	}
	if err != nil {
		return model.InventoryDocument{}, false, err
	}
	if !metadata.matches(existing) {
		return model.InventoryDocument{}, false, echo.NewHTTPError(http.StatusConflict, "幂等键已用于不同请求")
	}
	return existing, true, nil
}

func idempotencyConflictResponse(c *echo.Context, document model.InventoryDocument) error {
	return c.JSON(http.StatusOK, trimDocument(document, hasCostView(c)))
}

type normalizedDocumentRequest struct {
	Code               string                   `json:"code"`
	Type               string                   `json:"type"`
	WarehouseID        uint                     `json:"warehouse_id"`
	ToWarehouseID      *uint                    `json:"to_warehouse_id"`
	Reason             string                   `json:"reason"`
	Lines              []normalizedDocumentLine `json:"lines"`
	OperatorEmployeeID uint                     `json:"operator_employee_id"`
}

type normalizedDocumentLine struct {
	ItemType   string `json:"item_type"`
	ItemID     uint   `json:"item_id"`
	LocationID *uint  `json:"location_id"`
	Quantity   int64  `json:"quantity"`
	UnitCost   int64  `json:"unit_cost"`
	Amount     int64  `json:"amount"`
	Remark     string `json:"remark"`
}

func documentRequestHash(req documentRequest) string {
	lines := make([]normalizedDocumentLine, 0, len(req.Lines))
	for _, line := range req.Lines {
		amount := line.Amount
		if amount == 0 && line.UnitCost > 0 {
			amount = scaledAmount(line.Quantity, line.UnitCost)
		}
		lines = append(lines, normalizedDocumentLine{
			ItemType:   normalizeIdempotencyText(line.ItemType),
			ItemID:     line.ItemID,
			LocationID: line.LocationID,
			Quantity:   line.Quantity,
			UnitCost:   line.UnitCost,
			Amount:     amount,
			Remark:     normalizeIdempotencyText(line.Remark),
		})
	}
	return hashIdempotencyPayload(normalizedDocumentRequest{
		Code:               normalizeIdempotencyText(req.Code),
		Type:               normalizeIdempotencyText(req.Type),
		WarehouseID:        req.WarehouseID,
		ToWarehouseID:      req.ToWarehouseID,
		Reason:             normalizeIdempotencyText(req.Reason),
		Lines:              lines,
		OperatorEmployeeID: req.OperatorEmployeeID,
	})
}

type normalizedItemMovementRequest struct {
	ItemType           string `json:"item_type"`
	ItemID             uint   `json:"item_id"`
	BusinessType       string `json:"business_type"`
	Quantity           int64  `json:"quantity"`
	UnitCost           int64  `json:"unit_cost"`
	SupplierID         *uint  `json:"supplier_id"`
	CustomerID         *uint  `json:"customer_id"`
	DepartmentID       *uint  `json:"department_id"`
	OriginalDocumentID *uint  `json:"original_document_id"`
	Reason             string `json:"reason"`
	Remark             string `json:"remark"`
	OperatorEmployeeID uint   `json:"operator_employee_id"`
}

func itemMovementRequestHash(req itemMovementRequest, itemType string, itemID uint) string {
	return hashIdempotencyPayload(normalizedItemMovementRequest{
		ItemType:           normalizeIdempotencyText(itemType),
		ItemID:             itemID,
		BusinessType:       normalizeIdempotencyText(req.BusinessType),
		Quantity:           req.Quantity,
		UnitCost:           req.UnitCost,
		SupplierID:         req.SupplierID,
		CustomerID:         req.CustomerID,
		DepartmentID:       req.DepartmentID,
		OriginalDocumentID: req.OriginalDocumentID,
		Reason:             normalizeIdempotencyText(req.Reason),
		Remark:             normalizeIdempotencyText(req.Remark),
		OperatorEmployeeID: req.OperatorEmployeeID,
	})
}

func hashIdempotencyPayload(payload any) string {
	serialized, err := json.Marshal(payload)
	if err != nil {
		// These payloads contain only scalar values and therefore cannot fail to
		// marshal. Keep a deterministic fallback rather than silently dropping
		// idempotency protection if that invariant changes later.
		serialized = []byte{}
	}
	digest := sha256.Sum256(serialized)
	return hex.EncodeToString(digest[:])
}

func normalizeIdempotencyText(value string) string {
	return strings.TrimSpace(value)
}

func normalizeIdempotencyKey(value string) string {
	return normalizeIdempotencyText(value)
}
