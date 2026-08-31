package discovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// ServiceConfig 是发现服务门面的运行配置。
type ServiceConfig struct {
	Enabled           bool
	BindHost          string
	Port              int
	HTTPPort          int
	ScanTimeout       time.Duration
	PreflightTimeout  time.Duration
	HTTPTimeout       time.Duration
	RateLimitInterval time.Duration
	RateLimitEntries  int
	Logger            *slog.Logger
}

// Service 是服务端局域网发现能力的统一门面，封装单例身份、启动预检、
// UDP 响应器和可观测运行错误。
type Service struct {
	cfg       ServiceConfig
	identity  Identity
	logger    *slog.Logger
	scanner   *Scanner
	responder *Responder

	mu     sync.Mutex
	conn   *net.UDPConn
	cancel context.CancelFunc
	done   chan struct{}
	errCh  chan error
}

// NewService 创建发现服务门面。
func NewService(cfg ServiceConfig, identity Identity) *Service {
	if cfg.BindHost == "" {
		cfg.BindHost = "0.0.0.0"
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		cfg.Port = DefaultPort
	}
	if cfg.HTTPPort <= 0 || cfg.HTTPPort > 65535 {
		cfg.HTTPPort = 8080
	}
	if cfg.ScanTimeout <= 0 {
		cfg.ScanTimeout = defaultScanTimeout
	}
	if cfg.PreflightTimeout <= 0 {
		cfg.PreflightTimeout = defaultPreflightTimeout
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = defaultHTTPTimeout
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	responder := NewResponder(ResponderConfig{
		BindHost:          cfg.BindHost,
		Port:              cfg.Port,
		HTTPPort:          cfg.HTTPPort,
		RateLimitInterval: cfg.RateLimitInterval,
		RateLimitEntries:  cfg.RateLimitEntries,
	}, identity)
	return &Service{
		cfg:      cfg,
		identity: identity,
		logger:   cfg.Logger,
		scanner: NewScanner(ScannerConfig{
			Port:             cfg.Port,
			ScanTimeout:      cfg.ScanTimeout,
			PreflightTimeout: cfg.PreflightTimeout,
			HTTPTimeout:      cfg.HTTPTimeout,
		}),
		responder: responder,
		errCh:     make(chan error, 1),
	}
}

// Enabled 表示当前部署是否启用 UDP 发现和启动预检。
func (s *Service) Enabled() bool { return s != nil && s.cfg.Enabled }

// Identity 返回当前服务的公开身份快照。
func (s *Service) Identity() IdentityResponse {
	if s == nil {
		return IdentityResponse{}
	}
	return s.identity.Response()
}

// Preflight 在启动自身 UDP 响应器前发现并验证已有 ERP 服务。
//
// 任意一个候选通过 /ready 和身份验证都会返回 PeerConflictError，即使
// candidate 的 instance_id 与本服务相同；重复的实例身份不能被用来绕过
// 同一局域网只允许一个服务的启动约束。
func (s *Service) Preflight(ctx context.Context) error {
	if s == nil || !s.cfg.Enabled {
		return nil
	}
	preflightCtx, cancel := context.WithTimeout(ctx, s.cfg.PreflightTimeout)
	defer cancel()
	candidates, err := s.scanner.Scan(preflightCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// A blocked UDP broadcast is a valid client fallback condition and
			// must not prevent a fresh server from starting.
			return nil
		}
		return fmt.Errorf("discovery preflight: %w", err)
	}
	if len(candidates) == 0 {
		return nil
	}
	return &PeerConflictError{Candidates: candidates}
}

// Start 启动 UDP 响应器。响应器运行期的非预期错误可从 Errors 读取。
func (s *Service) Start(ctx context.Context) error {
	if s == nil || !s.cfg.Enabled {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		return errors.New("discovery service is already running")
	}
	conn, err := s.responder.Listen()
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.conn = conn
	s.cancel = cancel
	s.done = done
	go func() {
		defer close(done)
		if err := s.responder.ServeConn(runCtx, conn); err != nil {
			select {
			case s.errCh <- fmt.Errorf("discovery service stopped unexpectedly: %w", err):
			default:
			}
			s.logger.Error("discovery service stopped unexpectedly", "error", err)
		}
	}()
	s.logger.Info("ERP discovery responder started", "address", net.JoinHostPort(s.cfg.BindHost, fmt.Sprintf("%d", s.cfg.Port)))
	return nil
}

// Errors 返回发现服务运行期的致命错误通道。正常 Shutdown 不会发送错误。
func (s *Service) Errors() <-chan error {
	if s == nil {
		return nil
	}
	return s.errCh
}

// Shutdown 停止 UDP 响应器并等待其退出。
func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil || !s.cfg.Enabled {
		return nil
	}
	s.mu.Lock()
	conn := s.conn
	cancel := s.cancel
	done := s.done
	s.conn = nil
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()
	if conn == nil {
		return nil
	}
	if cancel != nil {
		cancel()
	}
	_ = conn.Close()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		s.logger.Info("ERP discovery responder stopped")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown discovery service: %w", ctx.Err())
	}
}
