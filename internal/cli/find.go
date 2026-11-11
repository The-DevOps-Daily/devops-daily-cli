package cli

import (
	"fmt"
	"strings"
)

type studyMatch struct {
	ID       string
	Title    string
	Category string
}

func findStudy(values []studyMatch, want, suffix, kind string) (int, error) {
	want = strings.ToLower(want)
	for i, value := range values {
		id := strings.ToLower(value.ID)
		if id == want || id == want+suffix || strings.TrimSuffix(id, suffix) == want {
			return i, nil
		}
	}
	for mode := 0; mode < 3; mode++ {
		var found []int
		for i, value := range values {
			match := false
			switch mode {
			case 0:
				match = strings.HasPrefix(strings.ToLower(value.ID), want)
			case 1:
				match = strings.Contains(strings.ToLower(value.ID), want) || strings.Contains(strings.ToLower(value.Title), want)
			default:
				match = strings.EqualFold(value.Category, want) || strings.HasPrefix(strings.ToLower(value.Category), want)
			}
			if match {
				found = append(found, i)
			}
		}
		if len(found) == 1 {
			return found[0], nil
		}
		if len(found) > 1 {
			ids := make([]string, len(found))
			for i, index := range found {
				ids[i] = values[index].ID
			}
			return 0, fmt.Errorf("Several matches for %s:\n%s", want, listMatches(ids))
		}
	}
	names := make([]string, len(values))
	titles := make([]string, len(values))
	for i, value := range values {
		names[i], titles[i] = strings.ToLower(value.ID), value.Title
	}
	close := closest(want, names, titles, 5)
	message := fmt.Sprintf("No %s called %s.", kind, want)
	if len(close) > 0 {
		message += "\nDid you mean:\n" + strings.Join(close, "\n")
	}
	return 0, fmt.Errorf("%s", message)
}
