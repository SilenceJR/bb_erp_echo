package discovery

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"encoding/json"
)

const (
	defaultScanTimeout       = 2500 * time.Millisecond
	defaultPreflightTimeout  = 3 * time.Second
	defaultHTTPTimeout       = 2 * time.Second
	defaultHTTPBodyMaxBytes  = 16 * 1024
	maxValidationCandidates  = 24
	maxValidationConcurrency = 4
)

// ScannerConfig 描述服务启动预检使用的 UDP 和 HTTP 参数。
type ScannerConfig struct {
	Port             int
	ScanTimeout      time.Duration
	PreflightTimeout time.Duration
	HTTPTimeout      time.Duration
	MaxHTTPBodyBytes int64
}

// Candidate 是一个已经通过 /ready 和匿名身份接口验证的服务候选。
type Candidate struct {
	Origin       string
	SourceIP     net.IP
	Announcement Announcement
	Identity     IdentityResponse
}

// PeerConflictError 包含导致本服务拒绝启动的已验证候选服务。
type PeerConflictError struct {
	Candidates []Candidate
}

func (e *PeerConflictError) Error() string {
	if len(e.Candidates) == 0 {
		return ErrPeerConflict.Error()
	}
	parts := make([]string, 0, len(e.Candidates))
	for _, candidate := range e.Candidates {
		name := strings.TrimSpace(candidate.Identity.ServerName)
		if name == "" {
			name = "未命名服务"
		}
		parts = append(parts, fmt.Sprintf("%s (%s, instance_id=%s)", name, candidate.Origin, candidate.Identity.InstanceID))
	}
	return fmt.Sprintf("%s: %s", ErrPeerConflict, strings.Join(parts, "; "))
}

func (e *PeerConflictError) Unwrap() error { return ErrPeerConflict }

// IsPeerConflict 判断错误是否表示发现了已验证的内网服务。
func IsPeerConflict(err error) bool { return errors.Is(err, ErrPeerConflict) }

// Scanner 使用当前主机的私网 IPv4 网卡广播发现请求并验证响应。
type Scanner struct {
	cfg ScannerConfig
}

// NewScanner 创建一个局域网服务发现扫描器。
func NewScanner(cfg ScannerConfig) *Scanner {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		cfg.Port = DefaultPort
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
	if cfg.MaxHTTPBodyBytes <= 0 {
		cfg.MaxHTTPBodyBytes = defaultHTTPBodyMaxBytes
	}
	return &Scanner{cfg: cfg}
}

// Scan 在每个可用私网 IPv4 网卡上发送一次 UDP 广播，并在固定窗口内收集
// 通过 HTTP 就绪和身份校验的候选服务。
func (s *Scanner) Scan(ctx context.Context) ([]Candidate, error) {
	targets, err := privateBroadcastTargets()
	if err != nil {
		return nil, err
	}
	return s.ScanTargets(ctx, targets)
}

