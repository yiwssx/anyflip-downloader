package main

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// isAllowedOutboundIP enforces a public Internet destination, including
// IPv4-mapped IPv6. netip.IsGlobalUnicast alone allows RFC1918/CGNAT IPs.
func isAllowedOutboundIP(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() ||
		address.IsPrivate() || address.IsLoopback() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

// dialAnyFlipContext resolves the fixed AnyFlip hostname, checks every
// candidate IP, and dials an approved IP directly. The HTTP transport
// still verifies the original hostname during its TLS handshake.
func dialAnyFlipContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid destination: %w", err)
	}
	if !strings.EqualFold(host, "online.anyflip.com") || port != "443" {
		return nil, fmt.Errorf("outbound connection blocked: unexpected destination")
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve AnyFlip hostname: %w", err)
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	var lastErr error
	for _, ip := range ips {
		if !isAllowedOutboundIP(ip) {
			continue
		}
		connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return connection, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, fmt.Errorf("cannot connect to public AnyFlip address: %w", lastErr)
	}
	return nil, fmt.Errorf("AnyFlip DNS did not return an approved public IP address")
}
