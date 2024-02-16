#!/bin/bash

go install golang.org/x/vuln/cmd/govulncheck@latest
go install golang.org/x/tools/cmd/deadcode@latest

gofmt -s -w .

revive ./...

gocyclo -over 15 .

go mod tidy

govulncheck ./...

deadcode ./cmd/*

go env -w CGO_ENABLED=1

go test -race ./...

#go test -bench=BenchmarkController ./cmd/gateboard

go env -w CGO_ENABLED=0

go install ./...

go env -u CGO_ENABLED