// ScanTargets 使用给定的 IPv4 广播或回环目标执行扫描。它保留为公开方法，
// 让测试和受控内网部署可以不依赖操作系统网卡枚举。
func (s *Scanner) ScanTargets(ctx context.Context, targets []net.IP) ([]Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, nil
	}

	nonce, err := NewNonce()
	if err != nil {
		return nil, err
	}
	payload, err := EncodeRequest(nonce)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return nil, fmt.Errorf("open discovery preflight socket: %w", err)
	}
	defer conn.Close()

	// The preflight deadline covers both UDP collection and HTTP validation. A
	// shorter scan window is nested inside it so a slow peer cannot extend the
	// startup gate indefinitely.
	preflightCtx, cancelPreflight := context.WithTimeout(ctx, s.cfg.PreflightTimeout)
	defer cancelPreflight()
	scanCtx, cancelScan := context.WithTimeout(preflightCtx, s.cfg.ScanTimeout)
	defer cancelScan()
	closeOnCancel := make(chan struct{})
	go func() {
		select {
		case <-scanCtx.Done():
			_ = conn.Close()
		case <-closeOnCancel:
		}
	}()
	defer close(closeOnCancel)

	for _, targetIP := range targets {
		ip := targetIP.To4()
		if !isAllowedIPv4(ip) {
			continue
		}
		if _, err := conn.WriteToUDP(payload, &net.UDPAddr{IP: ip, Port: s.cfg.Port}); err != nil {
			// UDP broadcast may be blocked by a host firewall. It is a discovery
			// miss, not a reason to prevent the service from starting.
			continue
		}
	}

	buffer := make([]byte, MaxPacketBytes+1)
	seen := make(map[string]struct{})
	announcements := make([]announcementCandidate, 0, maxValidationCandidates)
	for {
		n, source, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if errors.Is(scanCtx.Err(), context.DeadlineExceeded) {
				break
			}
			if errors.Is(scanCtx.Err(), context.Canceled) {
				return nil, scanCtx.Err()
			}
			if errors.Is(err, net.ErrClosed) {
				if scanCtx.Err() != nil {
					if errors.Is(scanCtx.Err(), context.DeadlineExceeded) {
						break
					}
					return nil, scanCtx.Err()
				}
				break
			}
			return nil, fmt.Errorf("read discovery preflight response: %w", err)
		}
		if n == 0 || n > MaxPacketBytes || source == nil || !isAllowedIPv4(source.IP) {
			continue
		}
		announcement, err := DecodeAnnouncement(buffer[:n])
		if err != nil || announcement.Nonce != nonce {
			continue
		}
		key := source.IP.String() + ":" + strconv.Itoa(announcement.HTTPPort) + ":" + announcement.InstanceID
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if len(announcements) >= maxValidationCandidates {
			// Keep reading neither unbounded input nor an attacker-controlled
			// validation queue. The first bounded set is enough to enforce the
			// single-service startup rule.
			break
		}
		announcements = append(announcements, announcementCandidate{
			sourceIP:     append(net.IP(nil), source.IP...),
			announcement: announcement,
		})
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := preflightCtx.Err(); err != nil {
		// A local global deadline is the documented best-effort fallback. The
		// parent context's cancellation still propagates to the caller above.
		return nil, nil
	}
	candidates := s.validateCandidates(preflightCtx, announcements)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return candidates, nil
}

type announcementCandidate struct {
	sourceIP     net.IP
	announcement Announcement
}

type validationResult struct {
	index     int
	candidate Candidate
	err       error
}

