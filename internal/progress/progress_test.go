package progress

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/srs"
)

func TestStoreRoundTripAndBestScore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "progress.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	if err := store.SaveQuiz("quiz", 7, 10, now); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveQuiz("quiz", 8, 20, now); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCard("deck", "card", srs.Card{Box: 2, Due: "2026-11-14"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetChecklistItem("check", "item", true, now); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	score, ok := store.Quiz("quiz")
	if !ok || score.Best != 7 || score.Total != 10 || score.Last != 8 || score.LastTotal != 20 || score.Played != 2 {
		t.Fatalf("Quiz() = %#v, %t", score, ok)
	}
	if got := store.Cards("deck")["card"]; got.Box != 2 {
		t.Fatalf("Cards() = %#v", got)
	}
	if !store.ChecklistDone("check")["item"] {
		t.Fatal("checklist item was not saved")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %#o", info.Mode().Perm())
		}
	}
}

func TestCorruptRecoveryAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "progress.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Warning() == "" {
		t.Fatal("missing corrupt-file warning")
	}
	if matches, err := filepath.Glob(path + ".bad-*"); err != nil || len(matches) != 1 {
		t.Fatalf("backup = %v, %v", matches, err)
	}
	if err := store.SetChecklistItem("check", "item", true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetChecklist("check"); err != nil {
		t.Fatal(err)
	}
	if len(store.ChecklistDone("check")) != 0 {
		t.Fatal("checklist was not reset")
	}
}
