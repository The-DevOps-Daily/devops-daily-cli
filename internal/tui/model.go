// Package tui is the Bubble Tea app each SSH session runs.
package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/content"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/feed"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/sponsor"
)

const (
	siteURL = content.SiteURL

	// Window sizes outside these bounds are clamped (large) or get a
	// "please resize" note (small).
	maxWidth  = 500
	maxHeight = 200
	minWidth  = 40
	minHeight = 12

	frameMax  = 104 // widest the layout grows; wider terminals center it
	tickEvery = 15 * time.Second
)

// Deps is what the app reads from the server.
type Deps struct {
	Posts       func() ([]feed.Post, time.Time)
	Content     *content.Snapshot
	Sponsor     func() sponsor.Sponsor
	Now         func() time.Time
	IdleTimeout time.Duration // quit after this long without a key press
	MaxSession  time.Duration // quit after this long in total
	OnQuit      func(reason string)
	Header      string
	Article     func(p feed.Post, width int) ([]string, error)
	OpenURL     func(url string) error
	Start       string
}

type screen int

const (
	screenMenu screen = iota
	screenPosts
	screenPost
	screenQuiz
	screenIncident
	screenNewsletter
	screenLinks
)

type menuItem struct {
	key, title, desc string
	target           screen
}

var menu = []menuItem{
	{"1", "Latest posts", "The newest posts from devops-daily.com", screenPosts},
	{"2", "Daily quiz", "One question a day, the same for everyone", screenQuiz},
	{"3", "Incident of the day", "A real-world DevOps problem and the fix", screenIncident},
	{"4", "Newsletter", "Get the weekly email", screenNewsletter},
	{"5", "Links", "Site, simulators, Opsville, roadmap, GitHub", screenLinks},
}

type tickMsg time.Time

// Model is the per-session app state.
type Model struct {
	d  Deps
	st styles

	width, height int
	screen        screen
	menuIdx       int
	scroll        int

	posts   []feed.Post
	updated time.Time
	postIdx int

	day           time.Time
	quiz          content.Question
	quizPick      int // -1 until the visitor answers
	quizShown     bool
	incident      content.Incident
	incidentShown bool

	sponsor  sponsor.Sponsor
	started  time.Time
	lastKey  time.Time
	quitting bool
	article  *articleCache
	openNote string
}

type articleCache struct {
	url   string
	width int
	lines []string
	err   error
}

// New returns the app for one session.
func New(r *lipgloss.Renderer, d Deps) Model {
	if d.Now == nil {
		d.Now = time.Now
	}
	now := d.Now()
	m := Model{d: d, st: newStyles(r), width: 80, height: 24, started: now, lastKey: now, quizPick: -1, day: now, article: &articleCache{}}
	if d.Sponsor != nil {
		m.sponsor = d.Sponsor()
	}
	if d.Content != nil {
		m.quiz = d.Content.QuestionFor(now)
		m.incident = d.Content.IncidentFor(now)
	}
	switch d.Start {
	case "incident":
		m = m.open(screenIncident)
	case "quiz":
		m = m.open(screenQuiz)
	case "posts":
		m = m.open(screenPosts)
	}
	return m
}

// Init starts the clock that enforces the idle and session limits.
func (m Model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func clampSize(w, h int) (int, int) {
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	return min(w, maxWidth), min(h, maxHeight)
}

// Update handles keys, window changes and the clock.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = clampSize(msg.Width, msg.Height)
		m = m.clampScroll()
		return m, nil
	case tickMsg:
		now := m.d.Now()
		if m.d.MaxSession > 0 && now.Sub(m.started) >= m.d.MaxSession {
			return m.quit(fmt.Sprintf("Sessions last at most %s. Come back any time.", human(m.d.MaxSession)))
		}
		if m.d.IdleTimeout > 0 && now.Sub(m.lastKey) >= m.d.IdleTimeout {
			return m.quit(fmt.Sprintf("Closed after %s without a key press.", human(m.d.IdleTimeout)))
		}
		return m, tick()
	case tea.KeyMsg:
		m.lastKey = m.d.Now()
		// Keys that arrive in one read (fast typing on a slow link) come as
		// one multi-rune message; handle them one by one.
		if msg.Type == tea.KeyRunes && !msg.Paste && len(msg.Runes) > 1 {
			var cmds []tea.Cmd
			for _, r := range msg.Runes {
				next, cmd := m.handleKey(string(r))
				m = next.(Model)
				cmds = append(cmds, cmd)
				if m.quitting {
					break
				}
			}
			return m, tea.Batch(cmds...)
		}
		return m.handleKey(msg.String())
	}
	return m, nil
}

