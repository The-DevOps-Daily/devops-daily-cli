# Data format v1

The CLI reads static JSON files that the DevOps Daily site writes at build
time. They live under `https://devops-daily.com/cli/v1/`. Nothing here is
personal data, and the CLI only ever downloads these files; it never sends
anything back.

All files are UTF-8 JSON. Every file has `"schema": 1` at the top level. A
change that old CLIs can read (a new optional field, a new file) keeps
`schema` at 1. A change that would break them moves to `/cli/v2/`.

Text fields are plain text unless noted. Dates are `YYYY-MM-DD`. URLs are
absolute `https://` links.

## index.json (the manifest)

```json
{
  "schema": 1,
  "generatedAt": "2026-11-11T10:00:00.000Z",
  "site": "https://devops-daily.com",
  "files": {
    "posts.json": { "sha256": "<hex>", "bytes": 210000 },
    "posts/0.json": { "sha256": "<hex>", "bytes": 480000 },
    "quizzes.json": { "sha256": "<hex>", "bytes": 800000 }
  },
  "counts": { "posts": 555, "quizzes": 46, "flashcards": 27, "checklists": 24, "incidents": 16, "games": 90, "sponsors": 3 }
}
```

`files` lists every other file in the set, with its SHA-256 and size. The CLI
downloads a file only when its hash differs from the cached copy, checks the
hash, and writes the manifest last. File names are relative to `/cli/v1/`
and match `^[a-z0-9-]+(/[a-z0-9-]+)?\.json$`.

## posts.json

Newest first.

```json
{
  "schema": 1,
  "posts": [
    {
      "slug": "kubernetes-pod-oomkilled",
      "title": "Why your pod was OOMKilled",
      "excerpt": "One or two sentences.",
      "date": "2026-11-10",
      "category": "Kubernetes",
      "tags": ["kubernetes", "memory"],
      "readingTime": "8 min read",
      "url": "https://devops-daily.com/posts/kubernetes-pod-oomkilled",
      "body": "posts/3.json"
    }
  ]
}
```

`body` names the file that holds the post's Markdown. `category`, `tags`,
`readingTime` and `excerpt` may be missing.

## posts/N.json

Post bodies, split into 16 files (`posts/0.json` to `posts/15.json`) so a new
or edited post changes one file, not all of them.

```json
{ "schema": 1, "posts": { "kubernetes-pod-oomkilled": "## Markdown body..." } }
```

The body is the post's Markdown without front matter. The site's custom
blocks (diagrams, charts, terminals, tabs, callouts) are already converted
to plain Markdown, and links and images are absolute URLs.

## quizzes.json

```json
{
  "schema": 1,
  "quizzes": [
    {
      "id": "ansible-quiz",
      "title": "Ansible Configuration Management Quiz",
      "description": "...",
      "category": "Configuration Management",
      "url": "https://devops-daily.com/quizzes/ansible-quiz",
      "questions": [
        {
          "id": "playbook-structure",
          "title": "Playbook YAML Structure",
          "situation": "...",
          "code": "...",
          "question": "Which of the following...?",
          "options": ["...", "...", "...", "..."],
          "correct": 2,
          "explanation": "...",
          "hint": "...",
          "difficulty": "beginner",
          "points": 12
        }
      ]
    }
  ]
}
```

Questions that need the web page to make sense (git branch or merge-conflict
drawings) are left out. `situation`, `code`, `hint`, `difficulty` and
`points` may be missing.

## flashcards.json

```json
{
  "schema": 1,
  "decks": [
    {
      "id": "ansible-basics",
      "title": "Ansible Basics",
      "description": "...",
      "category": "Automation",
      "difficulty": "beginner",
      "url": "https://devops-daily.com/flashcards/ansible-basics",
      "cards": [{ "id": "ansible-definition", "front": "What is Ansible?", "back": "...", "category": "Fundamentals" }]
    }
  ]
}
```

## checklists.json

```json
{
  "schema": 1,
  "checklists": [
    {
      "id": "api-rate-limiting",
      "title": "Designing Rate Limiting for APIs",
      "description": "...",
      "category": "API Design",
      "difficulty": "intermediate",
      "estimatedTime": "2-3 hours",
      "url": "https://devops-daily.com/checklists/api-rate-limiting",
      "items": [
        {
          "id": "choose-algorithm-for-traffic-shape",
          "title": "Pick the algorithm based on your traffic shape",
          "description": "...",
          "critical": true,
          "code": [{ "language": "python", "label": "Token bucket", "code": "..." }]
        }
      ]
    }
  ]
}
```

## incidents.json

Real-world problems from the "30 Days of Real-World DevOps Problems" series,
only the days whose answer is already published.

```json
{
  "schema": 1,
  "incidents": [
    {
      "id": "day-01",
      "title": "...",
      "topic": "...",
      "difficulty": "intermediate",
      "technologies": ["Kubernetes"],
      "scenario": "...",
      "options": [{ "key": "A", "text": "..." }],
      "correct": "B",
      "fix": "...",
      "lesson": "...",
      "linkTitle": "Kubernetes Pod Simulator",
      "linkUrl": "https://devops-daily.com/games/..."
    }
  ]
}
```

## games.json

Simulators and games that are live on the site, plus Opsville.

```json
{
  "schema": 1,
  "games": [
    { "id": "dns-simulator", "type": "simulator", "title": "DNS Simulator", "description": "...", "category": "Networking", "url": "https://devops-daily.com/games/dns-simulator" }
  ]
}
```

`type` is `simulator` or `game`.

## sponsors.json

Paid sponsors only. The site's own products are never listed here.

```json
{
  "schema": 1,
  "sponsors": [{ "name": "Example", "text": "What they do, in a few words", "url": "https://go.devops-daily.com/example?p=cli" }]
}
```

The CLI shows one sponsor line, rotating through the list one step per UTC
day. An empty list hides the line.
