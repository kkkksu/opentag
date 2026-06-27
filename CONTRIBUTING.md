# Contributing to OpenTag

Thanks for your interest in improving OpenTag. This is an early-stage project and
contributions are welcome.

## Getting started

```sh
git clone https://github.com/kkkksu/opentag
cd opentag
go build ./...
go test ./...
```

You'll need Go 1.26+. To run it end to end you also need a reachable
[kagent](https://github.com/kagent-dev/kagent) server and a Slack app in Socket
Mode (see the README).

## Before opening a PR

- `gofmt -l .` reports no files (run `gofmt -w .` to fix).
- `go vet ./...` is clean.
- `go test ./...` passes.
- New behavior has tests.

CI runs the same checks on every PR.

## Project layout

- `cmd/opentag` — entrypoint
- `internal/chat` — chat platform abstraction (`slack` is the only provider today)
- `internal/backend` — agent-runtime abstraction (`kagent` implementation)
- `internal/{routing,identity,governance,async,routines,audit,config,app}` — the coworker core

Keep the platform (`chat`) and runtime (`backend`) behind their interfaces so new
providers/backends can be added without touching the core.

## Commit messages

Short, imperative, lower-case subjects (e.g. `add discord provider`). Keep
unrelated changes in separate commits.

## Reporting issues

Use the issue templates. For bugs, include what you expected, what happened, and
the relevant config (with secrets redacted) and logs.
