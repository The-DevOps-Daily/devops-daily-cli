package study

import (
	"fmt"
	"math/rand"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/tui"
)

type QuizOptions struct {
	Options
	Count   int
	Shuffle bool
	Seed    int64
}

type Quiz struct {
	quiz      catalog.Quiz
	questions []catalog.Question
	opt       QuizOptions
	st        styles
	width     int
	height    int
	index     int
	correct   int
	pick      int
	hint      bool
	finished  bool
	saved     bool
	scroll    int
	quitting  bool
}

func NewQuiz(q catalog.Quiz, opt QuizOptions) Quiz {
	questions := append([]catalog.Question(nil), q.Questions...)
	if opt.Shuffle {
		rand.New(rand.NewSource(opt.Seed)).Shuffle(len(questions), func(i, j int) {
			questions[i], questions[j] = questions[j], questions[i]
		})
	}
	if opt.Count > 0 && opt.Count < len(questions) {
		questions = questions[:opt.Count]
	}
	return Quiz{quiz: q, questions: questions, opt: opt, st: newStyles(opt.Renderer), width: 80, height: 24, pick: -1}
}

func (m Quiz) Init() tea.Cmd { return nil }

func (m Quiz) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if m.finished {
			if key == "r" {
				m.index = 0
				m.correct = 0
				m.pick = -1
				m.finished = false
				m.saved = false
				m.scroll = 0
			}
			return m, nil
		}
		if m.pick >= 0 {
			if key == "enter" || key == " " {
				m.index++
				m.pick = -1
				m.hint = false
				m.scroll = 0
				if m.index >= len(m.questions) {
					m.finished = true
					m.save()
				}
				return m, nil
			}
		} else if key == "h" && m.question().Hint != "" {
			m.hint = !m.hint
			return m, nil
		} else if pick := answer(key, len(m.question().Options)); pick >= 0 {
			m.pick = pick
			if pick == m.question().Correct {
				m.correct++
			}
			if m.index == len(m.questions)-1 {
				m.save()
			}
			return m, nil
		}
		m.scroll = scrollKey(m.scroll, len(m.body()), m.bodyHeight(), key)
	}
	return m, nil
}

func (m Quiz) View() string {
	if m.quitting {
		return ""
	}
	if m.finished {
		return m.resultView()
	}
	sub := fmt.Sprintf("Question %d of %d · %d correct", m.index+1, len(m.questions), m.correct)
	return renderScreen(m.width, m.height, m.scroll, m.quiz.Title, sub, m.opt.Sponsor, m.st, m.body(), m.help())
}

func (m Quiz) resultView() string {
	percent := 0
	if len(m.questions) > 0 {
		percent = m.correct * 100 / len(m.questions)
	}
	lines := []string{m.st.title.Render("Quiz complete"), "", m.st.bold.Render(fmt.Sprintf("%d of %d (%d%%)", m.correct, len(m.questions), percent))}
	if m.opt.Progress != nil {
		if score, ok := m.opt.Progress.Quiz(m.quiz.ID); ok {
			lines = append(lines, m.st.muted.Render(fmt.Sprintf("Best so far: %d/%d", score.Best, score.Total)))
		}
	}
	if m.quiz.URL != "" {
		lines = append(lines, "", m.st.muted.Render("Take it on the site: ")+m.st.accent.Render(m.quiz.URL))
	}
	return renderScreen(m.width, m.height, 0, m.quiz.Title, "Result", m.opt.Sponsor, m.st, lines, help(m.st, [2]string{"r", "retry"}, [2]string{"q", "quit"}))
}

func (m Quiz) body() []string {
	if len(m.questions) == 0 {
		return []string{m.st.muted.Render("This quiz has no questions.")}
	}
	q := m.question()
	width := frameWidth(m.width) - 4
	lines := []string{}
	if q.Title != "" {
		lines = append(lines, m.st.bold.Render(q.Title), "")
	}
	if q.Situation != "" {
		lines = append(lines, tuiLines(q.Situation, width, m.st.muted)...)
		lines = append(lines, "")
	}
	lines = append(lines, codeLines(q.Code, width, m.st)...)
	if q.Code != "" {
		lines = append(lines, "")
	}
	lines = append(lines, tuiLines(q.Question, width, m.st.bold)...)
	lines = append(lines, "")
	for i, option := range q.Options {
		lines = append(lines, m.optionLines(i, option, width)...)
	}
	if m.pick < 0 && m.hint {
		lines = append(lines, "", m.st.muted.Render("Hint: "+q.Hint))
	}
	if m.pick >= 0 {
		lines = append(lines, "")
		if m.pick == q.Correct {
			lines = append(lines, m.st.good.Render("Correct!"))
		} else {
			lines = append(lines, m.st.bad.Render(fmt.Sprintf("Not quite. The answer is %c).", 'a'+q.Correct)))
		}
		lines = append(lines, tuiLines(q.Explanation, width, m.st.muted)...)
		lines = append(lines, "", m.st.key.Render("enter")+m.st.muted.Render(": next"))
	}
	return lines
}

// optionLines draws one answer with a hanging indent. Options that are code
// (several lines) keep their line breaks.
func (m Quiz) optionLines(i int, option string, width int) []string {
	st, mark := m.st.bold, ""
	if m.pick >= 0 {
		switch {
		case i == m.question().Correct:
			st, mark = m.st.good, "  ✓"
		case i == m.pick:
			st, mark = m.st.bad, "  ✗"
		default:
			st = m.st.muted
		}
	}
	var body []string
	if strings.Contains(option, "\n") {
		body = tui.HardWrap(option, max(8, width-3))
	} else {
		body = tui.Wrap(option, max(8, width-3))
	}
	out := make([]string, len(body))
	for j, line := range body {
		prefix := "   "
		if j == 0 {
			prefix = fmt.Sprintf("%c) ", 'a'+i)
		}
		if j == len(body)-1 {
			line += mark
		}
		out[j] = st.Render(prefix + line)
	}
	return out
}

func (m Quiz) help() string {
	if m.pick >= 0 {
		return help(m.st, [2]string{"enter", "next"}, [2]string{"↑/↓", "scroll"}, [2]string{"q", "quit"})
	}
	items := [][2]string{{"a-d", "answer"}, {"↑/↓", "scroll"}, {"q", "quit"}}
	if m.question().Hint != "" {
		items = append(items[:1], append([][2]string{{"h", "hint"}}, items[1:]...)...)
	}
	return help(m.st, items...)
}

func (m *Quiz) save() {
	if m.saved || m.opt.Progress == nil {
		return
	}
	_ = m.opt.Progress.SaveQuiz(m.quiz.ID, m.correct, len(m.questions), now(m.opt.Now))
	m.saved = true
}

func (m Quiz) question() catalog.Question {
	if m.index >= 0 && m.index < len(m.questions) {
		return m.questions[m.index]
	}
	return catalog.Question{}
}

func (m Quiz) bodyHeight() int {
	return max(1, m.height-5)
}

func answer(key string, count int) int {
	if len(key) != 1 {
		return -1
	}
	var value int
	switch {
	case key[0] >= 'a' && key[0] <= 'd':
		value = int(key[0] - 'a')
	case key[0] >= '1' && key[0] <= '4':
		value = int(key[0] - '1')
	default:
		return -1
	}
	if value >= count {
		return -1
	}
	return value
}