func (m Model) quit(reason string) (tea.Model, tea.Cmd) {
	m.quitting = true
	if reason != "" && m.d.OnQuit != nil {
		m.d.OnQuit(reason)
	}
	return m, tea.Quit
}

func isBack(key string) bool {
	return key == "esc" || key == "backspace" || key == "left" || key == "h"
}

func (m Model) handleKey(key string) (tea.Model, tea.Cmd) {
	if key == "ctrl+c" || key == "q" || key == "Q" {
		return m.quit("")
	}
	if m.tooSmall() {
		return m, nil
	}
	if key != "o" {
		m.openNote = ""
	}
	if key == "o" && m.d.OpenURL != nil {
		if url := m.openURL(); url != "" {
			if err := m.d.OpenURL(url); err != nil {
				m.openNote = "Could not open a browser. The link: " + url
			} else {
				m.openNote = "Opened in your browser."
			}
		}
		return m, nil
	}
	switch m.screen {
	case screenMenu:
		switch key {
		case "up", "k", "shift+tab":
			m.menuIdx = (m.menuIdx + len(menu) - 1) % len(menu)
		case "down", "j", "tab":
			m.menuIdx = (m.menuIdx + 1) % len(menu)
		case "enter", "right", "l", " ":
			return m.open(menu[m.menuIdx].target), nil
		default:
			for i, it := range menu {
				if key == it.key {
					m.menuIdx = i
					return m.open(it.target), nil
				}
			}
		}
		return m, nil

	case screenPosts:
		switch {
		case key == "up" || key == "k":
			if m.postIdx > 0 {
				m.postIdx--
			}
		case key == "down" || key == "j":
			if m.postIdx < len(m.posts)-1 {
				m.postIdx++
			}
		case key == "home" || key == "g":
			m.postIdx = 0
		case key == "end" || key == "G":
			m.postIdx = max(0, len(m.posts)-1)
		case key == "enter" || key == "right" || key == "l" || key == " ":
			if len(m.posts) > 0 {
				m.screen, m.scroll = screenPost, 0
			}
		case isBack(key):
			return m.open(screenMenu), nil
		}
		return m, nil

	case screenPost:
		switch {
		case key == "n":
			if m.postIdx < len(m.posts)-1 {
				m.postIdx, m.scroll = m.postIdx+1, 0
			}
			return m, nil
		case key == "p":
			if m.postIdx > 0 {
				m.postIdx, m.scroll = m.postIdx-1, 0
			}
			return m, nil
		case isBack(key):
			m.screen, m.scroll = screenPosts, 0
			return m, nil
		}

	case screenQuiz:
		if !m.quizShown {
			if i := answerIndex(key, len(m.quiz.Options)); i >= 0 {
				m.quizPick, m.quizShown = i, true
				return m.scrollToAnchor(), nil
			}
			if key == "s" {
				m.quizShown = true
				return m.scrollToAnchor(), nil
			}
		}

	case screenIncident:
		if !m.incidentShown && (key == "enter" || key == " " || key == "f") {
			m.incidentShown = true
			return m.scrollToAnchor(), nil
		}
	}

	if isBack(key) {
		return m.open(screenMenu), nil
	}
	return m.scrollKey(key), nil
}

