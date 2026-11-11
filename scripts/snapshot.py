#!/usr/bin/env python3
"""Build internal/content/snapshot.json from DevOps Daily content.

The Go binary embeds the snapshot, so run this, commit the result, then build.

  python3 scripts/snapshot.py \
      --quizzes <site checkout>/content/quizzes \
      --problems <devops-problems.json> \
      --out internal/content/snapshot.json

Quizzes are the site's content/quizzes/*.json files. Problems are the
"30 Days of Real-World DevOps Problems" series export; only days whose
status is "answered" are included, so no answer appears here before it was
published.
"""

import argparse
import datetime
import glob
import json
import os
import re
import sys

CONTROL = re.compile(r"[\x00-\x08\x0b-\x1f\x7f-\x9f‎‏‪-\u202e⁦-⁩]")


def clean(s):
    s = (s or "").replace("\r\n", "\n").replace("\t", "    ")
    s = CONTROL.sub("", s)
    # House style: no em or en dashes in anything we show.
    s = s.replace(" \u2014 ", ", ").replace("\u2014", "-").replace("\u2013", "-")
    return s.strip()


def load_questions(folder):
    questions = []
    files = sorted(glob.glob(os.path.join(folder, "*.json")))
    if not files:
        sys.exit(f"no quiz files in {folder}")
    for path in files:
        with open(path, encoding="utf-8") as f:
            quiz = json.load(f)
        quiz_id = quiz["id"]
        if not re.fullmatch(r"[a-z0-9-]+", quiz_id):
            sys.exit(f"{path}: unexpected quiz id {quiz_id!r}")
        for q in quiz.get("questions", []):
            # Questions that draw git branch diagrams need the web page.
            if "branches" in q:
                continue
            options = [clean(o) for o in q["options"]]
            correct = q["correctAnswer"]
            if not 0 <= correct < len(options):
                sys.exit(f"{path}: {q['id']} has correctAnswer {correct} out of range")
            questions.append({
                "quizId": quiz_id,
                "quizTitle": clean(quiz["title"]),
                "id": str(q["id"]),
                "title": clean(q.get("title")),
                "situation": clean(q.get("situation")),
                "code": clean(q.get("codeExample")),
                "question": clean(q.get("description")),
                "options": options,
                "correct": correct,
                "explanation": clean(q.get("explanation")),
                "difficulty": clean(q.get("difficulty")),
            })
    return questions


def fix_text(answer):
    """The answer post minus its first line (the letter, shown separately),
    the "Go deeper" link (shown separately) and the series footer."""
    text = clean(answer)
    text = re.sub(r"^The answer is [A-D]\.\s*", "", text)
    text = re.split(r"\n\s*Go deeper:", text)[0]
    text = text.replace("30 Days of Real-World DevOps Problems by DevOps Daily", "")
    return text.strip()


def load_incidents(path):
    with open(path, encoding="utf-8") as f:
        data = json.load(f)
    incidents = []
    for day in data["days"]:
        if day.get("status") != "answered":
            continue
        url = day["resource"]["url"]
        if not url.startswith("https://devops-daily.com/"):
            sys.exit(f"day {day['day']}: resource is not a devops-daily.com page: {url}")
        options = [{"key": k, "text": clean(v)} for k, v in sorted(day["options"].items())]
        if day["correct"] not in day["options"]:
            sys.exit(f"day {day['day']}: correct option {day['correct']} missing")
        incidents.append({
            "id": day["id"],
            "title": clean(day["title"]),
            "topic": clean(day.get("topic")),
            "difficulty": clean(day.get("difficulty")),
            "technologies": [clean(t) for t in day.get("technologies", [])],
            "scenario": clean(day["scenario"]),
            "options": options,
            "correct": day["correct"],
            "fix": fix_text(day["answer"]),
            "lesson": clean(day.get("lesson")),
            "linkTitle": clean(day["resource"]["title"]),
            "linkUrl": url,
        })
    if not incidents:
        sys.exit("no answered problems found")
    return incidents


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--quizzes", required=True)
    ap.add_argument("--problems", required=True)
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    snapshot = {
        "generated": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d"),
        "questions": load_questions(args.quizzes),
        "incidents": load_incidents(args.problems),
    }
    with open(args.out, "w", encoding="utf-8") as f:
        json.dump(snapshot, f, ensure_ascii=False, indent=1)
        f.write("\n")
    print(f"wrote {args.out}: {len(snapshot['questions'])} questions, {len(snapshot['incidents'])} incidents")


if __name__ == "__main__":
    main()
