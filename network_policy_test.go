package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/netip"
	"testing"
	"time"
)

func TestIsAllowedOutboundIP(t *testing.T) {
	for _, address := range []string{
		"1.1.1.1", "8.8.8.8", "9.9.9.9", "2606:4700:4700::1111",
	} {
		ip := netip.MustParseAddr(address)
		if !isAllowedOutboundIP(ip) {
			t.Errorf("expected public IP %s to be allowed", address)
		}
	}
	for _, address := range []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1",
		"100.64.0.1", "169.254.169.254", "0.0.0.0",
		"198.18.0.1", "192.0.2.1", "198.51.100.1", "203.0.113.1",
		"224.0.0.1", "255.255.255.255", "::1",
		"fe80::1", "fc00::123", "2001:db8::1",
		"::ffff:127.0.0.1",
	} {
		ip := netip.MustParseAddr(address)
		if isAllowedOutboundIP(ip) {
			t.Errorf("unsafe destination %s was allowed", address)
		}
	}
}

func TestStrictHTTPClientSettings(t *testing.T) {
	client := newAnyFlipHTTPClient()
	if client.Timeout <= 0 || client.Timeout > time.Minute {
		t.Fatalf("HTTP timeout is not bounded: %s", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.DialContext == nil || transport.Proxy != nil {
		t.Fatalf("HTTP transport does not enforce strict outbound dial policy")
	}
	if transport.TLSClientConfig == nil ||
		transport.TLSClientConfig.InsecureSkipVerify ||
		transport.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Fatal("TLS certificate verification is not enforced")
	}
}

func TestDialAnyFlipRejectsUnexpectedHostWithoutDNS(t *testing.T) {
	if _, err := dialAnyFlipContext(context.Background(), "tcp", "127.0.0.1:443"); err == nil {
		t.Fatal("unexpected private destination accepted")
	}
	if _, err := dialAnyFlipContext(context.Background(), "tcp", "evil.example:443"); err == nil {
		t.Fatal("unexpected hostname accepted")
	}
	if _, err := dialAnyFlipContext(context.Background(), "tcp", "online.anyflip.com:80"); err == nil {
		t.Fatal("outbound port 80 accepted")
	}
}

func TestRetryBackoff(t *testing.T) {
	tests := []struct {
		initial time.Duration
		attempt int
		want    time.Duration
	}{
		{time.Second, 1, time.Second},
		{time.Second, 2, 2 * time.Second},
		{time.Second, 3, 4 * time.Second},
		{time.Second, 7, time.Minute},
		{time.Minute, 9, time.Minute},
		{0, 1, 0},
		{time.Second, 0, 0},
	}
	for _, tt := range tests {
		if got := retryBackoff(tt.initial, tt.attempt); got != tt.want {
			t.Errorf("retryBackoff(%v, %d) = %v, want %v", tt.initial, tt.attempt, got, tt.want)
		}
	}
}
