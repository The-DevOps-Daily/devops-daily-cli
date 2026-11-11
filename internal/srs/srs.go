// Package srs implements the local flashcard schedule.
package srs

import (
	"sort"
	"time"
)

type Card struct {
	Box     int    `json:"box"`
	Due     string `json:"due"`
	Reviews int    `json:"reviews"`
	Lapses  int    `json:"lapses"`
}

type Grade int

const (
	Again Grade = iota
	Hard
	Good
	Easy
)

func Review(c Card, g Grade, today time.Time) Card {
	today = utcDate(today)
	c.Reviews++
	switch g {
	case Again:
		c.Box = 1
		c.Lapses++
		c.Due = today.Format("2006-01-02")
	case Hard:
		if c.Box < 1 {
			c.Box = 1
		}
		c.Due = today.AddDate(0, 0, 1).Format("2006-01-02")
	case Easy:
		c.Box = min(c.Box+2, 6)
		c.Due = today.AddDate(0, 0, interval(c.Box)).Format("2006-01-02")
	default:
		c.Box = min(c.Box+1, 6)
		c.Due = today.AddDate(0, 0, interval(c.Box)).Format("2006-01-02")
	}
	return c
}

func Queue(ids []string, state map[string]Card, today time.Time, newLimit int, all bool) (due []string, fresh []string) {
	todayText := utcDate(today).Format("2006-01-02")
	type item struct {
		id    string
		due   string
		index int
	}
	var pending []item
	for i, id := range ids {
		card, known := state[id]
		if !known || card.Box == 0 {
			if !all && newLimit > 0 && len(fresh) < newLimit {
				fresh = append(fresh, id)
			}
			continue
		}
		if card.Due != "" && card.Due <= todayText {
			pending = append(pending, item{id: id, due: card.Due, index: i})
		}
	}
	sort.SliceStable(pending, func(i, j int) bool {
		if pending[i].due == pending[j].due {
			return pending[i].index < pending[j].index
		}
		return pending[i].due < pending[j].due
	})
	for _, card := range pending {
		due = append(due, card.id)
	}
	if all {
		queued := make(map[string]bool, len(due))
		for _, id := range due {
			queued[id] = true
		}
		for _, id := range ids {
			if !queued[id] {
				fresh = append(fresh, id)
			}
		}
	}
	return due, fresh
}

func NextDue(ids []string, state map[string]Card) (string, bool) {
	var next string
	today := utcDate(time.Now()).Format("2006-01-02")
	for _, id := range ids {
		card, ok := state[id]
		if !ok || card.Due == "" {
			continue
		}
		if card.Due <= today {
			continue
		}
		if next == "" || card.Due < next {
			next = card.Due
		}
	}
	return next, next != ""
}

func interval(box int) int {
	switch box {
	case 1:
		return 1
	case 2:
		return 3
	case 3:
		return 7
	case 4:
		return 14
	case 5:
		return 30
	default:
		return 60
	}
}

func utcDate(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
