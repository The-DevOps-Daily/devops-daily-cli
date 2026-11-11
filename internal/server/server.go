// Package server runs the SSH side: a Wish server that only ever starts the
// menu app. There is no shell, no exec, no forwarding of any kind, no SFTP
// and no environment passing.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	bm "github.com/charmbracelet/wish/bubbletea"
	"github.com/muesli/termenv"
	gossh "golang.org/x/crypto/ssh"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/limits"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/text"
)

// AppFunc builds the Bubble Tea model for one session. The app calls onQuit
// with a reason (idle, time limit) that is printed after the screen clears.
type AppFunc func(r *lipgloss.Renderer, onQuit func(reason string)) tea.Model

// Config for New.
type Config struct {
	HostKeyPath     string
	Limits          limits.Config
	IdleTimeout     time.Duration // the app quits after this long without a key press
	MaxSession      time.Duration // the app quits after this long in total
	SessionDeadline time.Duration // a shell must start this soon after accept; default 30s
	App             AppFunc
	Logger          *slog.Logger
	Version         string // software version in the SSH banner
}

// Server is the SSH server.
type Server struct {
	cfg      Config
	srv      *ssh.Server
	lim      *limits.Limiter
	log      *slog.Logger
	signer   gossh.Signer
	sessions atomic.Int64
	refused  atomic.Int64

	refMu  sync.Mutex
	refLog map[string]time.Time
}

type connKey struct{}
type infoKey struct{}

// Window sizes are clamped to this before the app sees them.
const (
	maxCols = 500
	maxRows = 200
)

// New loads (or creates) the host key and builds the server.
func New(cfg Config) (*Server, error) {
	if cfg.App == nil {
		return nil, errors.New("server: App is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.SessionDeadline == 0 {
		cfg.SessionDeadline = 30 * time.Second
	}
	if cfg.Version == "" {
		cfg.Version = "DevOpsDaily"
	}
	signer, created, err := LoadOrCreateHostKey(cfg.HostKeyPath)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, lim: limits.New(cfg.Limits), log: cfg.Logger, signer: signer, refLog: map[string]time.Time{}}
	if created {
		s.log.Info("generated a new host key", "path", cfg.HostKeyPath)
	}
	srv, err := wish.NewServer(
		func(srv *ssh.Server) error { srv.AddHostKey(signer); return nil },
		// The last middleware runs first: guard, then the app, then goodbye.
		wish.WithMiddleware(
			s.goodbye,
			bm.Middleware(s.teaHandler),
			s.guard,
		),
		s.harden,
	)
	if err != nil {
		return nil, err
	}
	s.srv = srv
	return s, nil
}

// Fingerprint is the host key's SHA256 fingerprint.
func (s *Server) Fingerprint() string { return Fingerprint(s.signer) }

// harden turns off everything except one interactive session.
func (s *Server) harden(srv *ssh.Server) error {
	srv.Version = s.cfg.Version
	// Any key or none: public key and keyboard-interactive auth always
	// succeed, with no prompt. Keys are not stored or logged.
	srv.PublicKeyHandler = func(ssh.Context, ssh.PublicKey) bool { return true }
	srv.KeyboardInteractiveHandler = func(ssh.Context, gossh.KeyboardInteractiveChallenge) bool { return true }
	srv.PasswordHandler = nil
	// The app enforces the visible limits; these are the hard backstops.
	if s.cfg.IdleTimeout > 0 {
		srv.IdleTimeout = s.cfg.IdleTimeout + time.Minute
	}
	if s.cfg.MaxSession > 0 {
		srv.MaxTimeout = s.cfg.MaxSession + time.Minute
	}
	// Only "session" channels: direct-tcpip (ssh -L/-D) and every other
	// channel type is rejected. No global request handlers, so tcpip-forward
	// (ssh -R) is refused. No subsystems, so no SFTP.
	srv.ChannelHandlers = map[string]ssh.ChannelHandler{"session": s.sessionChannel}
	srv.RequestHandlers = map[string]ssh.RequestHandler{}
	srv.SubsystemHandlers = map[string]ssh.SubsystemHandler{}
	srv.LocalPortForwardingCallback = nil
	srv.ReversePortForwardingCallback = nil
	srv.ConnCallback = s.connCallback
	return nil
}