func answerIndex(key string, n int) int {
	i := -1
	if len(key) == 1 {
		switch c := key[0]; {
		case c >= 'a' && c <= 'd':
			i = int(c - 'a')
		case c >= '1' && c <= '4':
			i = int(c - '1')
		}
	}
	if i >= n {
		return -1
	}
	return i
}

func (m Model) open(s screen) Model {
	m.screen, m.scroll = s, 0
	m.openNote = ""
	if m.d.Sponsor != nil {
		m.sponsor = m.d.Sponsor()
	}
	switch s {
	case screenPosts:
		if m.d.Posts != nil {
			m.posts, m.updated = m.d.Posts()
		}
		if m.postIdx >= len(m.posts) {
			m.postIdx = 0
		}
	case screenQuiz, screenIncident:
		// A session can cross midnight UTC; show that day's picks.
		now := m.d.Now()
		if m.d.Content != nil && content.DayNumber(now) != content.DayNumber(m.day) {
			m.day = now
			m.quiz, m.quizPick, m.quizShown = m.d.Content.QuestionFor(now), -1, false
			m.incident, m.incidentShown = m.d.Content.IncidentFor(now), false
		}
	}
	if s == screenPost {
		m.article = &articleCache{}
	}
	return m
}

func (m Model) scrollable() bool {
	return m.screen != screenMenu && m.screen != screenPosts
}

func (m Model) maxScroll() int {
	if !m.scrollable() {
		return 0
	}
	bodyH := m.bodyHeight()
	lines, _ := m.body(bodyH, m.textWidth())
	return max(0, len(lines)-bodyH)
}

func (m Model) clampScroll() Model {
	m.scroll = max(0, min(m.scroll, m.maxScroll()))
	return m
}

func (m Model) scrollKey(key string) Model {
	if !m.scrollable() {
		return m
	}
	page := max(1, m.bodyHeight()-1)
	switch key {
	case "up", "k":
		m.scroll--
	case "down", "j":
		m.scroll++
	case "pgup", "ctrl+b", "ctrl+u":
		m.scroll -= page
	case "pgdown", " ", "ctrl+f", "ctrl+d":
		m.scroll += page
	case "home", "g":
		m.scroll = 0
	case "end", "G":
		m.scroll = m.maxScroll()
	}
	return m.clampScroll()
}

// scrollToAnchor brings a just-revealed answer into view.
func (m Model) scrollToAnchor() Model {
	bodyH := m.bodyHeight()
	_, anchor := m.body(bodyH, m.textWidth())
	if anchor+4 > m.scroll+bodyH {
		m.scroll = anchor - 2
	}
	return m.clampScroll()
}

func (m Model) tooSmall() bool { return m.width < minWidth || m.height < minHeight }

func (m Model) frameWidth() int { return min(m.width, frameMax) }

func (m Model) textWidth() int { return m.frameWidth() - 4 }

const headerHeight = 3

// footerGap is the blank line between the body and the footer, kept when
// the window has room for it.
func (m Model) footerGap() int {
	if m.height >= 18 {
		return 1
	}
	return 0
}

func (m Model) footerHeight() int {
	if m.sponsor.Empty() {
		return 1 + m.footerGap()
	}
	return 2 + m.footerGap()
}

func (m Model) bodyHeight() int { return max(1, m.height-headerHeight-m.footerHeight()) }

