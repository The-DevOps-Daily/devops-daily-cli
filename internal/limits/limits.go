// Package limits decides whether a new TCP connection may proceed. It
// enforces a per-address connection rate and caps on concurrent connections
// per address and in total. IPv6 addresses are grouped by /64, since one
// host usually controls a whole /64.
package limits

import (
	"net"
	"net/netip"
	"sync"
	"time"
)

// Config holds the limits. Zero values fall back to the defaults below.
type Config struct {
	RatePerIP  int           // new connections allowed per address in Window
	Window     time.Duration // the rate window
	MaxTotal   int           // concurrent connections overall
	MaxPerIP   int           // concurrent connections per address
	MaxTracked int           // addresses tracked at once; new ones are refused when full
}

// Defaults used for zero fields.
const (
	DefaultRatePerIP  = 10
	DefaultWindow     = time.Minute
	DefaultMaxTotal   = 200
	DefaultMaxPerIP   = 5
	DefaultMaxTracked = 50000
)

// Reason says why a connection was refused. The empty Reason means allowed.
type Reason string

// Refusal reasons.
const (
	OK           Reason = ""
	ServerFull   Reason = "server full"
	TooManyOpen  Reason = "too many open connections from this address"
	RateLimited  Reason = "connection rate limit"
	TrackingFull Reason = "address table full"
)

type entry struct {
	active int
	stamps []time.Time // accepted connections inside the window, oldest first
}

// Limiter is safe for concurrent use.
type Limiter struct {
	cfg       Config
	now       func() time.Time
	mu        sync.Mutex
	total     int
	ips       map[netip.Prefix]*entry
	lastPrune time.Time
}

// New returns a Limiter for cfg.
func New(cfg Config) *Limiter {
	if cfg.RatePerIP <= 0 {
		cfg.RatePerIP = DefaultRatePerIP
	}
	if cfg.Window <= 0 {
		cfg.Window = DefaultWindow
	}
	if cfg.MaxTotal <= 0 {
		cfg.MaxTotal = DefaultMaxTotal
	}
	if cfg.MaxPerIP <= 0 {
		cfg.MaxPerIP = DefaultMaxPerIP
	}
	if cfg.MaxTracked <= 0 {
		cfg.MaxTracked = DefaultMaxTracked
	}
	return &Limiter{cfg: cfg, now: time.Now, ips: make(map[netip.Prefix]*entry)}
}

// Config returns the effective limits.
func (l *Limiter) Config() Config { return l.cfg }

// Key returns the bucket for a remote address: the address itself for IPv4
// and the /64 for IPv6.
func Key(addr net.Addr) netip.Prefix {
	var ip netip.Addr
	switch a := addr.(type) {
	case *net.TCPAddr:
		ip, _ = netip.AddrFromSlice(a.IP)
	default:
		if ap, err := netip.ParseAddrPort(addr.String()); err == nil {
			ip = ap.Addr()
		}
	}
	ip = ip.Unmap()
	if !ip.IsValid() {
		return netip.Prefix{}
	}
	if ip.Is4() {
		return netip.PrefixFrom(ip, 32)
	}
	p, _ := ip.Prefix(64)
	return p
}

// Acquire checks the limits for a new connection from addr. When it is
// allowed, the returned release func must be called once the connection
// closes; calling it more than once is harmless. Refused attempts do not
// count toward the rate.
func (l *Limiter) Acquire(addr net.Addr) (release func(), reason Reason) {
	key := Key(addr)
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastPrune) >= l.cfg.Window {
		l.pruneLocked(now)
	}
	if l.total >= l.cfg.MaxTotal {
		return nil, ServerFull
	}
	e := l.ips[key]
	if e == nil {
		if len(l.ips) >= l.cfg.MaxTracked {
			l.pruneLocked(now)
			if len(l.ips) >= l.cfg.MaxTracked {
				return nil, TrackingFull
			}
		}
		e = &entry{}
		l.ips[key] = e
	}
	e.trim(now.Add(-l.cfg.Window))
	if e.active >= l.cfg.MaxPerIP {
		return nil, TooManyOpen
	}
	if len(e.stamps) >= l.cfg.RatePerIP {
		return nil, RateLimited
	}
	e.active++
	e.stamps = append(e.stamps, now)
	l.total++

	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			e.active--
			l.total--
			l.mu.Unlock()
		})
	}, OK
}

// Active returns the number of open connections.
func (l *Limiter) Active() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.total
}

// Tracked returns the number of addresses in the table.
func (l *Limiter) Tracked() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.ips)
}

func (e *entry) trim(cutoff time.Time) {
	i := 0
	for i < len(e.stamps) && !e.stamps[i].After(cutoff) {
		i++
	}
	if i > 0 {
		e.stamps = append(e.stamps[:0], e.stamps[i:]...)
	}
}

// pruneLocked drops addresses with no open connections and no recent ones.
func (l *Limiter) pruneLocked(now time.Time) {
	cutoff := now.Add(-l.cfg.Window)
	for k, e := range l.ips {
		e.trim(cutoff)
		if e.active == 0 && len(e.stamps) == 0 {
			delete(l.ips, k)
		}
	}
	l.lastPrune = now
}