func (s *Server) connCallback(ctx ssh.Context, c net.Conn) net.Conn {
	if tc, ok := c.(*trackedConn); ok {
		ctx.SetValue(connKey{}, tc)
	}
	return c
}

func (s *Server) sessionChannel(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	if tc, ok := ctx.Value(connKey{}).(*trackedConn); ok && !tc.claimSession() {
		_ = newChan.Reject(gossh.Prohibited, "one session per connection")
		return
	}
	ssh.DefaultSessionHandler(srv, conn, filteredChannel{newChan}, ctx)
}

// allowedRequests are the only channel requests that reach the session
// handler. env, x11-req, auth-agent-req@openssh.com, subsystem (sftp),
// signal and anything else get a "no".
var allowedRequests = map[string]bool{
	"pty-req":       true,
	"window-change": true,
	"shell":         true,
	"exec":          true, // reaches guard, which refuses it with a message
}

type filteredChannel struct{ gossh.NewChannel }

func (f filteredChannel) Accept() (gossh.Channel, <-chan *gossh.Request, error) {
	ch, reqs, err := f.NewChannel.Accept()
	if err != nil {
		return nil, nil, err
	}
	out := make(chan *gossh.Request)
	go func() {
		defer close(out)
		for req := range reqs {
			if allowedRequests[req.Type] {
				out <- req
				continue
			}
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}()
	return ch, out, nil
}

type sessionInfo struct {
	mu     sync.Mutex
	reason string
}

func (i *sessionInfo) set(r string) {
	if i == nil {
		return
	}
	i.mu.Lock()
	i.reason = r
	i.mu.Unlock()
}

func (i *sessionInfo) get() string {
	if i == nil {
		return ""
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.reason
}

// guard refuses commands and sessions without a terminal, then runs the app
// with the window size clamped.
func (s *Server) guard(next ssh.Handler) ssh.Handler {
	return func(sess ssh.Session) {
		if tc, ok := sess.Context().Value(connKey{}).(*trackedConn); ok {
			tc.started()
		}
		if sess.RawCommand() != "" || len(sess.Command()) > 0 {
			fmt.Fprint(sess.Stderr(), "This is the DevOps Daily menu app, not a shell, so commands are not run.\nConnect with: ssh ssh.devops-daily.com\n")
			_ = sess.Exit(1)
			s.log.Info("command refused")
			return
		}
		pty, _, ok := sess.Pty()
		if !ok {
			fmt.Fprint(sess.Stderr(), "This app needs an interactive terminal. Connect with: ssh -t ssh.devops-daily.com\n")
			_ = sess.Exit(1)
			s.log.Info("session without a terminal refused")
			return
		}
		info := &sessionInfo{}
		sess.Context().SetValue(infoKey{}, info)
		start := time.Now()
		n := s.sessions.Add(1)
		s.log.Info("session start", "active", n, "term", text.Line(pty.Term, 32), "cols", pty.Window.Width, "rows", pty.Window.Height)
		next(newClampedSession(sess))
		s.sessions.Add(-1)
		s.log.Info("session end", "seconds", int(time.Since(start).Seconds()), "reason", info.get())
	}
}

func (s *Server) teaHandler(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
	info, _ := sess.Context().Value(infoKey{}).(*sessionInfo)
	return s.cfg.App(newRenderer(sess), info.set), append(bm.MakeOptions(sess), tea.WithAltScreen())
}

type termEnv []string

func (e termEnv) Environ() []string { return e }

func (e termEnv) Getenv(k string) string {
	for _, kv := range e {
		if v, ok := strings.CutPrefix(kv, k+"="); ok {
			return v
		}
	}
	return ""
}

// newRenderer picks colors from TERM alone. Unlike wish's MakeRenderer it
// never queries the client terminal, which delays startup by up to a second
// and swallows keys typed in that time.
func newRenderer(sess ssh.Session) *lipgloss.Renderer {
	pty, _, _ := sess.Pty()
	r := lipgloss.NewRenderer(sess, termenv.WithEnvironment(termEnv{"TERM=" + text.Line(pty.Term, 64)}), termenv.WithUnsafe())
	r.SetHasDarkBackground(true)
	if r.ColorProfile() == termenv.ANSI {
		r.SetColorProfile(termenv.ANSI256) // nearly every terminal has 256 colors
	}
	return r
}

// goodbye prints after the app has left the alternate screen.
func (s *Server) goodbye(next ssh.Handler) ssh.Handler {
	return func(sess ssh.Session) {
		info, _ := sess.Context().Value(infoKey{}).(*sessionInfo)
		if r := info.get(); r != "" {
			fmt.Fprintf(sess, "%s\r\n", r)
		}
		fmt.Fprint(sess, "Thanks for visiting DevOps Daily: https://devops-daily.com\r\n")
		next(sess)
	}
}

// clampedSession hands the app a window size within bounds and drops
// window-change floods down to the latest size.
type clampedSession struct {
	ssh.Session
	pty ssh.Pty
	win chan ssh.Window
}

func clampWindow(w ssh.Window) ssh.Window {
	if w.Width <= 0 {
		w.Width = 80
	}
	if w.Height <= 0 {
		w.Height = 24
	}
	w.Width, w.Height = min(w.Width, maxCols), min(w.Height, maxRows)
	w.WidthPixels, w.HeightPixels = 0, 0
	return w
}

func newClampedSession(sess ssh.Session) ssh.Session {
	pty, winCh, ok := sess.Pty()
	if !ok {
		return sess
	}
	pty.Window = clampWindow(pty.Window)
	c := &clampedSession{Session: sess, pty: pty, win: make(chan ssh.Window, 1)}
	ctx := sess.Context()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case w, ok := <-winCh:
				if !ok {
					return
				}
				w = clampWindow(w)
				select {
				case c.win <- w:
				default: // replace the pending size with the newest one
					select {
					case <-c.win:
					default:
					}
					select {
					case c.win <- w:
					default:
					}
				}
			}
		}
	}()
	return c
}