// View draws the whole screen. Every line is cut to the window width, so
// the terminal never wraps.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.tooSmall() {
		msg := fmt.Sprintf("This window is %dx%d. Please make it at least %dx%d, or press q to quit.", m.width, m.height, minWidth, minHeight)
		lines := wrap(msg, max(10, m.width-2))
		if len(lines) > m.height {
			lines = lines[:m.height]
		}
		for i, l := range lines {
			lines[i] = ansi.Truncate(l, m.width, "")
		}
		return strings.Join(lines, "\n")
	}

	fw, tw := m.frameWidth(), m.textWidth()
	bodyH := m.bodyHeight()
	body, _ := m.body(bodyH, tw)
	scroll := 0
	if m.scrollable() {
		scroll = max(0, min(m.scroll, len(body)-bodyH))
	}
	more := scroll+bodyH < len(body)
	if len(body) > scroll {
		body = body[scroll:]
	} else {
		body = nil
	}
	if len(body) > bodyH {
		body = body[:bodyH]
	}

	out := make([]string, 0, m.height)
	out = append(out, m.header(tw)...)
	for _, l := range body {
		out = append(out, "  "+l)
	}
	for len(out) < headerHeight+bodyH+m.footerGap() {
		out = append(out, "")
	}
	if s := m.sponsorLine(tw); s != "" {
		out = append(out, "  "+s)
	}
	out = append(out, "  "+m.helpLine(tw, more))

	pad := strings.Repeat(" ", (m.width-fw)/2)
	for i, l := range out {
		out[i] = pad + ansi.Truncate(l, fw, "")
	}
	return strings.Join(out, "\n")
}

func (m Model) header(tw int) []string {
	left := m.st.accent.Render(">_ ") + m.st.bold.Render("DevOps Daily")
	header := m.d.Header
	if header == "" {
		header = "ssh.devops-daily.com"
	}
	right := m.st.muted.Render(header)
	gap := tw - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 2 {
		right, gap = "", 1
	}
	return []string{
		"  " + left + strings.Repeat(" ", gap) + right,
		"  " + m.st.rule.Render(strings.Repeat("─", tw)),
		"",
	}
}

func (m Model) sponsorLine(tw int) string {
	s := m.sponsor
	if s.Empty() {
		return ""
	}
	base := m.st.muted.Render("Supported by ") + m.st.title.Render(s.Name)
	url := ""
	if s.URL != "" {
		url = m.st.muted.Render("  ") + m.st.accent.Render(s.URL)
	}
	// The longest version that fits, so a URL is never cut in half.
	var candidates []string
	if s.Text != "" {
		candidates = append(candidates, base+m.st.muted.Render(": "+s.Text)+url)
	}
	for _, line := range append(candidates, base+url) {
		if ansi.StringWidth(line) <= tw {
			return line
		}
	}
	return base
}

func (m Model) helpLine(tw int, more bool) string {
	var items [][2]string
	switch m.screen {
	case screenMenu:
		items = [][2]string{{"↑/↓", "move"}, {"enter", "open"}, {"1-5", "jump"}, {"q", "quit"}}
	case screenPosts:
		items = [][2]string{{"↑/↓", "move"}, {"enter", "read"}, {"esc", "back"}, {"q", "quit"}}
	case screenPost:
		items = [][2]string{{"↑/↓", "scroll"}, {"n/p", "next/previous"}, {"esc", "back"}, {"q", "quit"}}
	case screenQuiz:
		if m.quizShown {
			items = [][2]string{{"↑/↓", "scroll"}, {"esc", "back"}, {"q", "quit"}}
		} else {
			items = [][2]string{{"a-d", "answer"}, {"s", "show answer"}, {"↑/↓", "scroll"}, {"esc", "back"}, {"q", "quit"}}
		}
	case screenIncident:
		if m.incidentShown {
			items = [][2]string{{"↑/↓", "scroll"}, {"esc", "back"}, {"q", "quit"}}
		} else {
			items = [][2]string{{"enter", "show the fix"}, {"↑/↓", "scroll"}, {"esc", "back"}, {"q", "quit"}}
		}
	default:
		items = [][2]string{{"↑/↓", "scroll"}, {"esc", "back"}, {"q", "quit"}}
	}
	if m.d.OpenURL != nil && m.openURL() != "" {
		items = append(items[:len(items)-1], append([][2]string{{"o", "open"}}, items[len(items)-1])...)
	}
	render := func(items [][2]string) string {
		parts := make([]string, len(items))
		for i, it := range items {
			parts[i] = m.st.key.Render(it[0]) + " " + m.st.muted.Render(it[1])
		}
		s := strings.Join(parts, m.st.muted.Render(" · "))
		if more {
			s += "   " + m.st.accent.Render("↓ more")
		}
		return s
	}
	s := render(items)
	// Narrow window: drop hints from the middle, but always keep "q quit".
	for ansi.StringWidth(s) > tw && len(items) > 1 {
		items = append(items[:len(items)-2:len(items)-2], items[len(items)-1])
		s = render(items)
	}
	return s
}

