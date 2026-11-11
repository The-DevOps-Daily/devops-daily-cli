package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/limits"
)

type quitModel struct{}

func (quitModel) Init() tea.Cmd                       { return tea.Quit }
func (quitModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return quitModel{}, nil }
func (quitModel) View() string                        { return "hello from the app" }

func start(t *testing.T, mod func(*Config)) string {
	t.Helper()
	cfg := Config{
		HostKeyPath:     filepath.Join(t.TempDir(), "host_ed25519"),
		Limits:          limits.Config{RatePerIP: 100, MaxPerIP: 5, MaxTotal: 50},
		IdleTimeout:     time.Minute,
		MaxSession:      time.Minute,
		SessionDeadline: 5 * time.Second,
		App:             func(*lipgloss.Renderer, func(string)) tea.Model { return quitModel{} },
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if mod != nil {
		mod(&cfg)
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = s.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return ln.Addr().String()
}

func noPrompt(name, instruction string, questions []string, echos []bool) ([]string, error) {
	return make([]string, len(questions)), nil
}

func dial(t *testing.T, addr string, auth ...gossh.AuthMethod) *gossh.Client {
	t.Helper()
	if len(auth) == 0 {
		auth = []gossh.AuthMethod{gossh.KeyboardInteractive(noPrompt)}
	}
	c, err := gossh.Dial("tcp", addr, &gossh.ClientConfig{
		User:            "visitor",
		Auth:            auth,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func readAll(t *testing.T, addr string) (string, time.Duration) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	start := time.Now()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	b, _ := io.ReadAll(conn)
	return string(b), time.Since(start)
}

func TestBannerNamesTheApp(t *testing.T) {
	addr := start(t, func(c *Config) { c.SessionDeadline = 300 * time.Millisecond })
	out, took := readAll(t, addr)
	if !strings.HasPrefix(out, "SSH-2.0-DevOpsDaily") {
		t.Fatalf("banner = %q", out)
	}
	if took > 3*time.Second {
		t.Fatalf("an idle connection stayed open for %v", took)
	}
}

func TestExecGivesNoShell(t *testing.T) {
	c := dial(t, start(t, nil))
	sess, err := c.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	out, err := sess.CombinedOutput("cat /etc/passwd")
	var ee *gossh.ExitError
	if !errors.As(err, &ee) || ee.ExitStatus() != 1 {
		t.Fatalf("want exit status 1, got %v", err)
	}
	if !strings.Contains(string(out), "not a shell") || strings.Contains(string(out), "root:") {
		t.Fatalf("output = %q", out)
	}
}

func TestAnyPublicKeyIsAccepted(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	dial(t, start(t, nil), gossh.PublicKeys(signer))
}

func TestForwardingIsRefused(t *testing.T) {
	c := dial(t, start(t, nil))
	if conn, err := c.Dial("tcp", "127.0.0.1:22"); err == nil {
		conn.Close()
		t.Fatal("local forwarding (direct-tcpip) was allowed")
	}
	if ln, err := c.Listen("tcp", "127.0.0.1:0"); err == nil {
		ln.Close()
		t.Fatal("remote forwarding (tcpip-forward) was allowed")
	}
}

func TestSessionRequestsAreRefused(t *testing.T) {
	c := dial(t, start(t, nil))
	sess, err := c.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.Setenv("LANG", "C"); err == nil {
		t.Error("env request was accepted")
	}
	if err := sess.RequestSubsystem("sftp"); err == nil {
		t.Error("sftp subsystem was accepted")
	}
	if ok, _ := sess.SendRequest("auth-agent-req@openssh.com", true, nil); ok {
		t.Error("agent forwarding was accepted")
	}
	x11 := gossh.Marshal(struct {
		Single bool
		Proto  string
		Cookie string
		Screen uint32
	}{false, "MIT-MAGIC-COOKIE-1", "00", 0})
	if ok, _ := sess.SendRequest("x11-req", true, x11); ok {
		t.Error("X11 forwarding was accepted")
	}
}

func TestOneSessionPerConnection(t *testing.T) {
	c := dial(t, start(t, nil))
	first, err := c.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := c.NewSession(); err == nil {
		second.Close()
		t.Fatal("a second session on one connection was allowed")
	}
}

func TestShellNeedsATerminal(t *testing.T) {
	c := dial(t, start(t, nil))
	sess, err := c.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	sess.Stderr = &stderr
	if err := sess.Shell(); err != nil {
		t.Fatal(err)
	}
	var ee *gossh.ExitError
	if err := sess.Wait(); !errors.As(err, &ee) || ee.ExitStatus() != 1 {
		t.Fatalf("want exit status 1, got %v", err)
	}
	if !strings.Contains(stderr.String(), "interactive terminal") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestAppRunsWithATerminal(t *testing.T) {
	c := dial(t, start(t, nil))
	sess, err := c.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.RequestPty("xterm-256color", 24, 80, gossh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	sess.Stdout = &out
	began := time.Now()
	if err := sess.Shell(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("session ended with %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("app session did not end")
	}
	if took := time.Since(began); took > 900*time.Millisecond {
		t.Fatalf("app took %v to start and quit", took)
	}
	if !strings.Contains(out.String(), "Thanks for visiting DevOps Daily") {
		t.Fatalf("goodbye missing from %q", out.String())
	}
	// The app must not probe the client terminal (OSC 11 / DA1): that
	// delays startup and eats the first keys.
	if strings.Contains(out.String(), "\x1b]11;?") || strings.Contains(out.String(), "\x1b[c") {
		t.Fatalf("terminal query sent: %q", out.String())
	}
}

func TestEleventhConnectionIsRefused(t *testing.T) {
	addr := start(t, func(c *Config) { c.Limits.RatePerIP = 10 })
	for i := 0; i < 10; i++ {
		dial(t, addr).Close()
	}
	out, _ := readAll(t, addr)
	if !strings.Contains(out, "Too many connections") || strings.Contains(out, "SSH-2.0") {
		t.Fatalf("11th connection got %q", out)
	}
}

func TestOpenConnectionsPerIP(t *testing.T) {
	addr := start(t, func(c *Config) { c.Limits.MaxPerIP = 2 })
	a := dial(t, addr)
	dial(t, addr)
	if out, _ := readAll(t, addr); !strings.Contains(out, "Too many connections") {
		t.Fatalf("3rd open connection got %q", out)
	}
	a.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		out, _ := readAll(t, addr)
		if strings.HasPrefix(out, "SSH-2.0-") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("slot was not freed after a connection closed")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestConnectionWithoutSessionIsClosed(t *testing.T) {
	addr := start(t, func(c *Config) { c.SessionDeadline = 300 * time.Millisecond })
	c := dial(t, addr) // like ssh -N: authenticated, no shell
	done := make(chan struct{})
	go func() { c.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a connection without a session stayed open")
	}
}

func TestHostKeyIsPersistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "host_ed25519")
	a, created, err := LoadOrCreateHostKey(path)
	if err != nil || !created {
		t.Fatalf("first load: created=%v err=%v", created, err)
	}
	b, created, err := LoadOrCreateHostKey(path)
	if err != nil || created {
		t.Fatalf("second load: created=%v err=%v", created, err)
	}
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatal("host key changed between loads")
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("host key mode %v, want 0600", fi.Mode().Perm())
	}
	if !strings.HasPrefix(Fingerprint(a), "SHA256:") {
		t.Fatalf("fingerprint %q", Fingerprint(a))
	}
}

func TestWindowIsClamped(t *testing.T) {
	w := clampWindow(ssh.Window{Width: 1 << 30, Height: 1 << 30, WidthPixels: 5, HeightPixels: 5})
	if w.Width != maxCols || w.Height != maxRows || w.WidthPixels != 0 {
		t.Fatalf("clamped to %+v", w)
	}
	if w := clampWindow(ssh.Window{}); w.Width != 80 || w.Height != 24 {
		t.Fatalf("zero size became %+v", w)
	}
}
