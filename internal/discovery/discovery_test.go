package discovery

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestResponderAnswersStrictRequestAndRateLimitsSource(t *testing.T) {
	port := freeUDPPort(t)
	identity := Identity{InstanceID: "22222222-2222-4222-8222-222222222222", Product: Product, DiscoveryProtocol: ProtocolVersion, ServerName: "测试服务", ServerVersion: "1.0.0"}
	responder := NewResponder(ResponderConfig{
		BindHost:          "127.0.0.1",
		Port:              port,
		HTTPPort:          8080,
		RateLimitInterval: time.Hour,
		RateLimitEntries:  2,
	}, identity)
	serverConn, err := responder.Listen()
	if err != nil {
		t.Fatalf("listen responder: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- responder.ServeConn(ctx, serverConn) }()

	clientConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("listen client: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		_ = clientConn.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("responder shutdown: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("responder did not stop")
		}
	})

	nonce := strings.Repeat("b", NonceBytes*2)
	payload, err := EncodeRequest(nonce)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	target := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}
	if _, err := clientConn.WriteToUDP(payload, target); err != nil {
		t.Fatalf("send first request: %v", err)
	}
	buffer := make([]byte, MaxPacketBytes+1)
	if err := clientConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	n, _, err := clientConn.ReadFromUDP(buffer)
	if err != nil {
		t.Fatalf("read first response: %v", err)
	}
	announcement, err := DecodeAnnouncement(buffer[:n])
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if announcement.Nonce != nonce || announcement.InstanceID != identity.InstanceID || announcement.HTTPPort != 8080 {
		t.Fatalf("unexpected announcement: %+v", announcement)
	}

	if _, err := clientConn.WriteToUDP(payload, target); err != nil {
		t.Fatalf("send rate-limited request: %v", err)
	}
	if err := clientConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("set rate-limit deadline: %v", err)
	}
	if _, _, err := clientConn.ReadFromUDP(buffer); err == nil {
		t.Fatal("rate-limited request unexpectedly received a response")
	}
}

