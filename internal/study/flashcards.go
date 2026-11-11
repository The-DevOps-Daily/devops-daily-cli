package study

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/srs"
)

type FlashOptions struct {
	Options
	NewLimit int
	All      bool
}

type Flashcards struct {
	deck     catalog.Deck
	opt      FlashOptions
	st       styles
	width    int
	height   int
	queue    []string
	byID     map[string]catalog.Card
	state    map[string]srs.Card
	index    int
	flipped  bool
	reviewed int
	again    int
	requeued map[string]bool
	dueCount int
	newCount int
	scroll   int
	quitting bool
}

func NewFlashcards(d catalog.Deck, opt FlashOptions) Flashcards {
	if opt.NewLimit == 0 {
		opt.NewLimit = 20
	}
	state := make(map[string]srs.Card)
	if opt.Progress != nil {
		state = opt.Progress.Cards(d.ID)
	}
	ids := make([]string, len(d.Cards))
	byID := make(map[string]catalog.Card, len(d.Cards))
	for i, card := range d.Cards {
		ids[i] = card.ID
		byID[card.ID] = card
	}
	due, fresh := srs.Queue(ids, state, now(opt.Now), opt.NewLimit, opt.All)
	newCount := 0
	for _, id := range fresh {
		if state[id].Box == 0 {
			newCount++
		}
	}
	return Flashcards{
		deck:     d,
		opt:      opt,
		st:       newStyles(opt.Renderer),
		width:    80,
		height:   24,
		queue:    append(due, fresh...),
		byID:     byID,
		state:    state,
		requeued: make(map[string]bool),
		dueCount: len(due),
		newCount: newCount,
	}
}

func (m Flashcards) Init() tea.Cmd { return nil }

func (m Flashcards) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if tooSmall(m.width, m.height) || m.index >= len(m.queue) {
			return m, nil
		}
		if !m.flipped && (key == " " || key == "enter") {
			m.flipped = true
			m.scroll = 0
			return m, nil
		}
		if m.flipped {
			switch key {
			case "1":
				m.grade(srs.Again)
				return m, nil
			case "2":
				m.grade(srs.Hard)
				return m, nil
			case "3":
				m.grade(srs.Good)
				return m, nil
			case "4":
				m.grade(srs.Easy)
				return m, nil
			}
		}
		m.scroll = scrollKey(m.scroll, len(m.body()), m.bodyHeight(), key)
	}
	return m, nil
}

func (m Flashcards) View() string {
	if m.quitting {
		return ""
	}
	if len(m.queue) == 0 {
		return m.emptyView()
	}
	if m.index >= len(m.queue) {
		return m.resultView()
	}
	left := m.queue[m.index:]
	fresh := 0
	for _, id := range left {
		if m.state[id].Box == 0 {
			fresh++
		}
	}
	sub := fmt.Sprintf("%d left · %d due · %d new", len(left), len(left)-fresh, fresh)
	return renderScreen(m.width, m.height, m.scroll, m.deck.Title, sub, m.opt.Sponsor, m.st, m.body(), m.help())
}

func (m Flashcards) emptyView() string {
	ids := make([]string, len(m.deck.Cards))
	for i, card := range m.deck.Cards {
		ids[i] = card.ID
	}
	next, ok := srs.NextDue(ids, m.state)
	line := "All cards are new; use --all to cram."
	if ok {
		line = fmt.Sprintf("Nothing due in %s today. Next card is due on %s.", m.deck.Title, next)
	}
	lines := []string{m.st.muted.Render(line), "", m.st.muted.Render("Cram: devops-daily flashcards " + m.deck.ID + " --all")}
	return renderScreen(m.width, m.height, 0, m.deck.Title, "Flashcards", m.opt.Sponsor, m.st, lines, help(m.st, [2]string{"q", "quit"}))
}

func (m Flashcards) resultView() string {
	ids := make([]string, len(m.deck.Cards))
	for i, card := range m.deck.Cards {
		ids[i] = card.ID
	}
	lines := []string{m.st.title.Render(fmt.Sprintf("Reviewed %d cards (%d again).", m.reviewed, m.again))}
	if next, ok := srs.NextDue(ids, m.state); ok {
		lines = append(lines, m.st.muted.Render("Next card due on "+next+"."))
	}
	return renderScreen(m.width, m.height, 0, m.deck.Title, "Session complete", m.opt.Sponsor, m.st, lines, help(m.st, [2]string{"q", "quit"}))
}

func (m Flashcards) body() []string {
	if m.index >= len(m.queue) {
		return nil
	}
	card := m.byID[m.queue[m.index]]
	width := frameWidth(m.width) - 4
	lines := []string{m.st.muted.Render("Front"), ""}
	lines = append(lines, tuiLines(card.Front, width, m.st.bold)...)
	if !m.flipped {
		lines = append(lines, "", m.st.muted.Render("Press space to flip."))
		return lines
	}
	lines = append(lines, "", m.st.muted.Render("Back"), "")
	lines = append(lines, tuiLines(card.Back, width, m.st.bold)...)
	return lines
}

func (m Flashcards) help() string {
	if m.flipped {
		return help(m.st, [2]string{"1", "again"}, [2]string{"2", "hard"}, [2]string{"3", "good"}, [2]string{"4", "easy"}, [2]string{"q", "quit"})
	}
	return help(m.st, [2]string{"space", "flip"}, [2]string{"↑/↓", "scroll"}, [2]string{"q", "quit"})
}

func (m *Flashcards) grade(grade srs.Grade) {
	id := m.queue[m.index]
	next := srs.Review(m.state[id], grade, now(m.opt.Now))
	m.state[id] = next
	if m.opt.Progress != nil {
		_ = m.opt.Progress.SaveCard(m.deck.ID, id, next)
	}
	m.reviewed++
	if grade == srs.Again {
		m.again++
		if !m.requeued[id] {
			m.queue = append(m.queue, id)
			m.requeued[id] = true
		}
	}
	m.index++
	m.flipped = false
	m.scroll = 0
}

func (m Flashcards) bodyHeight() int { return max(1, m.height-5) }
