package cli

import (
	"context"
	"strings"
	"time"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/content"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/feed"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/render"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/sponsor"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/text"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/tui"
)

func (a *App) home(ctx context.Context) error {
	if err := a.ensure(ctx); err != nil {
		return err
	}
	deps, err := a.homeDeps()
	if err != nil {
		return err
	}
	return a.RunTUI(tui.New(a.renderer(), deps))
}

func (a *App) homeDeps() (tui.Deps, error) {
	style := a.colorStyle()
	posts, err := a.Store.Posts()
	if err != nil {
		return tui.Deps{}, err
	}
	quizzes, err := a.Store.Quizzes()
	if err != nil {
		return tui.Deps{}, err
	}
	incidents, err := a.Store.Incidents()
	if err != nil {
		return tui.Deps{}, err
	}
	sponsors, err := a.Store.Sponsors()
	if err != nil {
		return tui.Deps{}, err
	}
	questions := make([]content.Question, 0)
	for _, quiz := range quizzes {
		for _, question := range quiz.Questions {
			questions = append(questions, content.Question{QuizID: quiz.ID, QuizTitle: quiz.Title, ID: question.ID, Title: question.Title, Situation: question.Situation, Code: question.Code, Question: question.Question, Options: question.Options, Correct: question.Correct, Explanation: question.Explanation, Difficulty: question.Difficulty})
		}
	}
	snapshot, err := content.New(questions, incidents)
	if err != nil {
		return tui.Deps{}, err
	}
	feedPosts := make([]feed.Post, 0, min(30, len(posts)))
	byURL := make(map[string]catalog.PostMeta)
	for i, post := range posts {
		if i == 30 {
			break
		}
		date, _ := time.Parse("2006-01-02", post.Date)
		feedPosts = append(feedPosts, feed.Post{Title: post.Title, URL: post.URL, Summary: post.Excerpt, Date: date})
		byURL[post.URL] = post
	}
	selected, _ := catalog.SponsorOn(sponsors, a.Now())
	return tui.Deps{Posts: func() ([]feed.Post, time.Time) { return feedPosts, a.Now() }, Content: snapshot, Sponsor: func() sponsor.Sponsor {
		return sponsor.Sponsor{Name: text.Line(selected.Name, 60), Text: text.Line(selected.Text, 120), URL: selected.URL}
	}, Now: a.Now, Header: "devops-daily.com", OpenURL: a.OpenURL, Article: func(post feed.Post, width int) ([]string, error) {
		meta := byURL[post.URL]
		body, err := a.Store.PostBody(meta)
		if err != nil {
			return nil, err
		}
		value, err := render.Markdown(body, width, style)
		if err != nil {
			return nil, err
		}
		return strings.Split(strings.TrimSpace(value), "\n"), nil
	}}, nil
}