// validateCandidates verifies a bounded candidate set with a small worker
// pool. Collection and validation are intentionally separate: one slow peer
// cannot prevent another valid peer from being checked within the same window.
func (s *Scanner) validateCandidates(ctx context.Context, announcements []announcementCandidate) []Candidate {
	if len(announcements) == 0 || ctx.Err() != nil {
		return nil
	}
	workerCount := min(maxValidationConcurrency, len(announcements))
	jobs := make(chan struct {
		index int
		value announcementCandidate
	})
	results := make(chan validationResult, len(announcements))
	client := newValidationClient(s.cfg.HTTPTimeout)

	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}
					candidate, err := validateCandidate(ctx, client, job.value.sourceIP, job.value.announcement, s.cfg.MaxHTTPBodyBytes)
					results <- validationResult{index: job.index, candidate: candidate, err: err}
				}
			}
		}()
	}

	for index, value := range announcements {
		select {
		case <-ctx.Done():
			break
		case jobs <- struct {
			index int
			value announcementCandidate
		}{index: index, value: value}:
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	workers.Wait()
	close(results)

	verified := make([]Candidate, len(announcements))
	valid := make([]bool, len(announcements))
	for result := range results {
		if result.err == nil {
			verified[result.index] = result.candidate
			valid[result.index] = true
		}
	}
	candidates := make([]Candidate, 0, len(announcements))
	for index, candidate := range verified {
		if valid[index] {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func validateCandidate(ctx context.Context, client *http.Client, sourceIP net.IP, announcement Announcement, maxBody int64) (Candidate, error) {
	ip := sourceIP.To4()
	if !isAllowedIPv4(ip) {
		return Candidate{}, errors.New("discovery response source is not an allowed IPv4 address")
	}
	origin := "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(announcement.HTTPPort))

	readyBody, status, err := fetchHTTPJSON(ctx, client, origin+"/ready", maxBody)
	if err != nil {
		return Candidate{}, err
	}
	if status != http.StatusOK {
		return Candidate{}, fmt.Errorf("peer readiness status %d", status)
	}
	var ready struct {
		Status  string `json:"status"`
		Message string `json:"message,omitempty"`
	}
	if err := decodeHTTPJSON(readyBody, &ready); err != nil || ready.Status != "ready" {
		if err == nil {
			err = errors.New("peer is not ready")
		}
		return Candidate{}, err
	}

	identityBody, status, err := fetchHTTPJSON(ctx, client, origin+"/api/v1/discovery/identity", maxBody)
	if err != nil {
		return Candidate{}, err
	}
	if status != http.StatusOK {
		return Candidate{}, fmt.Errorf("peer identity status %d", status)
	}
	var identity IdentityResponse
	if err := decodeHTTPJSON(identityBody, &identity); err != nil {
		return Candidate{}, err
	}
	if err := ValidateIdentityResponse(identity); err != nil {
		return Candidate{}, err
	}
	if identity.InstanceID != announcement.InstanceID {
		return Candidate{}, errors.New("peer identity instance_id does not match announcement")
	}
	return Candidate{
		Origin:       origin,
		SourceIP:     append(net.IP(nil), ip...),
		Announcement: announcement,
		Identity:     identity,
	}, nil
}

func newValidationClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:             nil,
			DisableKeepAlives: true,
			DialContext: (&net.Dialer{
				Timeout: timeout,
			}).DialContext,
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func fetchHTTPJSON(ctx context.Context, client *http.Client, url string, maxBody int64) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create peer validation request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("validate peer HTTP endpoint: %w", err)
	}
	defer response.Body.Close()
	if response.ContentLength > maxBody {
		return nil, response.StatusCode, errors.New("peer validation response is too large")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return nil, response.StatusCode, fmt.Errorf("read peer validation response: %w", err)
	}
	if int64(len(body)) > maxBody {
		return nil, response.StatusCode, errors.New("peer validation response is too large")
	}
	return body, response.StatusCode, nil
}

func decodeHTTPJSON(payload []byte, destination any) error {
	if err := rejectDuplicateJSONKeys(payload); err != nil {
		return fmt.Errorf("scan peer validation response: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode peer validation response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("peer validation response contains trailing JSON")
		}
		return fmt.Errorf("decode trailing peer validation response: %w", err)
	}
	return nil
}

func privateBroadcastTargets() ([]net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list discovery network interfaces: %w", err)
	}

	targets := make([]net.IP, 0, len(interfaces))
	seen := make(map[string]struct{})
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ip, network := ipv4Network(address)
			if !isAllowedIPv4(ip) {
				continue
			}
			var target net.IP
			if ip[0] == 127 {
				target = net.IPv4(127, 0, 0, 1).To4()
			} else if network != nil {
				target = make(net.IP, net.IPv4len)
				for index := range target {
					target[index] = ip[index] | ^network.Mask[index]
				}
			} else {
				target = append(net.IP(nil), ip...)
			}
			key := target.String()
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			targets = append(targets, target)
		}
	}
	return targets, nil
}

func ipv4Network(address net.Addr) (net.IP, *net.IPNet) {
	switch value := address.(type) {
	case *net.IPNet:
		ip := value.IP.To4()
		if ip == nil || len(value.Mask) != net.IPv4len {
			return nil, nil
		}
		return ip, &net.IPNet{IP: ip, Mask: value.Mask}
	case *net.IPAddr:
		return value.IP.To4(), nil
	default:
		return nil, nil
	}
}

func isAllowedIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	if ip[0] == 127 {
		return true
	}
	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
		(ip[0] == 192 && ip[1] == 168)
}