// body returns the lines for the current screen and, for screens that
// reveal an answer, the line where the answer starts.
func (m Model) body(bodyH, tw int) ([]string, int) {
	switch m.screen {
	case screenPosts:
		return m.postsBody(bodyH, tw), 0
	case screenPost:
		return m.postBody(tw), 0
	case screenQuiz:
		return m.quizBody(tw)
	case screenIncident:
		return m.incidentBody(tw)
	case screenNewsletter:
		return m.newsletterBody(tw), 0
	case screenLinks:
		return m.linksBody(tw), 0
	default:
		return m.menuBody(bodyH, tw), 0
	}
}

func (m Model) menuBody(bodyH, tw int) []string {
	var lines []string
	if bodyH >= 16 && tw >= 44 {
		box := m.st.box.Render(m.st.accent.Render(">_ ") + m.st.title.Render("DevOps Daily") + "\n" +
			m.st.muted.Render("Learn DevOps from your terminal."))
		lines = append(lines, strings.Split(box, "\n")...)
		lines = append(lines, "")
	}
	lines = append(lines, m.st.text.Render("Pick a section:"), "")
	for i, it := range menu {
		cursor, name := "  ", m.st.text.Render(padRight(it.title, 20))
		if i == m.menuIdx {
			cursor, name = m.st.accent.Render("▸ "), m.st.selected.Render(padRight(it.title, 20))
		}
		line := cursor + m.st.key.Render(it.key) + "  " + name
		if tw >= 72 { // descriptions only where they fit whole
			line += "  " + m.st.muted.Render(it.desc)
		}
		lines = append(lines, line)
	}
	if m.d.Content != nil && bodyH-len(lines) >= 2 {
		lines = append(lines, "", m.st.muted.Render("Today's incident: ")+m.st.text.Render(m.incident.Title))
	}
	return lines
}

func (m Model) postsBody(bodyH, tw int) []string {
	lines := []string{m.st.title.Render("Latest posts") + m.st.muted.Render("  from devops-daily.com, "+m.updatedText()), ""}
	if len(m.posts) == 0 {
		return append(lines, styleLines(wrap("The feed has not loaded yet. Please try again in a minute.", tw), m.st.text)...)
	}
	// One line for the URL, plus a blank line above it when there is room.
	avail := bodyH - len(lines) - 1
	blank := avail-1 >= len(m.posts)
	room := avail
	if blank {
		room--
	}
	room = max(1, room)
	start := 0
	if m.postIdx >= room {
		start = m.postIdx - room + 1
	}
	end := min(len(m.posts), start+room)
	for i := start; i < end; i++ {
		p := m.posts[i]
		date := "      "
		if !p.Date.IsZero() {
			date = p.Date.Format("02 Jan")
		}
		title := ansi.Truncate(p.Title, max(10, tw-10), "…")
		cursor, styled := "  ", m.st.text.Render(title)
		if i == m.postIdx {
			cursor, styled = m.st.accent.Render("▸ "), m.st.selected.Render(title)
		}
		lines = append(lines, cursor+m.st.muted.Render(date)+"  "+styled)
	}
	if blank {
		lines = append(lines, "")
	}
	return append(lines, m.st.muted.Render("→ ")+m.st.accent.Render(m.posts[m.postIdx].URL))
}

