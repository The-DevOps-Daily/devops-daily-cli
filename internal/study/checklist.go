package study

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
)

type ChecklistOptions struct {
	Options
}

type Checklist struct {
	checklist catalog.Checklist
	opt       ChecklistOptions
	st        styles
	width     int
	height    int
	done      map[string]bool
	cursor    int
	showDone  bool
	details   map[string]bool
	resetting bool
	scroll    int
	quitting  bool
}

func NewChecklist(c catalog.Checklist, opt ChecklistOptions) Checklist {
	done := make(map[string]bool)
	if opt.Progress != nil {
		done = opt.Progress.ChecklistDone(c.ID)
	}
	return Checklist{
		checklist: c,
		opt:       opt,
		st:        newStyles(opt.Renderer),
		width:     80,
		height:    24,
		done:      done,
		showDone:  true,
		details:   make(map[string]bool),
	}
}

func (m Checklist) Init() tea.Cmd { return nil }

func (m Checklist) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch value := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = size(value.Width, value.Height)
		return m, nil
	case tea.KeyMsg:
		key := value.String()
		if key == "q" || key == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		if tooSmall(m.width, m.height) {
			return m, nil
		}
		if m.resetting {
			switch key {
			case "y", "Y":
				m.done = make(map[string]bool)
				if m.opt.Progress != nil {
					_ = m.opt.Progress.ResetChecklist(m.checklist.ID)
				}
				m.resetting = false
			case "n", "N", "esc":
				m.resetting = false
			}
			return m, nil
		}
		items := m.visible()
		switch key {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(items)-1 {
				m.cursor++
			}
		case " ", "x":
			if len(items) > 0 {
				m.toggle(items[m.cursor])
			}
		case "enter":
			if len(items) > 0 {
				id := items[m.cursor].ID
				m.details[id] = !m.details[id]
			}
		case "u":
			m.showDone = !m.showDone
			m.cursor = 0
		case "R":
			m.resetting = true
		default:
			m.scroll = scrollKey(m.scroll, len(m.body()), m.bodyHeight(), key)
		}
	}
	return m, nil
}

func (m Checklist) View() string {
	if m.quitting {
		return ""
	}
	if m.resetting {
		lines := []string{m.st.bad.Render(fmt.Sprintf("Reset all %d items? y/n", len(m.checklist.Items)))}
		return renderScreen(m.width, m.height, 0, m.checklist.Title, m.progress(), m.opt.Sponsor, m.st, lines, help(m.st, [2]string{"y/n", "confirm"}, [2]string{"q", "quit"}))
	}
	return renderScreen(m.width, m.height, m.scroll, m.checklist.Title, m.progress(), m.opt.Sponsor, m.st, m.body(), m.help())
}

func (m Checklist) body() []string {
	width := frameWidth(m.width) - 4
	lines := []string{}
	for i, item := range m.visible() {
		cursor := "  "
		if i == m.cursor {
			cursor = m.st.accent.Render("▸ ")
		}
		mark := "[ ]"
		if m.done[item.ID] {
			mark = "[x]"
		}
		critical := ""
		if item.Critical {
			critical = m.st.accent.Render("! ")
		}
		lines = append(lines, cursor+m.st.muted.Render(mark+" ")+critical+m.st.bold.Render(item.Title))
		if m.details[item.ID] {
			if item.Description != "" {
				for _, line := range tuiLines(item.Description, max(8, width-4), m.st.muted) {
					lines = append(lines, "    "+line)
				}
			}
			for _, block := range item.Code {
				if block.Label != "" {
					lines = append(lines, "    "+m.st.muted.Render(block.Label))
				}
				for _, line := range codeLines(block.Code, max(8, width-4), m.st) {
					lines = append(lines, "    "+line)
				}
			}
		}
	}
	if len(lines) == 0 {
		lines = append(lines, m.st.muted.Render("All items are done. Press u to show them."))
	}
	lines = append(lines, "", m.st.muted.Render("! critical"))
	if m.checklist.URL != "" {
		lines = append(lines, m.st.muted.Render("On the site: ")+m.st.accent.Render(m.checklist.URL))
	}
	return lines
}

func (m Checklist) help() string {
	return help(m.st, [2]string{"↑/↓", "move"}, [2]string{"space", "toggle"}, [2]string{"enter", "details"}, [2]string{"u", "done"}, [2]string{"R", "reset"}, [2]string{"q", "quit"})
}

func (m Checklist) progress() string {
	done := 0
	for _, item := range m.checklist.Items {
		if m.done[item.ID] {
			done++
		}
	}
	barWidth := 10
	filled := 0
	if len(m.checklist.Items) > 0 {
		filled = done * barWidth / len(m.checklist.Items)
	}
	return fmt.Sprintf("%d of %d done %s", done, len(m.checklist.Items), strings.Repeat("■", filled)+strings.Repeat("□", barWidth-filled))
}

func (m Checklist) visible() []catalog.ChecklistItem {
	items := make([]catalog.ChecklistItem, 0, len(m.checklist.Items))
	for _, item := range m.checklist.Items {
		if m.showDone || !m.done[item.ID] {
			items = append(items, item)
		}
	}
	return items
}

func (m *Checklist) toggle(item catalog.ChecklistItem) {
	next := !m.done[item.ID]
	m.done[item.ID] = next
	if m.opt.Progress != nil {
		_ = m.opt.Progress.SetChecklistItem(m.checklist.ID, item.ID, next, now(m.opt.Now))
	}
}

func (m Checklist) bodyHeight() int { return max(1, m.height-5) }
