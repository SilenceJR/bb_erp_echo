package servertray

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestIsPrivateIPv4(t *testing.T) {
	tests := map[string]bool{
		"10.0.0.1": true, "172.16.0.1": true, "172.31.255.254": true, "192.168.1.1": true,
		"127.0.0.1": false, "172.15.1.1": false, "172.32.1.1": false, "8.8.8.8": false, "::1": false,
	}
	for raw, expected := range tests {
		if actual := isPrivateIPv4(net.ParseIP(raw)); actual != expected {
			t.Errorf("isPrivateIPv4(%q) = %v, want %v", raw, actual, expected)
		}
	}
}

func TestWaitReady(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("listen test server: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/ready" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		fmt.Fprint(response, `{"status":"ready"}`)
	})}
	go server.Serve(listener)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := WaitReady(ctx, listener.Addr().(*net.TCPAddr).Port); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
}

func TestWaitReadyRejectsInvalidPort(t *testing.T) {
	if err := WaitReady(context.Background(), 0); err == nil {
		t.Fatal("expected invalid port error")
	}
}
