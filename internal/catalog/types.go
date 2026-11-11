// Package catalog reads the site's CLI data set (docs/data-format.md): the
// JSON files the DevOps Daily site publishes under /cli/v1/, cached on disk
// so the CLI works offline after the first sync.
package catalog

import "github.com/The-DevOps-Daily/devops-daily-cli/internal/content"

// Schema is the data format version this build reads.
const Schema = 1

// FileInfo is one entry in the manifest.
type FileInfo struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Manifest is index.json.
type Manifest struct {
	Schema      int                 `json:"schema"`
	GeneratedAt string              `json:"generatedAt"`
	Site        string              `json:"site"`
	Files       map[string]FileInfo `json:"files"`
	Counts      map[string]int      `json:"counts"`
}

// PostMeta is one entry in posts.json.
type PostMeta struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Excerpt     string   `json:"excerpt,omitempty"`
	Date        string   `json:"date"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	ReadingTime string   `json:"readingTime,omitempty"`
	URL         string   `json:"url"`
	Body        string   `json:"body"`
}

// PostsIndex is posts.json.
type PostsIndex struct {
	Schema int        `json:"schema"`
	Posts  []PostMeta `json:"posts"`
}

// PostBodies is one posts/N.json file: slug to Markdown.
type PostBodies struct {
	Schema int               `json:"schema"`
	Posts  map[string]string `json:"posts"`
}

// Question is one multiple-choice quiz question.
type Question struct {
	ID          string   `json:"id"`
	Title       string   `json:"title,omitempty"`
	Situation   string   `json:"situation,omitempty"`
	Code        string   `json:"code,omitempty"`
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	Correct     int      `json:"correct"`
	Explanation string   `json:"explanation,omitempty"`
	Hint        string   `json:"hint,omitempty"`
	Difficulty  string   `json:"difficulty,omitempty"`
	Points      int      `json:"points,omitempty"`
}

// Quiz is one quiz.
type Quiz struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Category    string     `json:"category,omitempty"`
	URL         string     `json:"url"`
	Questions   []Question `json:"questions"`
}

// Quizzes is quizzes.json.
type Quizzes struct {
	Schema  int    `json:"schema"`
	Quizzes []Quiz `json:"quizzes"`
}

// Card is one flashcard.
type Card struct {
	ID       string `json:"id"`
	Front    string `json:"front"`
	Back     string `json:"back"`
	Category string `json:"category,omitempty"`
}

// Deck is one flashcard deck.
type Deck struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	Difficulty  string `json:"difficulty,omitempty"`
	URL         string `json:"url"`
	Cards       []Card `json:"cards"`
}

// Decks is flashcards.json.
type Decks struct {
	Schema int    `json:"schema"`
	Decks  []Deck `json:"decks"`
}

// CodeBlock is a code sample attached to a checklist item.
type CodeBlock struct {
	Language string `json:"language,omitempty"`
	Label    string `json:"label,omitempty"`
	Code     string `json:"code"`
}

// ChecklistItem is one box to tick.
type ChecklistItem struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	Critical    bool        `json:"critical,omitempty"`
	Code        []CodeBlock `json:"code,omitempty"`
}

// Checklist is one checklist.
type Checklist struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	Description   string          `json:"description,omitempty"`
	Category      string          `json:"category,omitempty"`
	Difficulty    string          `json:"difficulty,omitempty"`
	EstimatedTime string          `json:"estimatedTime,omitempty"`
	URL           string          `json:"url"`
	Items         []ChecklistItem `json:"items"`
}

// Checklists is checklists.json.
type Checklists struct {
	Schema     int         `json:"schema"`
	Checklists []Checklist `json:"checklists"`
}

// Incident is one real-world problem with its fix. It is the same shape the
// SSH app bakes into its snapshot.
type Incident = content.Incident

// Incidents is incidents.json.
type Incidents struct {
	Schema    int        `json:"schema"`
	Incidents []Incident `json:"incidents"`
}

// Game is one simulator or game on the site.
type Game struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	URL         string `json:"url"`
}

// Games is games.json.
type Games struct {
	Schema int    `json:"schema"`
	Games  []Game `json:"games"`
}

// Sponsor is one paid sponsor for the "Supported by" line.
type Sponsor struct {
	Name string `json:"name"`
	Text string `json:"text,omitempty"`
	URL  string `json:"url"`
}

// Sponsors is sponsors.json.
type Sponsors struct {
	Schema   int       `json:"schema"`
	Sponsors []Sponsor `json:"sponsors"`
}