func (c *clampedSession) Pty() (ssh.Pty, <-chan ssh.Window, bool) { return c.pty, c.win, true }

func (s *Server) onRefuse(reason limits.Reason, addr net.Addr) {
	s.refused.Add(1)
	key := limits.Key(addr).String()
	now := time.Now()
	s.refMu.Lock()
	if len(s.refLog) > 1000 {
		s.refLog = map[string]time.Time{}
	}
	last, seen := s.refLog[key]
	logIt := !seen || now.Sub(last) > time.Minute
	if logIt {
		s.refLog[key] = now
	}
	s.refMu.Unlock()
	// One line per address per minute, so a flood cannot fill the journal.
	if logIt {
		s.log.Warn("connection refused", "reason", string(reason), "from", key)
	}
}

// ListenAndServe listens on addr and serves until ctx ends.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, ln)
}

// Serve accepts connections on ln until ctx ends.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	ll := &limitedListener{Listener: ln, lim: s.lim, deadline: s.cfg.SessionDeadline, onRefuse: s.onRefuse}
	errc := make(chan error, 1)
	go func() { errc <- s.srv.Serve(ll) }()
	go s.logStats(ctx)
	select {
	case err := <-errc:
		if errors.Is(err, ssh.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.srv.Shutdown(sctx); err != nil {
			_ = s.srv.Close()
		}
		<-errc
		return nil
	}
}

func (s *Server) logStats(ctx context.Context) {
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	var lastRefused int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r := s.refused.Load()
			s.log.Info("stats", "sessions", s.sessions.Load(), "connections", s.lim.Active(), "refused_15m", r-lastRefused, "tracked_addresses", s.lim.Tracked())
			lastRefused = r
		}
	}
}