func (m Model) updatedText() string {
	if m.updated.IsZero() {
		return "not loaded yet"
	}
	age := m.d.Now().Sub(m.updated)
	switch {
	case age < time.Minute:
		return "updated just now"
	case age < time.Hour:
		return fmt.Sprintf("updated %d min ago", int(age.Minutes()))
	default:
		return fmt.Sprintf("updated %d h ago", int(age.Hours()))
	}
}

func (m Model) postBody(tw int) []string {
	if len(m.posts) == 0 {
		return nil
	}
	p := m.posts[m.postIdx]
	lines := styleLines(wrap(p.Title, tw), m.st.title)
	meta := fmt.Sprintf("Post %d of %d", m.postIdx+1, len(m.posts))
	if !p.Date.IsZero() {
		meta = p.Date.Format("Monday 2 January 2006") + "  ·  " + meta
	}
	lines = append(lines, m.st.muted.Render(meta), "")
	if m.d.Article != nil {
		if m.article == nil || m.article.url != p.URL || m.article.width != tw {
			lines, err := m.d.Article(p, tw)
			if m.article == nil {
				m.article = &articleCache{}
			}
			*m.article = articleCache{url: p.URL, width: tw, lines: lines, err: err}
		}
		if m.article.err == nil {
			lines = append(lines, m.article.lines...)
			lines = append(lines, "")
		} else {
			lines = append(lines, styleLines(wrap(p.Summary, tw), m.st.text)...)
			lines = append(lines, m.st.muted.Render("Could not render article: "+m.article.err.Error()), "")
		}
	} else if p.Summary != "" {
		lines = append(lines, styleLines(wrap(p.Summary, tw), m.st.text)...)
		lines = append(lines, "")
	}
	lines = append(lines, m.st.text.Render("Read it on the site:"))
	lines = append(lines, styleLines(hardWrap(p.URL, tw), m.st.accent)...)
	return m.withOpenNote(lines, tw)
}

func (m Model) dateLabel() string { return m.day.UTC().Format("Mon 2 Jan 2006") + " (UTC)" }

func (m Model) quizBody(tw int) ([]string, int) {
	q := m.quiz
	if len(q.Options) == 0 {
		return []string{m.st.text.Render("No quiz today.")}, 0
	}
	lines := []string{m.st.title.Render("Daily quiz") + m.st.muted.Render("  "+m.dateLabel())}
	from := "From the " + q.QuizTitle
	if q.Difficulty != "" {
		from += " · " + q.Difficulty
	}
	lines = append(lines, styleLines(wrap(from, tw), m.st.muted)...)
	lines = append(lines, "")
	if q.Title != "" {
		lines = append(lines, styleLines(wrap(q.Title, tw), m.st.bold)...)
	}
	if q.Situation != "" {
		lines = append(lines, styleLines(wrap(q.Situation, tw), m.st.text)...)
	}
	if q.Code != "" {
		lines = append(lines, "")
		for _, l := range hardWrap(q.Code, tw-2) {
			lines = append(lines, m.st.rule.Render("│ ")+m.st.code.Render(l))
		}
	}
	lines = append(lines, "")
	lines = append(lines, styleLines(wrap(q.Question, tw), m.st.bold)...)
	lines = append(lines, "")
	for i, o := range q.Options {
		st, mark := m.st.text, ""
		if m.quizShown {
			switch {
			case i == q.Correct:
				st, mark = m.st.good, "  ✓"
			case i == m.quizPick:
				st, mark = m.st.bad, "  ✗"
			default:
				st = m.st.muted
			}
		}
		lines = append(lines, styleLines(wrap(string(rune('a'+i))+") "+o+mark, tw), st)...)
	}
	lines = append(lines, "")
	anchor := len(lines)
	if !m.quizShown {
		lines = append(lines, styleLines(wrap("Press a, b, c or d to answer, or s to see the answer.", tw), m.st.muted)...)
		return m.withOpenNote(lines, tw), anchor
	}
	answer := "The answer is " + string(rune('a'+q.Correct)) + ")."
	switch {
	case m.quizPick == q.Correct:
		lines = append(lines, m.st.good.Render("Correct!"))
	case m.quizPick >= 0:
		lines = append(lines, m.st.bad.Render("Not quite.")+" "+m.st.text.Render(answer))
	default:
		lines = append(lines, m.st.bold.Render(answer))
	}
	if q.Explanation != "" {
		lines = append(lines, "")
		lines = append(lines, styleLines(wrap(q.Explanation, tw), m.st.text)...)
	}
	lines = append(lines, "", m.st.text.Render("Take the full quiz:"))
	lines = append(lines, styleLines(hardWrap(q.URL(), tw), m.st.accent)...)
	return m.withOpenNote(lines, tw), anchor
}

