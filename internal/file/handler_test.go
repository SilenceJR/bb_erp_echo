package file

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/role"

	"github.com/labstack/echo/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestListOwnerCategoryAndOrder(t *testing.T) {
	h, db := testHandler(t)
	product := model.Product{Name: "P", Code: "P-1"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range []model.ImageFile{{OwnerType: OwnerProduct, OwnerID: product.ID, Category: "main", StoragePath: "product/1.png", UploadedBy: 1}, {OwnerType: OwnerProduct, OwnerID: product.ID, Category: "detail", StoragePath: "product/2.png", UploadedBy: 1}, {OwnerType: OwnerProduct, OwnerID: product.ID, Category: "main", StoragePath: "product/3.png", UploadedBy: 1}} {
		if err := db.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
	}
	c := contextFor(t, http.MethodGet, "/api/v1/files?owner_type=product&owner_id=1&category=main", nil)
	setUser(c, &auth.CurrentUser{Username: "warehouse", OrganizationID: 1})
	if err := h.List(c); err != nil {
		t.Fatal(err)
	}
	var listed []ImageResponse
	if err := json.Unmarshal(c.Get("test_recorder").(*httptest.ResponseRecorder).Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].ID <= listed[1].ID || listed[0].Category != "main" || listed[1].Category != "main" {
		t.Fatalf("unexpected filtered list: %+v", listed)
	}
	missing := contextFor(t, http.MethodGet, "/api/v1/files", nil)
	setUser(missing, &auth.CurrentUser{Username: "warehouse", OrganizationID: 1})
	if err := h.List(missing); err == nil {
		t.Fatal("global list accepted")
	}
}

func TestPermissionAndDepartmentBoundaries(t *testing.T) {
	h, db := testHandler(t)
	product := model.Product{Name: "P", Code: "P-2"}
	mold := model.Mold{Name: "M", Code: "M-2"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&mold).Error; err != nil {
		t.Fatal(err)
	}
	dept2 := uint(2)
	task := model.DepartmentTask{DepartmentID: dept2, WorkOrderID: 1, Title: "T"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ImageFile{OwnerType: OwnerDepartmentTask, OwnerID: task.ID, StoragePath: "department_task/x.png", UploadedBy: 1}).Error; err != nil {
		t.Fatal(err)
	}
	warehouse := &auth.CurrentUser{Username: "warehouse", OrganizationID: 1}
	c := multipartContext(t, http.MethodPost, "/api/v1/files/images", "product", product.ID, "main", "one.png", []byte("\x89PNG\r\n\x1a\n"))
	setUser(c, warehouse)
	if err := h.Create(c); err != nil {
		t.Fatal(err)
	}
	moldC := multipartContext(t, http.MethodPost, "/api/v1/files/images", "mold", mold.ID, "", "one.png", []byte("\x89PNG\r\n\x1a\n"))
	setUser(moldC, warehouse)
	if err := h.Create(moldC); err == nil {
		t.Fatal("warehouse permission uploaded mold")
	}
	old := &auth.CurrentUser{Username: "legacy", OrganizationID: 1}
	h.enforcer.AddPolicy("legacy", "/api/v1/tasks", "read", "*", "*")
	h.enforcer.AddPolicy("legacy", "/api/v1/tasks", "write", "*", "*")
	work := model.WorkOrder{Code: "W-1", Title: "W"}
	if err := db.Create(&work).Error; err != nil {
		t.Fatal(err)
	}
	wc := multipartContext(t, http.MethodPost, "/api/v1/files/images", OwnerWorkOrder, work.ID, "", "one.png", []byte("\x89PNG\r\n\x1a\n"))
	setUser(wc, old)
	if err := h.Create(wc); err != nil {
		t.Fatalf("legacy tasks permission: %v", err)
	}
	reader := &auth.CurrentUser{Username: "dept", OrganizationID: 1, DepartmentID: ptr(uint(1))}
	h.enforcer.AddPolicy("dept", "/api/v1/workorder", "read", "*", "*")
	h.enforcer.AddPolicy("dept", "/api/v1/workorder", "write", "*", "*")
	lc := contextFor(t, http.MethodGet, "/api/v1/files?owner_type=department_task&owner_id=1", nil)
	setUser(lc, reader)
	lc.Request().URL.RawQuery = "owner_type=" + OwnerDepartmentTask + "&owner_id=" + uintString(task.ID)
	if err := h.List(lc); err != nil {
		t.Fatalf("cross department read: %v", err)
	}
	wc2 := multipartContext(t, http.MethodPut, "/1/content", OwnerDepartmentTask, task.ID, "", "one.png", []byte("\x89PNG\r\n\x1a\n"))
	setUser(wc2, reader)
	wc2.SetPath("/api/v1/files/:id/content")
	wc2.SetPathValues(echo.PathValues{{Name: "id", Value: uintString(task.ID)}})
	if err := h.Replace(wc2); err == nil {
		t.Fatal("cross department write accepted")
	} else {
		var httpErr *echo.HTTPError
		if !errors.As(err, &httpErr) || httpErr.Code != http.StatusForbidden {
			t.Fatalf("cross department error = %v", err)
		}
	}
}

