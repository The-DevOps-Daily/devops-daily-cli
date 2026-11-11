// Package content holds the quiz questions and incidents baked into the
// binary at build time, and picks one of each per UTC day so every visitor
// sees the same ones.
package content

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SiteURL is the public site.
const SiteURL = "https://devops-daily.com"

//go:embed snapshot.json
var snapshotJSON []byte

// Question is one multiple-choice quiz question.
type Question struct {
	QuizID      string   `json:"quizId"`
	QuizTitle   string   `json:"quizTitle"`
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Situation   string   `json:"situation,omitempty"`
	Code        string   `json:"code,omitempty"`
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	Correct     int      `json:"correct"`
	Explanation string   `json:"explanation"`
	Difficulty  string   `json:"difficulty,omitempty"`
}

// URL is the full quiz page on the site.
func (q Question) URL() string { return SiteURL + "/quizzes/" + q.QuizID }

// Option is one lettered answer.
type Option struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// Incident is one real-world problem with its fix.
type Incident struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Topic        string   `json:"topic"`
	Difficulty   string   `json:"difficulty"`
	Technologies []string `json:"technologies"`
	Scenario     string   `json:"scenario"`
	Options      []Option `json:"options"`
	Correct      string   `json:"correct"`
	Fix          string   `json:"fix"`
	Lesson       string   `json:"lesson"`
	LinkTitle    string   `json:"linkTitle"`
	LinkURL      string   `json:"linkUrl"`
}

// CorrectOption returns the option marked correct.
func (i Incident) CorrectOption() Option {
	for _, o := range i.Options {
		if o.Key == i.Correct {
			return o
		}
	}
	return Option{}
}

// Snapshot is the baked content.
type Snapshot struct {
	Generated string     `json:"generated"`
	Questions []Question `json:"questions"`
	Incidents []Incident `json:"incidents"`

	questionOrder []int
	incidentOrder []int
}

// Salts keep the question and incident rotations independent.
const (
	questionSalt = 0x51a7e0f1d0c0ffee
	incidentSalt = 0x1c1de7700dd0a11
)

// Load parses the embedded snapshot.
func Load() (*Snapshot, error) { return Parse(snapshotJSON) }

// Parse parses and checks a snapshot.
func Parse(b []byte) (*Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("content snapshot: %w", err)
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	s.questionOrder = permutation(len(s.Questions), questionSalt)
	s.incidentOrder = permutation(len(s.Incidents), incidentSalt)
	return &s, nil
}

func (s *Snapshot) validate() error {
	if len(s.Questions) == 0 || len(s.Incidents) == 0 {
		return fmt.Errorf("content snapshot: %d questions, %d incidents; need at least one of each", len(s.Questions), len(s.Incidents))
	}
	for _, q := range s.Questions {
		if q.QuizID == "" || q.Question == "" || len(q.Options) < 2 || q.Correct < 0 || q.Correct >= len(q.Options) {
			return fmt.Errorf("content snapshot: bad question %s/%s", q.QuizID, q.ID)
		}
	}
	for _, in := range s.Incidents {
		if in.Title == "" || in.Scenario == "" || in.Fix == "" || in.CorrectOption().Key == "" || !strings.HasPrefix(in.LinkURL, "https://") {
			return fmt.Errorf("content snapshot: bad incident %s", in.ID)
		}
	}
	return nil
}

// DayNumber is the number of whole days from 1970-01-01 to t's UTC date.
func DayNumber(t time.Time) int64 {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400
}

// QuestionFor returns the question for t's UTC date.
func (s *Snapshot) QuestionFor(t time.Time) Question {
	return s.Questions[pick(s.questionOrder, t)]
}

// IncidentFor returns the incident for t's UTC date.
func (s *Snapshot) IncidentFor(t time.Time) Incident {
	return s.Incidents[pick(s.incidentOrder, t)]
}

// pick walks a fixed shuffle one step per day, so consecutive days differ
// and every item comes up once per cycle.
func pick(order []int, t time.Time) int {
	n := int64(len(order))
	d := DayNumber(t) % n
	if d < 0 {
		d += n
	}
	return order[d]
}

// permutation returns a fixed shuffle of 0..n-1. It uses its own generator
// (splitmix64) so the order never depends on the Go version.
func permutation(n int, seed uint64) []int {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	state := seed
	for i := n - 1; i > 0; i-- {
		j := int(splitmix64(&state) % uint64(i+1))
		p[i], p[j] = p[j], p[i]
	}
	return p
}

func splitmix64(state *uint64) uint64 {
	*state += 0x9e3779b97f4a7c15
	z := *state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}