func TestScannerValidatesReadyAndIdentityUsingUDPSource(t *testing.T) {
	identity := IdentityResponse{
		Product:           Product,
		DiscoveryProtocol: ProtocolVersion,
		InstanceID:        "33333333-3333-4333-8333-333333333333",
		ServerName:        "局域网服务",
		ServerVersion:     "1.0.0",
	}
	httpListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen HTTP test server: %v", err)
	}
	httpServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ready" {
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
			return
		}
		if r.URL.Path == "/api/v1/discovery/identity" {
			_ = json.NewEncoder(w).Encode(identity)
			return
		}
		http.NotFound(w, r)
	}))
	httpServer.Listener = httpListener
	httpServer.Start()
	t.Cleanup(httpServer.Close)
	httpPort := httpListener.Addr().(*net.TCPAddr).Port

	udpPort := freeUDPPort(t)
	responder := NewResponder(ResponderConfig{BindHost: "127.0.0.1", Port: udpPort, HTTPPort: httpPort}, Identity{
		InstanceID:        identity.InstanceID,
		Product:           Product,
		DiscoveryProtocol: ProtocolVersion,
		ServerName:        identity.ServerName,
		ServerVersion:     identity.ServerVersion,
	})
	udpConn, err := responder.Listen()
	if err != nil {
		t.Fatalf("listen UDP responder: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- responder.ServeConn(ctx, udpConn) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("responder shutdown: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("responder did not stop")
		}
	})

	scanner := NewScanner(ScannerConfig{Port: udpPort, ScanTimeout: 300 * time.Millisecond, HTTPTimeout: time.Second})
	candidates, err := scanner.ScanTargets(context.Background(), []net.IP{net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("scan targets: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(candidates))
	}
	if candidates[0].Origin != "http://127.0.0.1:"+strconv.Itoa(httpPort) {
		t.Fatalf("candidate origin = %q", candidates[0].Origin)
	}
	if candidates[0].Identity != identity {
		t.Fatalf("candidate identity = %+v, want %+v", candidates[0].Identity, identity)
	}
}

func TestScannerIgnoresIdentityMismatchAndRedirect(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen HTTP test server: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ready" {
			http.Redirect(w, r, "/ready-ok", http.StatusTemporaryRedirect)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}))
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	port := listener.Addr().(*net.TCPAddr).Port
	udpPort := freeUDPPort(t)
	responder := NewResponder(ResponderConfig{BindHost: "127.0.0.1", Port: udpPort, HTTPPort: port}, Identity{
		InstanceID: "44444444-4444-4444-8444-444444444444", Product: Product, DiscoveryProtocol: ProtocolVersion, ServerName: "server", ServerVersion: "1",
	})
	conn, err := responder.Listen()
	if err != nil {
		t.Fatalf("listen UDP responder: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- responder.ServeConn(ctx, conn) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("responder did not stop")
		}
	})

	scanner := NewScanner(ScannerConfig{Port: udpPort, ScanTimeout: 100 * time.Millisecond, HTTPTimeout: 100 * time.Millisecond})
	candidates, err := scanner.ScanTargets(context.Background(), []net.IP{net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("scan targets: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("redirect/malformed peer candidates = %d, want 0", len(candidates))
	}
}

func TestScannerValidatesCollectedCandidatesConcurrentlyWithinGlobalDeadline(t *testing.T) {
	identity := IdentityResponse{
		Product:           Product,
		DiscoveryProtocol: ProtocolVersion,
		InstanceID:        "55555555-5555-4555-8555-555555555555",
		ServerName:        "快速服务",
		ServerVersion:     "1.0.0",
	}

	slowServer := newDiscoveryHTTPServer(t, identity, 500*time.Millisecond)
	fastIdentity := identity
	fastIdentity.InstanceID = "66666666-6666-4666-8666-666666666666"
	fastIdentity.ServerName = "快速响应服务"
	fastServer := newDiscoveryHTTPServer(t, fastIdentity, 0)

	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("listen fake discovery UDP: %v", err)
	}
	defer udpConn.Close()
	go func() {
		buffer := make([]byte, MaxPacketBytes+1)
		n, source, err := udpConn.ReadFromUDP(buffer)
		if err != nil {
			return
		}
		request, err := DecodeRequest(buffer[:n])
		if err != nil {
			return
		}
		for _, peer := range []struct {
			identity IdentityResponse
			port     int
		}{
			{identity: identity, port: slowServer.Port},
			{identity: fastIdentity, port: fastServer.Port},
		} {
			payload, err := EncodeAnnouncement(Announcement{
				Kind:       "announce",
				Protocol:   ProtocolVersion,
				Nonce:      request.Nonce,
				Product:    Product,
				InstanceID: peer.identity.InstanceID,
				ServerName: peer.identity.ServerName,
				HTTPPort:   peer.port,
			})
			if err != nil {
				return
			}
			_, _ = udpConn.WriteToUDP(payload, source)
		}
	}()

	scanner := NewScanner(ScannerConfig{
		Port:             udpConn.LocalAddr().(*net.UDPAddr).Port,
		ScanTimeout:      20 * time.Millisecond,
		PreflightTimeout: 120 * time.Millisecond,
		HTTPTimeout:      time.Second,
	})
	candidates, err := scanner.ScanTargets(context.Background(), []net.IP{net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("scan targets: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want one fast peer despite slow peer", len(candidates))
	}
	if candidates[0].Identity.InstanceID != fastIdentity.InstanceID {
		t.Fatalf("candidate identity = %+v, want fast peer %+v", candidates[0].Identity, fastIdentity)
	}
}

func TestServiceStartAndConcurrentShutdownCanRepeat(t *testing.T) {
	service := NewService(ServiceConfig{
		Enabled:  true,
		BindHost: "127.0.0.1",
		Port:     freeUDPPort(t),
	}, Identity{
		InstanceID:        "77777777-7777-4777-8777-777777777777",
		Product:           Product,
		DiscoveryProtocol: ProtocolVersion,
		ServerName:        "生命周期测试",
		ServerVersion:     "1.0.0",
	})

	for iteration := 0; iteration < 50; iteration++ {
		if err := service.Start(context.Background()); err != nil {
			t.Fatalf("iteration %d start: %v", iteration, err)
		}
		var shutdowns sync.WaitGroup
		for caller := 0; caller < 8; caller++ {
			shutdowns.Add(1)
			go func() {
				defer shutdowns.Done()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := service.Shutdown(ctx); err != nil {
					t.Errorf("concurrent shutdown: %v", err)
				}
			}()
		}
		shutdowns.Wait()
	}
}

type discoveryHTTPServer struct {
	Port int
}

func newDiscoveryHTTPServer(t *testing.T, identity IdentityResponse, readyDelay time.Duration) discoveryHTTPServer {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen discovery HTTP server: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ready":
			if readyDelay > 0 {
				time.Sleep(readyDelay)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
		case "/api/v1/discovery/identity":
			_ = json.NewEncoder(w).Encode(identity)
		default:
			http.NotFound(w, r)
		}
	}))
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return discoveryHTTPServer{Port: listener.Addr().(*net.TCPAddr).Port}
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("allocate UDP port: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	if err := conn.Close(); err != nil {
		t.Fatalf("release UDP port: %v", err)
	}
	return port
}
