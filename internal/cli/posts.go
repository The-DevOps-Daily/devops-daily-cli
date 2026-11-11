package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
)

type postJSON struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Excerpt     string   `json:"excerpt,omitempty"`
	Date        string   `json:"date"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	ReadingTime string   `json:"readingTime,omitempty"`
	URL         string   `json:"url"`
}

func (a *App) postsCmd() *cobra.Command {
	var tag string
	var limit int
	var all, jsonOut bool
	cmd := &cobra.Command{
		Use:     "posts",
		Short:   "List recent posts",
		Example: "  devops-daily posts --tag k8s",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensure(cmd.Context()); err != nil {
				return err
			}
			posts, err := a.Store.Posts()
			if err != nil {
				return err
			}
			allPosts := posts
			posts = filterPosts(posts, tag)
			if len(posts) == 0 && tag != "" {
				fmt.Fprintf(a.Stdout, "No posts for tag %q. Try: %s\n", tag, strings.Join(commonTags(allPosts), ", "))
				return nil
			}
			if !all && len(posts) > limit {
				posts = posts[:limit]
			}
			if jsonOut {
				values := make([]postJSON, len(posts))
				for i, p := range posts {
					values[i] = postJSON{p.Slug, p.Title, p.Excerpt, p.Date, p.Category, p.Tags, p.ReadingTime, p.URL}
				}
				return json.NewEncoder(a.Stdout).Encode(values)
			}
			for _, post := range posts {
				a.printPost(post)
			}
			fmt.Fprintln(a.Stdout, a.muted("Read one: devops-daily read <slug>"))
			return nil
		}}
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum posts")
	cmd.Flags().BoolVar(&all, "all", false, "show all")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	return cmd
}

func (a *App) printPost(post catalog.PostMeta) {
	width := a.width()
	if len(post.Date)+2+len([]rune(post.Title))+2+len(post.Slug) <= width {
		fmt.Fprintf(a.Stdout, "%s  %s  %s\n", post.Date, post.Title, a.muted(post.Slug))
		return
	}
	indent := strings.Repeat(" ", len(post.Date)+2)
	fmt.Fprintf(a.Stdout, "%s  %s\n%s%s\n", post.Date, truncate(post.Title, width-len(indent)), indent, a.muted(post.Slug))
}
func filterPosts(posts []catalog.PostMeta, tag string) []catalog.PostMeta {
	if tag == "" {
		return posts
	}
	want := alias(slug(tag))
	var result []catalog.PostMeta
	for _, post := range posts {
		for _, value := range append(append([]string{}, post.Tags...), post.Category) {
			if alias(slug(value)) == want {
				result = append(result, post)
				break
			}
		}
	}
	return result
}
func commonTags(posts []catalog.PostMeta) []string {
	counts := map[string]int{}
	for _, post := range posts {
		for _, tag := range post.Tags {
			counts[tag]++
		}
	}
	tags := make([]string, 0, len(counts))
	for tag := range counts {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool {
		return counts[tags[i]] > counts[tags[j]] || counts[tags[i]] == counts[tags[j]] && tags[i] < tags[j]
	})
	if len(tags) > 8 {
		tags = tags[:8]
	}
	return tags
}
