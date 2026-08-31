package discovery

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestProtocolRoundTripAndStrictValidation(t *testing.T) {
	nonce := strings.Repeat("a", NonceBytes*2)
	payload, err := EncodeRequest(nonce)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	request, err := DecodeRequest(payload)
	if err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if request.Kind != "discover" || request.Protocol != ProtocolVersion || request.Nonce != nonce {
		t.Fatalf("unexpected request: %+v", request)
	}

	announcement := Announcement{
		Kind:       "announce",
		Protocol:   ProtocolVersion,
		Nonce:      nonce,
		Product:    Product,
		InstanceID: "11111111-1111-4111-8111-111111111111",
		ServerName: "测试服务器",
		HTTPPort:   8080,
	}
	encoded, err := EncodeAnnouncement(announcement)
	if err != nil {
		t.Fatalf("encode announcement: %v", err)
	}
	decoded, err := DecodeAnnouncement(encoded)
	if err != nil {
		t.Fatalf("decode announcement: %v", err)
	}
	if decoded != announcement {
		t.Fatalf("announcement round trip = %+v, want %+v", decoded, announcement)
	}

	tests := []struct {
		name    string
		payload string
	}{
		{name: "unknown field", payload: `{"kind":"discover","protocol":1,"nonce":"` + nonce + `","url":"http://192.168.1.1"}`},
		{name: "duplicate field", payload: `{"kind":"discover","protocol":1,"nonce":"` + nonce + `","kind":"discover"}`},
		{name: "trailing value", payload: `{"kind":"discover","protocol":1,"nonce":"` + nonce + `"} {}`},
		{name: "wrong nonce", payload: `{"kind":"discover","protocol":1,"nonce":"not-a-nonce"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeRequest([]byte(tt.payload))
			if !errors.Is(err, ErrInvalidPacket) {
				t.Fatalf("error = %v, want ErrInvalidPacket", err)
			}
		})
	}

	tooLarge := []byte(`{"kind":"discover","protocol":1,"nonce":"` + nonce + `","padding":"` + strings.Repeat("x", MaxPacketBytes) + `"}`)
	if _, err := DecodeRequest(tooLarge); !errors.Is(err, ErrInvalidPacket) {
		t.Fatalf("oversized packet error = %v, want ErrInvalidPacket", err)
	}
}

func TestProtocolRejectsAnnouncementBoundaryAndIdentityMismatch(t *testing.T) {
	nonce := strings.Repeat("0", NonceBytes*2)
	base := Announcement{Kind: "announce", Protocol: ProtocolVersion, Nonce: nonce, Product: Product, InstanceID: "11111111-1111-4111-8111-111111111111", ServerName: "server", HTTPPort: 8080}
	for _, port := range []int{0, 65536} {
		base.HTTPPort = port
		if _, err := EncodeAnnouncement(base); !errors.Is(err, ErrInvalidPacket) {
			t.Fatalf("port %d error = %v, want ErrInvalidPacket", port, err)
		}
	}
	if err := ValidateIdentityResponse(IdentityResponse{Product: "other", DiscoveryProtocol: ProtocolVersion, InstanceID: "11111111-1111-4111-8111-111111111111", ServerName: "server", ServerVersion: "1"}); !errors.Is(err, ErrInvalidPacket) {
		t.Fatalf("identity product error = %v, want ErrInvalidPacket", err)
	}

	validIdentity := IdentityResponse{
		Product:           Product,
		DiscoveryProtocol: ProtocolVersion,
		InstanceID:        "11111111-1111-4111-8111-111111111111",
		ServerName:        "server",
		ServerVersion:     "1",
	}
	for _, test := range []struct {
		name     string
		identity IdentityResponse
	}{
		{name: "server name too long", identity: IdentityResponse{Product: validIdentity.Product, DiscoveryProtocol: validIdentity.DiscoveryProtocol, InstanceID: validIdentity.InstanceID, ServerName: strings.Repeat("x", 121), ServerVersion: validIdentity.ServerVersion}},
		{name: "server version too long", identity: IdentityResponse{Product: validIdentity.Product, DiscoveryProtocol: validIdentity.DiscoveryProtocol, InstanceID: validIdentity.InstanceID, ServerName: validIdentity.ServerName, ServerVersion: strings.Repeat("x", 65)}},
		{name: "server name control", identity: IdentityResponse{Product: validIdentity.Product, DiscoveryProtocol: validIdentity.DiscoveryProtocol, InstanceID: validIdentity.InstanceID, ServerName: "server\nname", ServerVersion: validIdentity.ServerVersion}},
		{name: "server version control", identity: IdentityResponse{Product: validIdentity.Product, DiscoveryProtocol: validIdentity.DiscoveryProtocol, InstanceID: validIdentity.InstanceID, ServerName: validIdentity.ServerName, ServerVersion: "1.0\r"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateIdentityResponse(test.identity); !errors.Is(err, ErrInvalidPacket) {
				t.Fatalf("identity validation error = %v, want ErrInvalidPacket", err)
			}
		})
	}

	unknownField := []byte(`{"product":"bb-erp","discovery_protocol":1,"instance_id":"11111111-1111-4111-8111-111111111111","server_name":"server","server_version":"1","unexpected":true}`)
	if err := decodeHTTPJSON(unknownField, &IdentityResponse{}); err == nil {
		t.Fatal("identity response with unknown field was accepted")
	}

	// Keep the wire contract explicit so a future field cannot silently grow
	// the datagram beyond the firewall-safe 512-byte limit.
	encoded, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal base announcement: %v", err)
	}
	if len(encoded) >= MaxPacketBytes {
		t.Fatalf("base announcement unexpectedly uses %d bytes", len(encoded))
	}
}
