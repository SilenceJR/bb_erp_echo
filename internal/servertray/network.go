package servertray

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"time"
)

func PrivateIPv4() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list network interfaces: %w", err)
	}
	seen := make(map[string]struct{})
	addresses := make([]string, 0, len(interfaces))
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		ifaceAddresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range ifaceAddresses {
			var ip net.IP
			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP.To4()
			case *net.IPAddr:
				ip = value.IP.To4()
			}
			if !isPrivateIPv4(ip) {
				continue
			}
			text := ip.String()
			if _, ok := seen[text]; ok {
				continue
			}
			seen[text] = struct{}{}
			addresses = append(addresses, text)
		}
	}
	sort.Slice(addresses, func(i, j int) bool {
		return bytesLess(net.ParseIP(addresses[i]).To4(), net.ParseIP(addresses[j]).To4())
	})
	return addresses, nil
}

func isPrivateIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil || ip[0] == 127 {
		return false
	}
	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
		(ip[0] == 192 && ip[1] == 168)
}

func bytesLess(left, right net.IP) bool {
	for index := 0; index < net.IPv4len; index++ {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}

func WaitReady(ctx context.Context, port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid HTTP port %d", port)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/ready", port)
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			var payload struct {
				Status string `json:"status"`
			}
			decodeErr := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&payload)
			closeErr := response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && payload.Status == "ready" && closeErr == nil {
				return nil
			}
			lastErr = fmt.Errorf("ready endpoint returned status %d with state %q", response.StatusCode, payload.Status)
			if decodeErr != nil {
				lastErr = fmt.Errorf("decode ready response: %w", decodeErr)
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), lastErr)
		case <-ticker.C:
		}
	}
}
