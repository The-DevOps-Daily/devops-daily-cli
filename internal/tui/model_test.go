package tui

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/content"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/feed"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/sponsor"
)

type harness struct {
	now    time.Time
	reason string
}

func newModel(t *testing.T, h *harness, withSponsor bool) Model {
	t.Helper()
	snap, err := content.Load()
	if err != nil {
		t.Fatal(err)
	}
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI256)
	r.SetHasDarkBackground(true)
	posts := []feed.Post{}
	for i := 0; i < 15; i++ {
		posts = append(posts, feed.Post{
			Title:   strings.Repeat("A fairly long post title that needs truncating ", 2),
			URL:     "https://devops-daily.com/posts/example-post-with-a-long-slug-for-wrapping-tests",
			Summary: "A short summary of the post. It has two sentences.",
			Date:    h.now.Add(-time.Duration(i) * 24 * time.Hour),
		})
	}
	sp := sponsor.Sponsor{}
	if withSponsor {
		sp = sponsor.Sponsor{Name: "Example", Text: "Managed things for teams", URL: "https://example.com/product"}
	}
	return New(r, Deps{
		Posts:       func() ([]feed.Post, time.Time) { return posts, h.now.Add(-12 * time.Minute) },
		Content:     snap,
		Sponsor:     func() sponsor.Sponsor { return sp },
		Now:         func() time.Time { return h.now },
		IdleTimeout: 10 * time.Minute,
		MaxSession:  30 * time.Minute,
		OnQuit:      func(r string) { h.reason = r },
	})
}

func send(m Model, msgs ...tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	var tm tea.Model = m
	for _, msg := range msgs {
		tm, cmd = tm.Update(msg)
	}
	return tm.(Model), cmd
}

func key(s string) tea.Msg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// checkFits fails if the view is taller or wider than the window.
func checkFits(t *testing.T, name string, view string, w, h int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > h {
		t.Errorf("%s: %d lines in a %d-line window", name, len(lines), h)
	}
	for i, l := range lines {
		if n := ansi.StringWidth(l); n > w {
			t.Errorf("%s: line %d is %d columns in a %d-column window", name, i, n, w)
		}
	}
	if strings.Contains(view, "\u2014") {
		t.Errorf("%s: view contains an em dash", name)
	}
}

