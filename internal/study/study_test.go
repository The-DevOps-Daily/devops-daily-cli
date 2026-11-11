package study

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/progress"
)

func TestQuizFlow(t *testing.T) {
	store, err := progress.Open(filepath.Join(t.TempDir(), "progress.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewQuiz(catalog.Quiz{ID: "quiz", Title: "Quiz", Questions: []catalog.Question{{Question: "Question", Options: []string{"no", "yes"}, Correct: 1, Explanation: "Because"}}}, QuizOptions{Options: Options{Progress: store, Now: fixedNow}})
	m = resizeQuiz(t, m, 80, 24)
	m = keyQuiz(t, m, "b")
	if !strings.Contains(m.View(), "Correct!") {
		t.Fatal("answer was not shown")
	}
	if score, ok := store.Quiz("quiz"); !ok || score.Last != 1 {
		t.Fatalf("saved score = %#v, %t", score, ok)
	}
	m = keyQuiz(t, m, "enter")
	if !strings.Contains(m.View(), "1 of 1 (100%)") {
		t.Fatal("result was not shown")
	}
	m = keyQuiz(t, m, "r")
	if !strings.Contains(m.View(), "Question 1 of 1") {
		t.Fatal("retry did not restart")
	}
	fit(t, m.View(), 80)
}

func TestFlashcardsAndChecklist(t *testing.T) {
	store, err := progress.Open(filepath.Join(t.TempDir(), "progress.json"))
	if err != nil {
		t.Fatal(err)
	}
	flash := NewFlashcards(catalog.Deck{ID: "deck", Title: "Deck", Cards: []catalog.Card{{ID: "card", Front: "front", Back: "back"}}}, FlashOptions{Options: Options{Progress: store, Now: fixedNow}})
	flash = resizeFlash(t, flash, 80, 24)
	flash = keyFlash(t, flash, " ")
	if !strings.Contains(flash.View(), "back") {
		t.Fatal("flashcard did not flip")
	}
	flash = keyFlash(t, flash, "1")
	flash = keyFlash(t, flash, " ")
	flash = keyFlash(t, flash, "3")
	if store.Cards("deck")["card"].Reviews != 2 {
		t.Fatal("flashcard grade was not saved")
	}
	check := NewChecklist(catalog.Checklist{ID: "check", Title: "Check", Items: []catalog.ChecklistItem{{ID: "one", Title: "One", Description: "Details", Critical: true}}}, ChecklistOptions{Options: Options{Progress: store, Now: fixedNow}})
	check = resizeChecklist(t, check, 40, 12)
	check = keyChecklist(t, check, " ")
	if !store.ChecklistDone("check")["one"] {
		t.Fatal("checklist item was not saved")
	}
	check = keyChecklist(t, check, "enter")
	if !strings.Contains(check.View(), "Details") {
		t.Fatal("details were not shown")
	}
	check = keyChecklist(t, check, "R")
	check = keyChecklist(t, check, "y")
	if store.ChecklistDone("check")["one"] {
		t.Fatal("checklist was not reset")
	}
	fit(t, flash.View(), 80)
	fit(t, check.View(), 40)
}

func fixedNow() time.Time { return time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC) }

func keyMsg(value string) tea.KeyMsg {
	if value == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
}

func resizeQuiz(t *testing.T, m Quiz, width, height int) Quiz {
	t.Helper()
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(Quiz)
}

func keyQuiz(t *testing.T, m Quiz, key string) Quiz {
	t.Helper()
	next, _ := m.Update(keyMsg(key))
	return next.(Quiz)
}

func resizeFlash(t *testing.T, m Flashcards, width, height int) Flashcards {
	t.Helper()
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(Flashcards)
}

func keyFlash(t *testing.T, m Flashcards, key string) Flashcards {
	t.Helper()
	next, _ := m.Update(keyMsg(key))
	return next.(Flashcards)
}

func resizeChecklist(t *testing.T, m Checklist, width, height int) Checklist {
	t.Helper()
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(Checklist)
}

func keyChecklist(t *testing.T, m Checklist, key string) Checklist {
	t.Helper()
	next, _ := m.Update(keyMsg(key))
	return next.(Checklist)
}

func fit(t *testing.T, view string, width int) {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > width {
			t.Fatalf("line width %d exceeds %d: %q", ansi.StringWidth(line), width, line)
		}
	}
}
