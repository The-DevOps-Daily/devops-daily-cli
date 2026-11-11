package catalog

import (
	"strings"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/text"
)

func cleanURL(value string) string {
	if strings.HasPrefix(value, "https://") {
		return text.Line(value, 500)
	}
	return ""
}
func cleanPosts(values []PostMeta) {
	for i := range values {
		p := &values[i]
		p.Slug = text.Line(p.Slug, 160)
		p.Title = text.Line(p.Title, 300)
		p.Excerpt = text.Clean(p.Excerpt, true)
		p.Date = text.Line(p.Date, 20)
		p.Category = text.Line(p.Category, 100)
		p.ReadingTime = text.Line(p.ReadingTime, 60)
		p.URL = cleanURL(p.URL)
		p.Body = text.Line(p.Body, 100)
		for j := range p.Tags {
			p.Tags[j] = text.Line(p.Tags[j], 100)
		}
	}
}
func cleanQuizzes(values []Quiz) {
	for i := range values {
		q := &values[i]
		q.ID = text.Line(q.ID, 160)
		q.Title = text.Line(q.Title, 300)
		q.Description = text.Clean(q.Description, true)
		q.Category = text.Line(q.Category, 100)
		q.URL = cleanURL(q.URL)
		for j := range q.Questions {
			x := &q.Questions[j]
			x.ID = text.Line(x.ID, 160)
			x.Title = text.Line(x.Title, 300)
			x.Situation = text.Clean(x.Situation, true)
			x.Code = text.Clean(x.Code, true)
			x.Question = text.Clean(x.Question, true)
			x.Explanation = text.Clean(x.Explanation, true)
			x.Hint = text.Clean(x.Hint, true)
			x.Difficulty = text.Line(x.Difficulty, 60)
			for k := range x.Options {
				x.Options[k] = text.Clean(x.Options[k], true)
			}
		}
		// A question whose answer is not one of its options cannot be played.
		kept := q.Questions[:0]
		for _, x := range q.Questions {
			if x.Correct >= 0 && x.Correct < len(x.Options) {
				kept = append(kept, x)
			}
		}
		q.Questions = kept
	}
}
func cleanDecks(values []Deck) {
	for i := range values {
		x := &values[i]
		x.ID = text.Line(x.ID, 160)
		x.Title = text.Line(x.Title, 300)
		x.Description = text.Clean(x.Description, true)
		x.Category = text.Line(x.Category, 100)
		x.Difficulty = text.Line(x.Difficulty, 60)
		x.URL = cleanURL(x.URL)
		for j := range x.Cards {
			c := &x.Cards[j]
			c.ID = text.Line(c.ID, 160)
			c.Front = text.Clean(c.Front, true)
			c.Back = text.Clean(c.Back, true)
			c.Category = text.Line(c.Category, 100)
		}
	}
}
func cleanChecks(values []Checklist) {
	for i := range values {
		x := &values[i]
		x.ID = text.Line(x.ID, 160)
		x.Title = text.Line(x.Title, 300)
		x.Description = text.Clean(x.Description, true)
		x.Category = text.Line(x.Category, 100)
		x.Difficulty = text.Line(x.Difficulty, 60)
		x.EstimatedTime = text.Line(x.EstimatedTime, 60)
		x.URL = cleanURL(x.URL)
		for j := range x.Items {
			item := &x.Items[j]
			item.ID = text.Line(item.ID, 160)
			item.Title = text.Line(item.Title, 300)
			item.Description = text.Clean(item.Description, true)
			for k := range item.Code {
				item.Code[k].Language = text.Line(item.Code[k].Language, 60)
				item.Code[k].Label = text.Line(item.Code[k].Label, 100)
				item.Code[k].Code = text.Clean(item.Code[k].Code, true)
			}
		}
	}
}
func cleanIncidents(values []Incident) {
	for i := range values {
		x := &values[i]
		x.ID = text.Line(x.ID, 160)
		x.Title = text.Line(x.Title, 300)
		x.Topic = text.Line(x.Topic, 100)
		x.Difficulty = text.Line(x.Difficulty, 60)
		x.Correct = text.Line(x.Correct, 10)
		x.Scenario = text.Clean(x.Scenario, true)
		x.Fix = text.Clean(x.Fix, true)
		x.Lesson = text.Clean(x.Lesson, true)
		x.LinkTitle = text.Line(x.LinkTitle, 300)
		x.LinkURL = cleanURL(x.LinkURL)
		for j := range x.Technologies {
			x.Technologies[j] = text.Line(x.Technologies[j], 100)
		}
		for j := range x.Options {
			x.Options[j].Key = text.Line(x.Options[j].Key, 10)
			x.Options[j].Text = text.Clean(x.Options[j].Text, true)
		}
	}
}
func cleanGames(values []Game) {
	for i := range values {
		x := &values[i]
		x.ID = text.Line(x.ID, 160)
		x.Type = text.Line(x.Type, 30)
		x.Title = text.Line(x.Title, 300)
		x.Description = text.Clean(x.Description, true)
		x.Category = text.Line(x.Category, 100)
		x.URL = cleanURL(x.URL)
	}
}
func cleanSponsors(values []Sponsor) {
	for i := range values {
		values[i].Name = text.Line(values[i].Name, 60)
		values[i].Text = text.Line(values[i].Text, 120)
		values[i].URL = cleanURL(values[i].URL)
	}
}
