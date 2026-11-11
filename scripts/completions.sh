#!/bin/sh
# Writes shell completions for the release archives (goreleaser before hook).
set -eu
mkdir -p completions
go run ./cmd/devops-daily completion bash > completions/devops-daily.bash
go run ./cmd/devops-daily completion zsh > completions/_devops-daily
go run ./cmd/devops-daily completion fish > completions/devops-daily.fish