func (m Model) incidentBody(tw int) ([]string, int) {
	in := m.incident
	if in.Title == "" {
		return []string{m.st.text.Render("No incident today.")}, 0
	}
	lines := []string{m.st.title.Render("Incident of the day") + m.st.muted.Render("  "+m.dateLabel())}
	lines = append(lines, styleLines(wrap("From the 30 Days of Real-World DevOps Problems series", tw), m.st.muted)...)
	lines = append(lines, "")
	lines = append(lines, styleLines(wrap(in.Title, tw), m.st.bold)...)
	var meta []string
	if in.Difficulty != "" {
		meta = append(meta, in.Difficulty)
	}
	if len(in.Technologies) > 0 {
		meta = append(meta, strings.Join(in.Technologies, ", "))
	}
	if len(meta) > 0 {
		lines = append(lines, styleLines(wrap(strings.Join(meta, " · "), tw), m.st.muted)...)
	}
	lines = append(lines, "")
	lines = append(lines, styleLines(wrap(in.Scenario, tw), m.st.text)...)
	lines = append(lines, "")
	for _, o := range in.Options {
		st, mark := m.st.text, ""
		if m.incidentShown {
			if o.Key == in.Correct {
				st, mark = m.st.good, "  ✓"
			} else {
				st = m.st.muted
			}
		}
		lines = append(lines, styleLines(wrap(o.Key+") "+o.Text+mark, tw), st)...)
	}
	lines = append(lines, "")
	anchor := len(lines)
	if !m.incidentShown {
		lines = append(lines, styleLines(wrap("Think it through, then press enter to see the fix.", tw), m.st.muted)...)
		return lines, anchor
	}
	c := in.CorrectOption()
	lines = append(lines, styleLines(wrap("The fix: "+c.Key+") "+c.Text, tw), m.st.good)...)
	lines = append(lines, "")
	lines = append(lines, styleLines(wrap(in.Fix, tw), m.st.text)...)
	if in.Lesson != "" {
		lines = append(lines, "")
		lines = append(lines, styleLines(wrap("Lesson: "+in.Lesson, tw), m.st.bold)...)
	}
	lines = append(lines, "")
	lines = append(lines, styleLines(wrap("Go deeper: "+in.LinkTitle, tw), m.st.text)...)
	lines = append(lines, styleLines(hardWrap(in.LinkURL, tw), m.st.accent)...)
	return m.withOpenNote(lines, tw), anchor
}

func (m Model) newsletterBody(tw int) []string {
	lines := []string{m.st.title.Render("Newsletter"), ""}
	lines = append(lines, styleLines(wrap("Every Monday we send a roundup of the week's posts, simulators and learning resources. One email a week, no spam, unsubscribe anytime.", tw), m.st.text)...)
	lines = append(lines, "", m.st.text.Render("Subscribe on the site:"))
	lines = append(lines, styleLines(hardWrap(siteURL+"/newsletters", tw), m.st.accent)...)
	lines = append(lines, "")
	lines = append(lines, styleLines(wrap("This app never asks for your email. Sign-up happens on the site only.", tw), m.st.muted)...)
	lines = append(lines, "")
	lines = append(lines, styleLines(wrap("Prefer a feed reader? "+siteURL+"/feed.xml", tw), m.st.muted)...)
	return m.withOpenNote(lines, tw)
}

