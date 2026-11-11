package limits

import (
	"fmt"
	"net"
	"net/netip"
	"testing"
	"time"
)

func addr(s string) net.Addr {
	return net.TCPAddrFromAddrPort(netip.MustParseAddrPort(s))
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newTest(cfg Config) (*Limiter, *clock) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	l := New(cfg)
	l.now = c.now
	return l, c
}

func TestEleventhConnectionInAMinuteIsRefused(t *testing.T) {
	l, c := newTest(Config{RatePerIP: 10, Window: time.Minute, MaxPerIP: 100})
	for i := 1; i <= 10; i++ {
		release, r := l.Acquire(addr("203.0.113.7:4000"))
		if r != OK {
			t.Fatalf("connection %d refused: %s", i, r)
		}
		release()
		c.t = c.t.Add(time.Second)
	}
	if _, r := l.Acquire(addr("203.0.113.7:4001")); r != RateLimited {
		t.Fatalf("11th connection: got %q, want %q", r, RateLimited)
	}
	// Other addresses have their own budget.
	if _, r := l.Acquire(addr("198.51.100.1:4000")); r != OK {
		t.Fatalf("other address refused: %s", r)
	}
	// Refused attempts do not extend the block: once the first stamp leaves
	// the window, one more connection is allowed.
	c.t = time.Unix(1_800_000_000, 0).Add(time.Minute + time.Millisecond)
	if _, r := l.Acquire(addr("203.0.113.7:4002")); r != OK {
		t.Fatalf("after the window: %s", r)
	}
	if _, r := l.Acquire(addr("203.0.113.7:4003")); r != RateLimited {
		t.Fatalf("second one after the window: got %q", r)
	}
}

func TestConcurrentPerIP(t *testing.T) {
	l, _ := newTest(Config{RatePerIP: 100, MaxPerIP: 5})
	var releases []func()
	for i := 0; i < 5; i++ {
		rel, r := l.Acquire(addr("203.0.113.9:1000"))
		if r != OK {
			t.Fatalf("connection %d refused: %s", i+1, r)
		}
		releases = append(releases, rel)
	}
	if _, r := l.Acquire(addr("203.0.113.9:1000")); r != TooManyOpen {
		t.Fatalf("6th open connection: got %q, want %q", r, TooManyOpen)
	}
	releases[0]()
	releases[0]() // a second call must not free another slot
	if _, r := l.Acquire(addr("203.0.113.9:1000")); r != OK {
		t.Fatalf("after one closed: %s", r)
	}
	if _, r := l.Acquire(addr("203.0.113.9:1000")); r != TooManyOpen {
		t.Fatalf("double release freed a slot: %q", r)
	}
}

func TestMaxTotal(t *testing.T) {
	l, _ := newTest(Config{RatePerIP: 100, MaxPerIP: 5, MaxTotal: 200})
	for i := 0; i < 200; i++ {
		// 5 connections from each of 40 addresses
		a := addr(fmt.Sprintf("10.0.0.%d:%d", i/5+1, 1000+i))
		if _, r := l.Acquire(a); r != OK {
			t.Fatalf("connection %d refused: %s", i+1, r)
		}
	}
	if got := l.Active(); got != 200 {
		t.Fatalf("Active = %d", got)
	}
	if _, r := l.Acquire(addr("192.0.2.1:1")); r != ServerFull {
		t.Fatalf("201st connection: got %q, want %q", r, ServerFull)
	}
}

func TestIPv6GroupedBy64(t *testing.T) {
	l, _ := newTest(Config{RatePerIP: 100, MaxPerIP: 2})
	for _, a := range []string{"[2001:db8::1]:1", "[2001:db8::ffff:2]:1"} {
		if _, r := l.Acquire(addr(a)); r != OK {
			t.Fatalf("%s refused: %s", a, r)
		}
	}
	if _, r := l.Acquire(addr("[2001:db8::3]:1")); r != TooManyOpen {
		t.Fatalf("same /64: got %q, want %q", r, TooManyOpen)
	}
	if _, r := l.Acquire(addr("[2001:db8:0:1::1]:1")); r != OK {
		t.Fatalf("other /64 refused: %s", r)
	}
}

func TestIPv4MappedIsSameAsIPv4(t *testing.T) {
	a := Key(addr("[::ffff:203.0.113.7]:22"))
	b := Key(addr("203.0.113.7:22"))
	if a != b {
		t.Fatalf("mapped %v != plain %v", a, b)
	}
}

func TestTableFullThenPruned(t *testing.T) {
	l, c := newTest(Config{RatePerIP: 10, MaxTracked: 3, Window: time.Minute})
	for i := 1; i <= 3; i++ {
		rel, r := l.Acquire(addr(fmt.Sprintf("198.51.100.%d:1", i)))
		if r != OK {
			t.Fatalf("address %d refused: %s", i, r)
		}
		rel()
	}
	if _, r := l.Acquire(addr("198.51.100.9:1")); r != TrackingFull {
		t.Fatalf("4th address: got %q, want %q", r, TrackingFull)
	}
	c.t = c.t.Add(2 * time.Minute)
	if _, r := l.Acquire(addr("198.51.100.9:1")); r != OK {
		t.Fatalf("after prune: %s", r)
	}
	if got := l.Tracked(); got != 1 {
		t.Fatalf("Tracked = %d, want 1", got)
	}
}
