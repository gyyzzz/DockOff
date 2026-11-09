# Repository Guidelines

## Project Structure & Module Organization

DockOff is a Go 1.24+ command-line tool. The CLI and installer are in `cmd/dockoff/`; shared bundle, RPM, and host helpers are in the same package. Target profile loading and validation live in `internal/targets/`, with one JSON profile per OS and architecture under `internal/targets/profiles/`. Go tests sit beside the code they cover. User workflows are documented in `README.md` and `README_zh.md`; design details are in `docs/详细设计.md`.

## Build, Test, and Development Commands

- `go build -o dockoff ./cmd/dockoff` builds the CLI.
- `go test ./...` runs all package tests.
- `go vet ./...` checks for common Go mistakes.
- `gofmt -w cmd/dockoff internal/targets` formats Go source before review.
- `./dockoff targets` lists embedded target IDs.

`prepare` and actual installation require a host matching the selected target profile and its system tools; do not treat a successful local build or staging run as installation validation. See the README before running commands that use root or change RPM trust or Docker service state.

## Coding Style & Naming Conventions

Use standard Go formatting and idioms: tabs are applied by `gofmt`, exported names use `MixedCaps`, and unexported helpers use lower-case `mixedCaps`. Keep target IDs and profile filenames descriptive and consistent, such as `openeuler-22.03-lts-sp4-amd64`. Keep profiles declarative; validate schema and package pins in `internal/targets/profile.go` when adding fields or targets.

## Testing Guidelines

Use Go's built-in `testing` package. Name tests `Test...` and keep them next to the package under test. Cover profile matching and rejection cases, plus bundle or installer validation failures when changing those paths. Run `go test ./...` and `go vet ./...` before submitting. Target-specific RPM collection or installation changes also need validation on a matching host, as documented in the design guide.

## Commit & Pull Request Guidelines

History is small and includes a `docs:` prefixed commit as well as less descriptive messages. Prefer a concise imperative subject with a conventional type prefix, for example `fix: reject mismatched profile architecture`. Pull requests should explain behavior and rationale, list test commands and results, link related issues when applicable, and include documentation updates for user-facing workflow or profile changes. Screenshots are generally unnecessary for this CLI.

## Security & Configuration

Treat signing-key hashes, package versions, and repository URLs in target profiles as security-sensitive. Update profile tests and documentation when changing them. Never weaken signature or offline dependency checks to make a build pass; include the reason and verification evidence in the pull request.
