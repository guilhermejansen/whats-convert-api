# Repository Guidelines

## Project Structure & Module Organization
`cmd/api/main.go` boots the Fiber server; keep it thin and push logic into `internal/`. Key packages: `internal/config` (env parsing), `internal/server` (routing/middleware), `internal/handlers` (HTTP adapters), `internal/services` (conversion flows), and `internal/pool` (worker/buffer orchestration). Shared helpers that must be exported live in `pkg/utils`; load tooling sits in `scripts/`, container assets in `docker/`, and the prebuilt `media-converter` CLI plus the `web/` static UI need updates whenever API shapes change.

## Build, Test, and Development Commands
Use the Makefile for repeatable workflows: `make deps` to sync modules, `make build` or `make run` for local binaries, and `make dev` when working with Air hot reloads. Docker contributors lean on `make docker-build`, `make docker-run`, and `make docker-shell`, followed by `make docker-stop` or `make docker-clean` to tear down. Monitoring stacks come up with `make monitoring-up` and should be shut down via `make monitoring-down` before handing the environment off.

## Coding Style & Naming Conventions
Go 1.25 formatting is enforced with `go fmt ./...`; run `golangci-lint run` (via `make lint`) before every PR. Stick to idiomatic Go naming: packages lower-case, exported types/functions PascalCase, local identifiers camelCase, shared constants UPPER_SNAKE. Structured logging and contextual error wrapping (`fmt.Errorf("detail: %w", err)`) keep observability intact.

## Testing Guidelines
`make test` executes unit and integration coverage with `-race`; add `_test.go` files beside the code they exercise. Track deltas with `make test-coverage` and flag coverage below 80% in the PR body. Exercise the pipeline under load with `make benchmark`, `make load-test`, or `make stress-test`, and document deviations for reviewers.

## Commit & Pull Request Guidelines
History follows Conventional Commits (`feat:`, `fix:`, `chore:`); subjects stay under 70 characters and use imperative voice. Bundle related changes only, reference tickets with `Refs #123`, and note user-visible impacts in the body. Pull requests need a summary, test command output, config or schema updates, and screenshots or curl examples for new endpoints; request reviewers for any `internal/` or `web/` modules touched.

## Environment & Configuration
Start from `.env.example`, keeping production overrides in `.env.production`; call out changes to `MAX_WORKERS`, `BUFFER_POOL_SIZE`, `BUFFER_SIZE`, `GOGC`, or `GOMEMLIMIT`. Install FFmpeg and libvips locally via `make install-deps-mac` or `make install-deps-ubuntu` before testing conversions. Never commit secrets—use env files ignored by git or Docker secrets when sharing credentials.