func (m Model) withOpenNote(lines []string, tw int) []string {
	if m.openNote == "" {
		return lines
	}
	lines = append(lines, "")
	return append(lines, styleLines(wrap(m.openNote, tw), m.st.muted)...)
}

func (m Model) openURL() string {
	switch m.screen {
	case screenPost:
		if len(m.posts) > 0 {
			return m.posts[m.postIdx].URL
		}
	case screenQuiz:
		return m.quiz.URL()
	case screenIncident:
		if m.incidentShown {
			return m.incident.LinkURL
		}
	case screenNewsletter:
		return siteURL + "/newsletters"
	}
	return ""
}

var links = []struct{ label, url, desc string }{
	{"Site", siteURL, "Posts, guides, quizzes and news"},
	{"Simulators", siteURL + "/simulators", "Hands-on DevOps simulators in your browser"},
	{"Opsville", siteURL + "/opsville/", "A free DevOps town game"},
	{"Roadmap", siteURL + "/roadmap", "A step-by-step DevOps learning path"},
	{"GitHub", "https://github.com/The-DevOps-Daily", "Open source projects and challenges"},
}

func (m Model) linksBody(tw int) []string {
	lines := []string{m.st.title.Render("Links"), ""}
	for _, l := range links {
		lines = append(lines, m.st.bold.Render(padRight(l.label, 12))+m.st.accent.Render(l.url))
		lines = append(lines, strings.Repeat(" ", 12)+m.st.muted.Render(l.desc))
	}
	lines = append(lines, "")
	lines = append(lines, styleLines(wrap("Want to sponsor DevOps Daily? "+siteURL+"/sponsorship", tw), m.st.muted)...)
	return lines
}

var bulletRE = regexp.MustCompile(`^\s*(?:[-*•]|\d+[.)]|[A-Da-d]\))\s+`)

// wrap word-wraps plain text to w columns. Paragraph breaks are kept, and
// list items get a hanging indent.
func wrap(s string, w int) []string {
	w = max(w, 8)
	var out []string
	for _, para := range strings.Split(s, "\n") {
		para = strings.TrimRight(para, " ")
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		prefix := bulletRE.FindString(para)
		indent := strings.Repeat(" ", ansi.StringWidth(prefix))
		rest := para[len(prefix):]
		for i, l := range strings.Split(ansi.Wrap(rest, max(4, w-len(indent)), ""), "\n") {
			if i == 0 {
				out = append(out, prefix+l)
			} else {
				out = append(out, indent+l)
			}
		}
	}
	return out
}

// Wrap exposes the shared plain-text wrapping used by the terminal screens.
func Wrap(s string, w int) []string { return wrap(s, w) }

// hardWrap breaks lines at exactly w columns. Used for code and URLs.
func hardWrap(s string, w int) []string {
	w = max(w, 8)
	var out []string
	for _, l := range strings.Split(s, "\n") {
		out = append(out, strings.Split(ansi.Hardwrap(l, w, true), "\n")...)
	}
	return out
}

// HardWrap exposes the shared hard wrapping used for code and URLs.
func HardWrap(s string, w int) []string { return hardWrap(s, w) }

func styleLines(lines []string, st lipgloss.Style) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		if l != "" {
			out[i] = st.Render(l)
		}
	}
	return out
}

func padRight(s string, w int) string {
	if n := ansi.StringWidth(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// PadRight exposes display-width-aware padding for terminal layouts.
func PadRight(s string, w int) string { return padRight(s, w) }

func human(d time.Duration) string {
	if d%time.Minute == 0 && d >= time.Minute {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	return d.String()
}
