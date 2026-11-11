// Package progress stores local study progress.
package progress

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/srs"
)

const version = 1

type QuizScore struct {
	Best      int       `json:"best"`
	Total     int       `json:"total"`
	Last      int       `json:"last"`
	LastTotal int       `json:"lastTotal"`
	Played    int       `json:"played"`
	Updated   time.Time `json:"updated"`
}

type checklist struct {
	Done    []string  `json:"done"`
	Updated time.Time `json:"updated"`
}

type document struct {
	Version    int                            `json:"version"`
	Quizzes    map[string]QuizScore           `json:"quizzes"`
	Flashcards map[string]map[string]srs.Card `json:"flashcards"`
	Checklists map[string]checklist           `json:"checklists"`
}

type Store struct {
	path    string
	doc     document
	warning string
	mu      sync.Mutex
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, doc: empty()}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.doc); err != nil || s.doc.Version != version {
		bad := fmt.Sprintf("%s.bad-%d", path, time.Now().Unix())
		if renameErr := os.Rename(path, bad); renameErr != nil {
			return nil, renameErr
		}
		s.doc = empty()
		s.warning = "Progress file was unreadable and was reset."
		return s, nil
	}
	s.ensureMaps()
	return s, nil
}

func (s *Store) Warning() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	warning := s.warning
	s.warning = ""
	return warning
}

func (s *Store) Quiz(id string) (QuizScore, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.doc.Quizzes[id]
	return v, ok
}

func (s *Store) SaveQuiz(id string, score, total int, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.doc.Quizzes[id]
	if v.Total == 0 || score*max(1, v.Total) > v.Best*max(1, total) {
		v.Best = score
		v.Total = total
	}
	v.Last = score
	v.LastTotal = total
	v.Played++
	v.Updated = now.UTC()
	s.doc.Quizzes[id] = v
	return s.writeLocked()
}

func (s *Store) Cards(deck string) map[string]srs.Card {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]srs.Card)
	for id, card := range s.doc.Flashcards[deck] {
		result[id] = card
	}
	return result
}

func (s *Store) SaveCard(deck, card string, value srs.Card) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.doc.Flashcards[deck] == nil {
		s.doc.Flashcards[deck] = make(map[string]srs.Card)
	}
	s.doc.Flashcards[deck][card] = value
	return s.writeLocked()
}

func (s *Store) ChecklistDone(id string) map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]bool)
	for _, item := range s.doc.Checklists[id].Done {
		result[item] = true
	}
	return result
}

func (s *Store) SetChecklistItem(id, item string, done bool, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.doc.Checklists[id]
	items := make(map[string]bool, len(v.Done)+1)
	for _, value := range v.Done {
		items[value] = true
	}
	if done {
		items[item] = true
	} else {
		delete(items, item)
	}
	v.Done = keys(items)
	v.Updated = now.UTC()
	s.doc.Checklists[id] = v
	return s.writeLocked()
}

func (s *Store) ResetChecklist(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.doc.Checklists, id)
	return s.writeLocked()
}

func (s *Store) ensureMaps() {
	if s.doc.Quizzes == nil {
		s.doc.Quizzes = make(map[string]QuizScore)
	}
	if s.doc.Flashcards == nil {
		s.doc.Flashcards = make(map[string]map[string]srs.Card)
	}
	if s.doc.Checklists == nil {
		s.doc.Checklists = make(map[string]checklist)
	}
}

func (s *Store) writeLocked() error {
	s.doc.Version = version
	s.ensureMaps()
	b, err := json.Marshal(s.doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".progress-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(s.path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func empty() document {
	return document{
		Version:    version,
		Quizzes:    make(map[string]QuizScore),
		Flashcards: make(map[string]map[string]srs.Card),
		Checklists: make(map[string]checklist),
	}
}

func keys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