func TestEveryScreenFitsCommonSizes(t *testing.T) {
	h := &harness{now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	sizes := [][2]int{{80, 24}, {40, 12}, {120, 40}, {300, 100}}
	screens := map[string][]tea.Msg{
		"menu":          nil,
		"posts":         {key("1")},
		"post":          {key("1"), key("down"), key("enter")},
		"quiz":          {key("2")},
		"quiz answered": {key("2"), key("a")},
		"incident":      {key("3")},
		"incident fix":  {key("3"), key("enter")},
		"newsletter":    {key("4")},
		"links":         {key("5")},
	}
	for _, sp := range []bool{true, false} {
		for _, sz := range sizes {
			for name, keys := range screens {
				m := newModel(t, h, sp)
				m, _ = send(m, append([]tea.Msg{tea.WindowSizeMsg{Width: sz[0], Height: sz[1]}}, keys...)...)
				checkFits(t, name, m.View(), sz[0], sz[1])
				// Scrolling to the end must also fit.
				m, _ = send(m, key("G"))
				checkFits(t, name+" (end)", m.View(), sz[0], sz[1])
			}
		}
	}
}

func TestHugeWindowIsClamped(t *testing.T) {
	h := &harness{now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	m := newModel(t, h, true)
	m, _ = send(m, tea.WindowSizeMsg{Width: 1 << 30, Height: 1 << 30}, key("3"), key("enter"))
	if m.width != maxWidth || m.height != maxHeight {
		t.Fatalf("size not clamped: %dx%d", m.width, m.height)
	}
	checkFits(t, "huge", m.View(), maxWidth, maxHeight)
}

func TestTinyWindowAsksToResize(t *testing.T) {
	h := &harness{now: time.Now()}
	m := newModel(t, h, true)
	m, _ = send(m, tea.WindowSizeMsg{Width: 20, Height: 5})
	v := m.View()
	checkFits(t, "tiny", v, 20, 5)
	if !strings.Contains(v, "Please") {
		t.Fatalf("no resize note: %q", v)
	}
	if _, cmd := send(m, key("q")); !isQuit(cmd) {
		t.Fatal("q must still quit in a tiny window")
	}
}

func TestMenuItemsWork(t *testing.T) {
	h := &harness{now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	base := newModel(t, h, true)
	base, _ = send(base, tea.WindowSizeMsg{Width: 100, Height: 60})

	m, _ := send(base, key("1"))
	if !strings.Contains(m.View(), "Latest posts") || !strings.Contains(m.View(), "updated 12 min ago") {
		t.Fatalf("posts screen:\n%s", m.View())
	}
	m, _ = send(m, key("enter"))
	if !strings.Contains(m.View(), "Read it on the site") {
		t.Fatalf("post screen:\n%s", m.View())
	}

	m, _ = send(base, key("2"), key("s"))
	q := m.quiz
	if !strings.Contains(m.View(), "The answer is") || !strings.Contains(m.View(), q.URL()) {
		t.Fatalf("quiz screen:\n%s", m.View())
	}

	m, _ = send(base, key("2"), key(string(rune('a'+base.quiz.Correct))))
	if !strings.Contains(m.View(), "Correct!") {
		t.Fatalf("right answer not marked:\n%s", m.View())
	}

	m, _ = send(base, key("3"))
	if strings.Contains(m.View(), "The fix:") {
		t.Fatal("fix shown before asking")
	}
	m, _ = send(m, key("enter"))
	if !strings.Contains(m.View(), "The fix:") || !strings.Contains(m.View(), m.incident.LinkURL) {
		t.Fatalf("incident screen:\n%s", m.View())
	}

	m, _ = send(base, key("4"))
	if !strings.Contains(m.View(), "https://devops-daily.com/newsletters") {
		t.Fatalf("newsletter screen:\n%s", m.View())
	}

	m, _ = send(base, key("5"))
	for _, u := range []string{"https://devops-daily.com/simulators", "https://devops-daily.com/opsville/", "https://devops-daily.com/roadmap", "https://github.com/The-DevOps-Daily"} {
		if !strings.Contains(m.View(), u) {
			t.Fatalf("links screen is missing %s", u)
		}
	}
	m, _ = send(m, key("esc"))
	if m.screen != screenMenu {
		t.Fatal("esc did not go back to the menu")
	}
	if !strings.Contains(m.View(), "Supported by") {
		t.Fatal("sponsor line missing")
	}
}

func TestSponsorLineHiddenWhenEmpty(t *testing.T) {
	m := newModel(t, &harness{now: time.Now()}, false)
	if strings.Contains(m.View(), "Supported by") {
		t.Fatal("empty sponsor should hide the line")
	}
}

func TestQuitKeys(t *testing.T) {
	m := newModel(t, &harness{now: time.Now()}, true)
	for _, k := range []string{"q", "ctrl+c"} {
		if _, cmd := send(m, key("3"), key(k)); !isQuit(cmd) {
			t.Fatalf("%s did not quit", k)
		}
	}
}

func TestIdleAndMaxSession(t *testing.T) {
	h := &harness{now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	m := newModel(t, h, true)

	h.now = h.now.Add(9 * time.Minute)
	m, cmd := send(m, tickMsg(h.now))
	if isQuitCmdName(cmd) {
		t.Fatal("quit before the idle timeout")
	}
	m, _ = send(m, key("down")) // activity resets the idle clock
	h.now = h.now.Add(9 * time.Minute)
	if _, cmd := send(m, tickMsg(h.now)); isQuitCmdName(cmd) {
		t.Fatal("idle clock did not reset on a key press")
	}
	h.now = h.now.Add(2 * time.Minute)
	if _, cmd := send(m, tickMsg(h.now)); !isQuit(cmd) || !strings.Contains(h.reason, "10 minutes") {
		t.Fatalf("idle quit missing, reason %q", h.reason)
	}

	h2 := &harness{now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	m2 := newModel(t, h2, true)
	for i := 0; i < 5; i++ { // a key every 5 minutes keeps it from going idle
		h2.now = h2.now.Add(5 * time.Minute)
		var cmd tea.Cmd
		m2, cmd = send(m2, key("j"), tickMsg(h2.now))
		if isQuitCmdName(cmd) {
			t.Fatalf("quit after %d minutes", (i+1)*5)
		}
	}
	h2.now = h2.now.Add(5 * time.Minute)
	if _, cmd := send(m2, key("j"), tickMsg(h2.now)); !isQuit(cmd) || !strings.Contains(h2.reason, "30 minutes") {
		t.Fatalf("max session quit missing, reason %q", h2.reason)
	}
}

// isQuitCmdName checks for tea.Quit without running a tick command, which
// would sleep.
func isQuitCmdName(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	done := make(chan bool, 1)
	go func() { _, ok := cmd().(tea.QuitMsg); done <- ok }()
	select {
	case ok := <-done:
		return ok
	case <-time.After(50 * time.Millisecond):
		return false
	}
}

func TestQuizDayRollsOver(t *testing.T) {
	h := &harness{now: time.Date(2026, 11, 10, 23, 59, 0, 0, time.UTC)}
	m := newModel(t, h, true)
	first := m.quiz.ID + m.quiz.QuizID
	h.now = h.now.Add(2 * time.Minute)
	m, _ = send(m, key("2"))
	if m.quiz.ID+m.quiz.QuizID == first {
		t.Fatal("quiz did not change after midnight UTC")
	}
}

func TestKeysInOneReadAreSplit(t *testing.T) {
	m := newModel(t, &harness{now: time.Now()}, true)
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2s")})
	if m.screen != screenQuiz || !m.quizShown {
		t.Fatalf("\"2s\" in one read: screen %d, shown %v", m.screen, m.quizShown)
	}
	if _, cmd := send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("xq")}); !isQuit(cmd) {
		t.Fatal("q inside a multi-rune read did not quit")
	}
}