func TestContentMissingAndServicePathValidation(t *testing.T) {
	h, db := testHandler(t)
	product := model.Product{Name: "P", Code: "P-3"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	asset := model.ImageFile{OwnerType: OwnerProduct, OwnerID: product.ID, StoragePath: filepath.Join("product", "missing.png"), MimeType: "image/png", Size: 3}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatal(err)
	}
	c := contextFor(t, http.MethodGet, "/api/v1/files/1/content", nil)
	c.SetPath("/api/v1/files/:id/content")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: uintString(asset.ID)}})
	setUser(c, &auth.CurrentUser{Username: "warehouse", OrganizationID: 1})
	if err := h.Content(c); err == nil {
		t.Fatal("missing content accepted")
	} else {
		var httpErr *echo.HTTPError
		if !errors.As(err, &httpErr) || httpErr.Code != http.StatusNotFound {
			t.Fatalf("missing content error = %v", err)
		}
	}
	ownerMissing := contextFor(t, http.MethodGet, "/api/v1/files?owner_type=product&owner_id=999", nil)
	setUser(ownerMissing, &auth.CurrentUser{Username: "warehouse", OrganizationID: 1})
	if err := h.List(ownerMissing); err == nil {
		t.Fatal("missing owner accepted")
	} else {
		var httpErr *echo.HTTPError
		if !errors.As(err, &httpErr) || httpErr.Code != http.StatusNotFound {
			t.Fatalf("missing owner error = %v", err)
		}
	}
	if _, err := h.service.SaveImage(uploadHeader(t, "one.png", []byte("\x89PNG\r\n\x1a\n")), "../../outside", 1, "", nil, 1); err == nil {
		t.Fatal("invalid owner path accepted")
	}
}

func testHandler(t *testing.T) (*Handler, *gorm.DB) {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Product{}, &model.Mold{}, &model.WorkOrder{}, &model.DepartmentTask{}, &model.ImageFile{}); err != nil {
		t.Fatal(err)
	}
	e, err := role.NewEnforcer()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = e.AddPolicy("warehouse", "/api/v1/warehouse", "read", "*", "*")
	_, _ = e.AddPolicy("warehouse", "/api/v1/warehouse", "write", "*", "*")
	return NewHandler(NewService(t.TempDir(), db), db, e), db
}
func contextFor(t *testing.T, method, path string, body *bytes.Buffer) *echo.Context {
	t.Helper()
	if body == nil {
		body = bytes.NewBuffer(nil)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(method, path, body), rec)
	c.Set("test_recorder", rec)
	return c
}
func setUser(c *echo.Context, u *auth.CurrentUser) { c.Set(auth.ContextUserKey, u) }
func ptr(v uint) *uint                             { return &v }
func uintString(v uint) string                     { return idString(v) }
func multipartContext(t *testing.T, method, path, ownerType string, ownerID uint, category, name string, data []byte) *echo.Context {
	t.Helper()
	body := new(bytes.Buffer)
	w := multipart.NewWriter(body)
	_ = w.WriteField("owner_type", ownerType)
	_ = w.WriteField("owner_id", uintString(ownerID))
	_ = w.WriteField("category", category)
	part, err := w.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = w.Close()
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return echo.New().NewContext(req, httptest.NewRecorder())
}
