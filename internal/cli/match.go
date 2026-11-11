package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
)

func findPost(posts []catalog.PostMeta, want string) (catalog.PostMeta, error) {
	want = strings.ToLower(want)
	for _, post := range posts {
		if post.Slug == want {
			return post, nil
		}
	}
	if found := matchingPosts(posts, want, true); len(found) == 1 {
		return found[0], nil
	} else if len(found) > 1 {
		return catalog.PostMeta{}, severalPosts(want, found)
	}
	if found := matchingPosts(posts, want, false); len(found) == 1 {
		return found[0], nil
	} else if len(found) > 1 {
		return catalog.PostMeta{}, severalPosts(want, found)
	}
	return catalog.PostMeta{}, fmt.Errorf("No post called %s.\nDid you mean:\n%s", want, strings.Join(postSuggestions(posts, want), "\n"))
}
func matchingPosts(posts []catalog.PostMeta, want string, prefix bool) []catalog.PostMeta {
	var result []catalog.PostMeta
	for _, post := range posts {
		slugMatch, titleMatch := strings.Contains(post.Slug, want), strings.Contains(strings.ToLower(post.Title), want)
		if prefix {
			slugMatch = strings.HasPrefix(post.Slug, want)
			titleMatch = false
		}
		if slugMatch || titleMatch {
			result = append(result, post)
		}
	}
	return result
}
func severalPosts(want string, posts []catalog.PostMeta) error {
	names := make([]string, 0, len(posts))
	for _, post := range posts {
		names = append(names, post.Slug)
	}
	return fmt.Errorf("Several posts match %s:\n%s", want, listMatches(names))
}
func postSuggestions(posts []catalog.PostMeta, want string) []string {
	names := make([]string, len(posts))
	titles := make([]string, len(posts))
	for i, post := range posts {
		names[i], titles[i] = post.Slug, post.Title
	}
	return closest(want, names, titles, 5)
}

// closest ranks names by how many words of want they or their titles contain,
// then by edit distance to want, and returns at most n of them.
func closest(want string, names, titles []string, n int) []string {
	words := strings.FieldsFunc(strings.ToLower(want), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	type candidate struct {
		name     string
		hits     int
		distance int
	}
	candidates := make([]candidate, len(names))
	for i, name := range names {
		title := ""
		if i < len(titles) {
			title = strings.ToLower(titles[i])
		}
		hits := 0
		distance := levenshtein(name, want)
		parts := strings.FieldsFunc(name, func(r rune) bool { return r == '-' })
		for _, word := range words {
			if len(word) < 2 {
				continue
			}
			if strings.Contains(name, word) || strings.Contains(title, word) {
				hits++
				continue
			}
			// A typo in a longer word still counts ("kubernets").
			for _, part := range parts {
				if d := levenshtein(part, word); len(word) >= 5 && d <= 2 {
					hits++
					distance = min(distance, d)
					break
				}
			}
		}
		candidates[i] = candidate{name, hits, distance}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].hits != candidates[j].hits {
			return candidates[i].hits > candidates[j].hits
		}
		return candidates[i].distance < candidates[j].distance
	})
	out := make([]string, 0, n)
	for _, c := range candidates {
		if len(out) == n || (c.hits == 0 && len(out) > 0 && candidates[0].hits > 0) {
			break
		}
		out = append(out, c.name)
	}
	return out
}

// listMatches joins names one per line, at most ten, for a "several match"
// error.
func listMatches(names []string) string {
	if len(names) <= 10 {
		return strings.Join(names, "\n")
	}
	return strings.Join(names[:10], "\n") + fmt.Sprintf("\n...and %d more. Try a longer name.", len(names)-10)
}

func levenshtein(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, x := range a {
		current := make([]int, len(b)+1)
		current[0] = i + 1
		for j, y := range b {
			cost := 0
			if x != y {
				cost = 1
			}
			current[j+1] = minInt(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}
func minInt(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}
