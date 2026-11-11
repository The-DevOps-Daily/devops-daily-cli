package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/content"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/text"
)

func (s *Store) load(name string, out any) error {
	b, err := os.ReadFile(filepath.Join(s.Dir, filepath.FromSlash(name)))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cached content is missing %s", name)
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	return nil
}

func (s *Store) Manifest() (*Manifest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.manifest == nil {
		var v Manifest
		if err := s.load("index.json", &v); err != nil {
			return nil, err
		}
		if v.Schema != Schema {
			return nil, ErrNewerSchema
		}
		s.manifest = &v
	}
	return s.manifest, nil
}

func (s *Store) Posts() ([]PostMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.posts == nil {
		var v PostsIndex
		if err := s.load("posts.json", &v); err != nil {
			return nil, err
		}
		if v.Schema != Schema {
			return nil, ErrNewerSchema
		}
		cleanPosts(v.Posts)
		s.posts = &v
	}
	return s.posts.Posts, nil
}

func (s *Store) PostBody(meta PostMeta) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !fileNameRE.MatchString(meta.Body) {
		return "", fmt.Errorf("invalid post body file %q", meta.Body)
	}
	if s.bodies == nil {
		s.bodies = make(map[string]PostBodies)
	}
	bodies, ok := s.bodies[meta.Body]
	if !ok {
		if err := s.load(meta.Body, &bodies); err != nil {
			return "", err
		}
		if bodies.Schema != Schema {
			return "", ErrNewerSchema
		}
		s.bodies[meta.Body] = bodies
	}
	body, ok := bodies.Posts[meta.Slug]
	if !ok {
		return "", fmt.Errorf("post body for %s is missing", meta.Slug)
	}
	return text.Clean(body, true), nil
}

func (s *Store) Quizzes() ([]Quiz, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.quiz == nil {
		var v Quizzes
		if err := s.load("quizzes.json", &v); err != nil {
			return nil, err
		}
		if v.Schema != Schema {
			return nil, ErrNewerSchema
		}
		cleanQuizzes(v.Quizzes)
		s.quiz = &v
	}
	return s.quiz.Quizzes, nil
}
func (s *Store) Decks() ([]Deck, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.decks == nil {
		var v Decks
		if err := s.load("flashcards.json", &v); err != nil {
			return nil, err
		}
		if v.Schema != Schema {
			return nil, ErrNewerSchema
		}
		cleanDecks(v.Decks)
		s.decks = &v
	}
	return s.decks.Decks, nil
}
func (s *Store) Checklists() ([]Checklist, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.checks == nil {
		var v Checklists
		if err := s.load("checklists.json", &v); err != nil {
			return nil, err
		}
		if v.Schema != Schema {
			return nil, ErrNewerSchema
		}
		cleanChecks(v.Checklists)
		s.checks = &v
	}
	return s.checks.Checklists, nil
}
func (s *Store) Incidents() ([]Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.incidents == nil {
		var v Incidents
		if err := s.load("incidents.json", &v); err != nil {
			return nil, err
		}
		if v.Schema != Schema {
			return nil, ErrNewerSchema
		}
		cleanIncidents(v.Incidents)
		s.incidents = &v
	}
	return s.incidents.Incidents, nil
}
func (s *Store) Games() ([]Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.games == nil {
		var v Games
		if err := s.load("games.json", &v); err != nil {
			return nil, err
		}
		if v.Schema != Schema {
			return nil, ErrNewerSchema
		}
		cleanGames(v.Games)
		s.games = &v
	}
	return s.games.Games, nil
}
func (s *Store) Sponsors() ([]Sponsor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sponsors == nil {
		var v Sponsors
		if err := s.load("sponsors.json", &v); err != nil {
			return nil, err
		}
		if v.Schema != Schema {
			return nil, ErrNewerSchema
		}
		cleanSponsors(v.Sponsors)
		s.sponsors = &v
	}
	return s.sponsors.Sponsors, nil
}

func SponsorOn(sponsors []Sponsor, t time.Time) (Sponsor, bool) {
	if len(sponsors) == 0 {
		return Sponsor{}, false
	}
	n := content.DayNumber(t) % int64(len(sponsors))
	if n < 0 {
		n += int64(len(sponsors))
	}
	return sponsors[n], true
}
