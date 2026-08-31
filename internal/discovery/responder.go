package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	defaultRateLimitInterval = 250 * time.Millisecond
	defaultRateLimitEntries  = 256
)

// ResponderConfig 描述 UDP 响应器监听和限流参数。
type ResponderConfig struct {
	BindHost          string
	Port              int
	HTTPPort          int
	RateLimitInterval time.Duration
	RateLimitEntries  int
}

// Responder 在 UDP 发现端口接收严格校验的请求，并向来源地址单播响应。
type Responder struct {
	cfg      ResponderConfig
	identity Identity
	limiter  *sourceRateLimiter
}

// NewResponder 创建 UDP 发现响应器。
func NewResponder(cfg ResponderConfig, identity Identity) *Responder {
	if cfg.BindHost == "" {
		cfg.BindHost = "0.0.0.0"
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		cfg.Port = DefaultPort
	}
	if cfg.RateLimitInterval <= 0 {
		cfg.RateLimitInterval = defaultRateLimitInterval
	}
	if cfg.RateLimitEntries <= 0 {
		cfg.RateLimitEntries = defaultRateLimitEntries
	}
	return &Responder{
		cfg:      cfg,
		identity: identity,
		limiter:  newSourceRateLimiter(cfg.RateLimitInterval, cfg.RateLimitEntries),
	}
}

// Listen 打开 UDP4 监听 socket。调用方负责随后调用 ServeConn 和关闭连接。
func (r *Responder) Listen() (*net.UDPConn, error) {
	address := &net.UDPAddr{IP: net.ParseIP(r.cfg.BindHost), Port: r.cfg.Port}
	if address.IP == nil {
		return nil, fmt.Errorf("invalid discovery bind host %q", r.cfg.BindHost)
	}
	if address.IP.To4() == nil {
		return nil, errors.New("discovery responder requires an IPv4 bind host")
	}
	address.IP = address.IP.To4()
	conn, err := net.ListenUDP("udp4", address)
	if err != nil {
		return nil, fmt.Errorf("listen discovery UDP %s:%d: %w", r.cfg.BindHost, r.cfg.Port, err)
	}
	return conn, nil
}

// Serve 在指定 context 下监听发现请求。监听或读取错误会返回给上层服务，
// 但单个非法包、来源不允许或响应发送失败不会终止响应器。
func (r *Responder) Serve(ctx context.Context) error {
	conn, err := r.Listen()
	if err != nil {
		return err
	}
	return r.ServeConn(ctx, conn)
}

// ServeConn 使用指定 socket 提供发现响应，便于生命周期管理和隔离测试。
func (r *Responder) ServeConn(ctx context.Context, conn *net.UDPConn) error {
	if conn == nil {
		return errors.New("serve discovery UDP: connection is nil")
	}
	closeOnCancel := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closeOnCancel:
		}
	}()
	defer close(closeOnCancel)
	defer conn.Close()

	buffer := make([]byte, MaxPacketBytes+1)
	for {
		n, source, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("read discovery UDP packet: %w", err)
		}
		if n == 0 || n > MaxPacketBytes || source == nil || !isAllowedIPv4(source.IP) {
			continue
		}
		if !r.limiter.Allow(source.IP.String(), time.Now()) {
			continue
		}
		request, err := DecodeRequest(buffer[:n])
		if err != nil {
			continue
		}
		payload, err := EncodeAnnouncement(r.identity.Announcement(request.Nonce, r.cfg.HTTPPort))
		if err != nil {
			// Identity/configuration is validated during app construction. Keep
			// malformed configuration from crashing the UDP serving goroutine.
			continue
		}
		// A response write failure for one datagram is not a listener failure;
		// the source may simply have disappeared between request and response.
		_, _ = conn.WriteToUDP(payload, source)
	}
}

type sourceRateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	max      int
	seen     map[string]time.Time
}

func newSourceRateLimiter(interval time.Duration, max int) *sourceRateLimiter {
	return &sourceRateLimiter{interval: interval, max: max, seen: make(map[string]time.Time, max)}
}

func (l *sourceRateLimiter) Allow(source string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if previous, ok := l.seen[source]; ok && now.Sub(previous) < l.interval {
		return false
	}
	if len(l.seen) >= l.max {
		var oldestKey string
		var oldest time.Time
		for key, timestamp := range l.seen {
			if oldestKey == "" || timestamp.Before(oldest) {
				oldestKey = key
				oldest = timestamp
			}
		}
		if oldestKey != "" {
			delete(l.seen, oldestKey)
		}
	}
	l.seen[source] = now
	return true
}
