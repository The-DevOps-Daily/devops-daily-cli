BIN      ?= bin/ssh-site
VERSION  ?= $(shell git describe --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
HOST     ?=

.PHONY: build test vet run snapshot deploy clean cli cli-all

# A static linux/amd64 binary for the server.
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/ssh-site

cli:
	go build -trimpath -ldflags '-s -w -X main.version=$(VERSION) -X main.commit=$(shell git rev-parse --short HEAD 2>/dev/null || true) -X main.date=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)' -o bin/devops-daily ./cmd/devops-daily

cli-all:
	@for os in linux darwin windows; do for arch in amd64 arm64; do ext=; test $$os = windows && ext=.exe; CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '-s -w -X main.version=$(VERSION) -X main.commit=$(shell git rev-parse --short HEAD 2>/dev/null || true) -X main.date=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)' -o bin/devops-daily-$$os-$$arch$$ext ./cmd/devops-daily; done; done

test:
	go test ./...

vet:
	go vet ./...

# Run locally on 127.0.0.1:2222, then: ssh -p 2222 localhost
run:
	go run ./cmd/ssh-site -listen 127.0.0.1:2222 -state-dir ./state -sponsor-file deploy/sponsor.conf.example

# Refresh the baked quiz and incident content, then commit the result.
snapshot:
	@test -n "$(QUIZZES)" -a -n "$(PROBLEMS)" || { echo "usage: make snapshot QUIZZES=<site>/content/quizzes PROBLEMS=<devops-problems.json>"; exit 1; }
	python3 scripts/snapshot.py --quizzes $(QUIZZES) --problems $(PROBLEMS) --out internal/content/snapshot.json

# make deploy HOST=root@<admin address>
deploy: build
	@test -n "$(HOST)" || { echo "usage: make deploy HOST=root@<admin address>"; exit 1; }
	./deploy/deploy.sh $(BIN) $(HOST)

clean:
	rm -rf bin state
