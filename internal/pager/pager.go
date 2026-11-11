// Package pager provides the full-screen article reader.
package pager

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	title         string
	view          viewport.Model
	width, height int
}

// Run opens content in an alternate-screen pager.
func Run(title, content string) error {
	m := model{title: title, view: viewport.New(80, 22), width: 80, height: 24}
	m.view.SetContent(content)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m model) Init() tea.Cmd { return nil }
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = x.Width, x.Height
		m.view.Width, m.view.Height = x.Width, max(1, x.Height-2)
	case tea.KeyMsg:
		switch x.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "g", "home":
			m.view.GotoTop()
		case "G", "end":
			m.view.GotoBottom()
		case "pgup", "b":
			m.view.PageUp()
		case "pgdown", " ":
			m.view.PageDown()
		case "ctrl+u":
			m.view.HalfPageUp()
		case "ctrl+d":
			m.view.HalfPageDown()
		case "up", "k":
			m.view.ScrollUp(1)
		case "down", "j":
			m.view.ScrollDown(1)
		}
	}
	return m, nil
}
func (m model) View() string {
	title := "\x1b[1m" + m.title + "\x1b[0m"
	footer := "\x1b[38;2;161;161;170m↑/↓ scroll · space/b page · g/G top/bottom · q quit\x1b[0m"
	return fmt.Sprintf("%s\n%s\n%s  %3.0f%%", title, m.view.View(), footer, m.view.ScrollPercent()*100)
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
