package server

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/limits"
)

// limitedListener applies the connection limits before the SSH handshake
// starts, so a refused connection costs one accept and one write.
type limitedListener struct {
	net.Listener
	lim      *limits.Limiter
	deadline time.Duration // accept to shell start; 0 disables
	onRefuse func(limits.Reason, net.Addr)
}

func (l *limitedListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		release, reason := l.lim.Acquire(c.RemoteAddr())
		if reason != limits.OK {
			if l.onRefuse != nil {
				l.onRefuse(reason, c.RemoteAddr())
			}
			refuse(c, reason)
			continue
		}
		return newTrackedConn(c, release, l.deadline), nil
	}
}

// refuse sends a short note and closes. SSH allows text lines before the
// version banner, and OpenSSH shows them with -v.
func refuse(c net.Conn, reason limits.Reason) {
	msg := "Too many connections from your address. Please try again in a minute.\r\n"
	if reason == limits.ServerFull || reason == limits.TrackingFull {
		msg = "The server is busy. Please try again in a few minutes.\r\n"
	}
	_ = c.SetWriteDeadline(time.Now().Add(time.Second))
	_, _ = io.WriteString(c, msg)
	_ = c.Close()
}

// trackedConn frees its limiter slot on Close and closes itself if no shell
// or exec starts within the deadline. That bounds slow handshakes and
// connections that only want port forwarding (ssh -N).
type trackedConn struct {
	net.Conn
	release  func()
	once     sync.Once
	timer    *time.Timer
	sessions atomic.Int32
}

func newTrackedConn(c net.Conn, release func(), deadline time.Duration) *trackedConn {
	tc := &trackedConn{Conn: c, release: release}
	if deadline > 0 {
		tc.timer = time.AfterFunc(deadline, func() { _ = tc.Close() })
	}
	return tc
}

func (c *trackedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() {
		if c.timer != nil {
			c.timer.Stop()
		}
		c.release()
	})
	return err
}

// started stops the no-session deadline.
func (c *trackedConn) started() {
	if c.timer != nil {
		c.timer.Stop()
	}
}

// claimSession allows one session channel per connection.
func (c *trackedConn) claimSession() bool { return c.sessions.Add(1) == 1 }
