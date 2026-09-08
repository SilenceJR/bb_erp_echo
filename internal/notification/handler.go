package notification

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"bb_erp_echo/internal/auth"

	"github.com/labstack/echo/v5"
)

// Handler 暴露通知 SSE 连接。
type Handler struct {
	Service *Service
}

// NewHandler 创建通知处理器。
func NewHandler(service *Service) *Handler { return &Handler{Service: service} }

// RegisterRoutes 注册需要 JWT 的实时通知路由。
func (h *Handler) RegisterRoutes(protected *echo.Group) {
	if h == nil || h.Service == nil || protected == nil {
		return
	}
	protected.GET("/notifications/stream", h.Stream)
}

// Stream 建立带 Bearer 鉴权的 fetch-SSE 连接。
//
// URL 不接受令牌参数；JWT 必须由外层认证中间件从 Authorization header
// 解析。连接不回放历史消息，断线后由客户端重新建立连接并刷新当前模块。
//
// @Summary 实时数据变更通知
// @Description 建立当前登录账号的临时 Server-Sent Events 通知流；消息不持久化且断线不回放。
// @Tags 通知
// @Security BearerAuth
// @Produce text/event-stream
// @Success 200 {string} string "SSE stream"
// @Failure 401 {object} map[string]string
// @Failure 429 {object} map[string]string
// @Router /api/v1/notifications/stream [get]
func (h *Handler) Stream(c *echo.Context) error {
	if h == nil || h.Service == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "通知服务不可用")
	}
	current := auth.GetCurrentUser(c)
	if current == nil || current.ID == 0 {
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}
	events, cancel, err := h.Service.Subscribe(current.ID)
	if err != nil {
		if errors.Is(err, ErrConnectionLimit) {
			return echo.NewHTTPError(http.StatusTooManyRequests, "通知连接数已达上限")
		}
		return echo.NewHTTPError(http.StatusServiceUnavailable, "通知服务不可用")
	}
	defer cancel()

	writer := c.Response()
	writer.Header().Set(echo.HeaderContentType, "text/event-stream; charset=utf-8")
	writer.Header().Set(echo.HeaderCacheControl, "no-cache, no-transform")
	writer.Header().Set("X-Accel-Buffering", "no")
	// Echo/HTTP Server 默认写超时为两分钟；SSE 是单个长期响应，必须清除
	// 该 deadline。连接关闭时由请求 context 和客户端断开结束循环。
	if err := http.NewResponseController(writer).SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusNotImplemented, "当前连接不支持实时通知")
	}

	if _, err := io.WriteString(writer, "retry: 3000\n\n"); err != nil {
		return err
	}
	flusher.Flush()
	ticker := time.NewTicker(h.Service.config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return nil
			}
			if err := writeEvent(writer, flusher, event); err != nil {
				return err
			}
		case <-ticker.C:
			if _, err := io.WriteString(writer, ": keep-alive\n\n"); err != nil {
				return err
			}
			flusher.Flush()
		case <-c.Request().Context().Done():
			return nil
		}
	}
}

func writeEvent(writer io.Writer, flusher http.Flusher, event Notification) error {
	data, err := marshalNotification(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event.Kind, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func marshalNotification(event Notification) ([]byte, error) {
	return json.Marshal(event)
}
