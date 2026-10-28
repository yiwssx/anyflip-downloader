package main

import (
	"net"
	"sync"
	"time"
)

const (
	authMaxFailures   = 5
	authFailureWindow = time.Minute
	authLockout       = 5 * time.Minute
	authMaxPeers      = 1024
)

// authThrottle limits consecutive failed token checks per TCP peer. It never
// trusts request-supplied forwarding headers, which are attacker-controlled
// without an explicitly configured and trusted reverse proxy.
type authThrottle struct {
	mu    sync.Mutex
	peers map[string]authPeerFailures
	clock func() time.Time
}

type authPeerFailures struct {
	count        int
	firstFailure time.Time
	lastFailure  time.Time
	blockedUntil time.Time
}

func newAuthThrottle() *authThrottle {
	return &authThrottle{
		peers: make(map[string]authPeerFailures),
		clock: time.Now,
	}
}

func authPeer(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil || host == "" {
		return "unknown"
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return host
}

func (a *authThrottle) retryAfter(remote string) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := authPeer(remote)
	entry, ok := a.peers[key]
	if !ok {
		return 0
	}
	now := a.clock()
	if entry.blockedUntil.After(now) {
		return entry.blockedUntil.Sub(now)
	}
	if !entry.blockedUntil.IsZero() || now.Sub(entry.firstFailure) >= authFailureWindow {
		delete(a.peers, key)
	}
	return 0
}

func (a *authThrottle) recordFailure(remote string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := authPeer(remote)
	now := a.clock()
	entry, exists := a.peers[key]
	if exists && entry.blockedUntil.After(now) {
		return
	}
	if !exists || !entry.blockedUntil.IsZero() || now.Sub(entry.firstFailure) >= authFailureWindow {
		if !exists && len(a.peers) >= authMaxPeers {
			// Keep memory bounded even under spoofed or extremely diverse
			// peer traffic. An upstream proxy/WAF must enforce its own limits.
			var oldest string
			var earliest time.Time
			for peer, prior := range a.peers {
				if earliest.IsZero() || prior.lastFailure.Before(earliest) {
					oldest, earliest = peer, prior.lastFailure
				}
			}
			delete(a.peers, oldest)
		}
		entry = authPeerFailures{firstFailure: now}
	}
	entry.count++
	entry.lastFailure = now
	if entry.count >= authMaxFailures {
		entry.blockedUntil = now.Add(authLockout)
	}
	a.peers[key] = entry
}

func (a *authThrottle) recordSuccess(remote string) {
	a.mu.Lock()
	delete(a.peers, authPeer(remote))
	a.mu.Unlock()
}
