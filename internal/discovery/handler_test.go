package discovery

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestGetIdentityDisablesCaching(t *testing.T) {
	identity := Identity{
		InstanceID:        "88888888-8888-4888-8888-888888888888",
		Product:           Product,
		DiscoveryProtocol: ProtocolVersion,
		ServerName:        "缓存测试服务",
		ServerVersion:     "1.0.0",
	}
	handler := NewHandler(identity)
	e := echo.New()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/discovery/identity", nil)
	ctx := e.NewContext(request, recorder)

	if err := handler.GetIdentity(ctx); err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}
