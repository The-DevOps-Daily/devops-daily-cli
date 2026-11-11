package srs

import (
	"reflect"
	"testing"
	"time"
)

func TestReview(t *testing.T) {
	today := time.Date(2026, 11, 11, 15, 0, 0, 0, time.FixedZone("x", 3600))
	tests := []struct {
		name  string
		card  Card
		grade Grade
		box   int
		due   string
		laps  int
	}{
		{"new again", Card{}, Again, 1, "2026-11-11", 1},
		{"new hard", Card{}, Hard, 1, "2026-11-12", 0},
		{"new good", Card{}, Good, 1, "2026-11-12", 0},
		{"new easy", Card{}, Easy, 2, "2026-11-14", 0},
		{"box three again", Card{Box: 3}, Again, 1, "2026-11-11", 1},
		{"box three hard", Card{Box: 3}, Hard, 3, "2026-11-12", 0},
		{"box three good", Card{Box: 3}, Good, 4, "2026-11-25", 0},
		{"box three easy", Card{Box: 3}, Easy, 5, "2026-12-11", 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Review(test.card, test.grade, today)
			if got.Box != test.box || got.Due != test.due || got.Lapses != test.laps || got.Reviews != test.card.Reviews+1 {
				t.Fatalf("Review() = %#v", got)
			}
		})
	}
}

func TestQueueAndNextDue(t *testing.T) {
	ids := []string{"old", "due-a", "new-a", "due-b", "later", "new-b"}
	state := map[string]Card{
		"old":   {Box: 2, Due: "2026-11-01"},
		"due-a": {Box: 2, Due: "2026-11-10"},
		"due-b": {Box: 2, Due: "2026-11-10"},
		"later": {Box: 2, Due: "2026-11-14"},
	}
	today := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	due, fresh := Queue(ids, state, today, 1, false)
	if !reflect.DeepEqual(due, []string{"old", "due-a", "due-b"}) || !reflect.DeepEqual(fresh, []string{"new-a"}) {
		t.Fatalf("Queue() = %v, %v", due, fresh)
	}
	due, fresh = Queue(ids, state, today, 1, true)
	if !reflect.DeepEqual(due, []string{"old", "due-a", "due-b"}) || !reflect.DeepEqual(fresh, []string{"new-a", "later", "new-b"}) {
		t.Fatalf("Queue(all) = %v, %v", due, fresh)
	}
	state["later"] = Card{Box: 2, Due: time.Now().UTC().AddDate(0, 0, 2).Format("2006-01-02")}
	if next, ok := NextDue(ids, state); !ok || next != state["later"].Due {
		t.Fatalf("NextDue() = %q, %t", next, ok)
	}
}
