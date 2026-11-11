package render

import (
	"net/url"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gmtext "github.com/yuin/goldmark/text"
)

type Link struct {
	Text string
	URL  string
}

func Links(markdown string, base string) []Link {
	source := []byte(markdown)
	document := goldmark.New().Parser().Parse(gmtext.NewReader(source))
	seen := make(map[string]bool)
	var links []Link
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var raw, label string
		switch value := node.(type) {
		case *ast.Link:
			raw = string(value.Destination)
			label = strings.TrimSpace(string(value.Text(source)))
		case *ast.AutoLink:
			raw = string(value.URL(source))
			label = string(value.Label(source))
		default:
			return ast.WalkContinue, nil
		}
		u, err := url.Parse(raw)
		if err != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") || seen[raw] {
			return ast.WalkContinue, nil
		}
		seen[raw] = true
		if label == "" {
			label = raw
		}
		links = append(links, Link{Text: label, URL: raw})
		return ast.WalkContinue, nil
	})
	return links
}
