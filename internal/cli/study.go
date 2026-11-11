package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/paths"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/progress"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/sponsor"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/text"
)

func (a *App) progressStore() (*progress.Store, error) {
	store, err := progress.Open(filepath.Join(paths.StateDir(), "progress.json"))
	if err != nil {
		return nil, err
	}
	if warning := store.Warning(); warning != "" {
		fmt.Fprintln(a.Stderr, a.muted(text.Clean(warning, false)))
	}
	return store, nil
}

func (a *App) studySponsor() (sponsor.Sponsor, error) {
	values, err := a.Store.Sponsors()
	if err != nil {
		return sponsor.Sponsor{}, err
	}
	value, ok := catalog.SponsorOn(values, a.Now())
	if !ok {
		return sponsor.Sponsor{}, nil
	}
	return sponsor.Sponsor{Name: value.Name, Text: value.Text, URL: value.URL}, nil
}

func studyMatches[T any](values []T, details func(T) studyMatch, want, suffix, kind string) (T, error) {
	matches := make([]studyMatch, len(values))
	for i, value := range values {
		matches[i] = details(value)
	}
	index, err := findStudy(matches, want, suffix, kind)
	if err != nil {
		var zero T
		return zero, err
	}
	return values[index], nil
}

func plainChecklist(out *strings.Builder, checklist catalog.Checklist, done map[string]bool) {
	count := 0
	for _, item := range checklist.Items {
		if done[item.ID] {
			count++
		}
	}
	fmt.Fprintf(out, "%s\n%d of %d done\n", checklist.Title, count, len(checklist.Items))
	for _, item := range checklist.Items {
		mark := "[ ]"
		if done[item.ID] {
			mark = "[x]"
		}
		critical := ""
		if item.Critical {
			critical = " (critical)"
		}
		fmt.Fprintf(out, "%s %s%s\n", mark, item.Title, critical)
	}
	if checklist.URL != "" {
		fmt.Fprintln(out, checklist.URL)
	}
}
