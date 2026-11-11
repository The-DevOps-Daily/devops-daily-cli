package content

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestEmbeddedSnapshotLoads(t *testing.T) {
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Questions) < 100 || len(s.Incidents) < 5 {
		t.Fatalf("snapshot looks too small: %d questions, %d incidents", len(s.Questions), len(s.Incidents))
	}
	for _, q := range s.Questions {
		if q.URL() != SiteURL+"/quizzes/"+q.QuizID {
			t.Fatalf("bad quiz URL %s", q.URL())
		}
	}
}

func TestDayNumber(t *testing.T) {
	if got := DayNumber(time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)); got != 20767 {
		t.Fatalf("DayNumber(2026-11-10) = %d, want 20767", got)
	}
	// The UTC date counts, not the local one: 23:30 in New York on Nov 9 is
	// already Nov 10 in UTC.
	ny := time.FixedZone("EST", -5*3600)
	if got := DayNumber(time.Date(2026, 11, 9, 23, 30, 0, 0, ny)); got != 20767 {
		t.Fatalf("DayNumber in EST = %d, want 20767", got)
	}
}

// The permutation must match a fixed reference (computed independently),
// so a Go upgrade cannot reshuffle the daily picks.
func TestPermutationIsStable(t *testing.T) {
	if got, want := permutation(10, 42), []int{0, 9, 5, 8, 6, 4, 7, 2, 1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("permutation(10, 42) = %v, want %v", got, want)
	}
	if got, want := permutation(5, questionSalt), []int{2, 1, 0, 4, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("permutation(5, questionSalt) = %v, want %v", got, want)
	}
}

func testSnapshot(nq, ni int) *Snapshot {
	s := &Snapshot{}
	for i := 0; i < nq; i++ {
		s.Questions = append(s.Questions, Question{QuizID: "q", ID: fmt.Sprint(i), Question: "?", Options: []string{"a", "b"}})
	}
	for i := 0; i < ni; i++ {
		s.Incidents = append(s.Incidents, Incident{ID: fmt.Sprint(i), Title: "t", Scenario: "s", Fix: "f",
			Options: []Option{{Key: "A", Text: "x"}}, Correct: "A", LinkURL: "https://devops-daily.com/x"})
	}
	s.questionOrder = permutation(nq, questionSalt)
	s.incidentOrder = permutation(ni, incidentSalt)
	return s
}

func TestSamePickAllDay(t *testing.T) {
	s := testSnapshot(50, 7)
	morning := time.Date(2026, 11, 10, 0, 0, 1, 0, time.UTC)
	night := time.Date(2026, 11, 10, 23, 59, 59, 0, time.UTC)
	if s.QuestionFor(morning).ID != s.QuestionFor(night).ID {
		t.Fatal("question changed during the day")
	}
	if s.IncidentFor(morning).ID != s.IncidentFor(night).ID {
		t.Fatal("incident changed during the day")
	}
}

func TestPicksCycleThroughEverything(t *testing.T) {
	s := testSnapshot(50, 7)
	start := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	seen := map[string]bool{}
	prev := ""
	for d := 0; d < 50; d++ {
		id := s.QuestionFor(start.AddDate(0, 0, d)).ID
		if id == prev {
			t.Fatalf("day %d repeats the previous question", d)
		}
		seen[id] = true
		prev = id
	}
	if len(seen) != 50 {
		t.Fatalf("50 days showed %d distinct questions, want 50", len(seen))
	}
	seenInc := map[string]bool{}
	for d := 0; d < 7; d++ {
		seenInc[s.IncidentFor(start.AddDate(0, 0, d)).ID] = true
	}
	if len(seenInc) != 7 {
		t.Fatalf("7 days showed %d distinct incidents, want 7", len(seenInc))
	}
}

func TestPickBeforeEpochDoesNotPanic(t *testing.T) {
	s := testSnapshot(3, 2)
	_ = s.QuestionFor(time.Date(1960, 1, 1, 0, 0, 0, 0, time.UTC))
	_ = s.IncidentFor(time.Date(1960, 1, 1, 0, 0, 0, 0, time.UTC))
}

func TestParseRejectsBadSnapshots(t *testing.T) {
	for name, doc := range map[string]string{
		"empty":         `{"questions":[],"incidents":[]}`,
		"answer range":  `{"questions":[{"quizId":"q","question":"?","options":["a","b"],"correct":2}],"incidents":[{"title":"t","scenario":"s","fix":"f","options":[{"key":"A","text":"x"}],"correct":"A","linkUrl":"https://x"}]}`,
		"incident link": `{"questions":[{"quizId":"q","question":"?","options":["a","b"],"correct":1}],"incidents":[{"title":"t","scenario":"s","fix":"f","options":[{"key":"A","text":"x"}],"correct":"A","linkUrl":"javascript:x"}]}`,
	} {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
