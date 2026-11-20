# AGENTS.md

Project context and guardrails for AI coding and review agents working on AudioMoments.

## Architecture & System Boundaries

- **Standalone Go Sidecar ([ADR-0002](docs/decisions/0002-standalone-go-sidecar.md))**: Lightweight Go binary bridging content generation (**AudioMuse-AI**) and streaming delivery (**OpenSubsonic** servers: Navidrome, Gonic, Airsonic). Standard library first; zero external runtime dependencies unless explicitly required by an issue.
- **Zero Vector Math**: AudioMoments performs no acoustic analysis, CLAP embedding math, or custom audio scoring. It is strictly a temporal and situational REST orchestrator (`DayType × Daypart` evaluation, 3-archetype slot selection: `upbeat`, `focus`, `mellow`, and playlist synchronization).
- **Strict REST Contracts**: Communicate with OpenSubsonic servers and AudioMuse-AI exclusively over standard HTTP/JSON REST APIs. Never couple to AudioMuse internal Python modules or Navidrome-proprietary plugin hooks.
- **Timezone Locality ([ADR-0001](docs/decisions/0001-server-configured-timezone-locality.md))**: Dayparts evaluate against the server-configured `TIMEZONE` environment variable (`time.LoadLocation`, falling back to `time.Local`). Keep the domain core (`internal/clock`, `internal/station`) decoupled from transport so it can be reused inside an in-line `httputil.ReverseProxy` gateway if the ADR-0001 two-way door is exercised.

## Package Layout & Data Boundaries

- `internal/clock`: Pure temporal domain logic (`Daypart`, `DayType`, `Moment`, `Evaluate`).
- `internal/config`: Environment configuration loader and validator (`SUBSONIC_URL`, `SUBSONIC_USER`, `SUBSONIC_PASSWORD`, `STATION_BANK_PATH`, `TIMEZONE`).
- `internal/station`: Station bank schema, validation, strict JSON decoding (`DisallowUnknownFields` + trailing-token check), `SeedStations()` prompt templates, and atomic persistence (`SaveFile`, `LoadFile`, `LoadBank`).
- `docs/decisions/`: Architecture Decision Records (ADRs).
- **Fixtures vs. Runtime State**:
  - Subsonic `track_ids` are installation-specific database IDs from a user's local library. Never commit static or synthetic station JSON files to `stations/` or the repo root.
  - `./stations` (default `STATION_BANK_PATH`, relative to the process working directory; git-ignored at the repository root via `/stations/`) is runtime state populated via AudioMuse prompt fulfillment (`station.SaveFile`).
  - Synthetic station JSON fixtures for tests belong strictly in package `testdata/` directories (e.g., `internal/station/testdata/`).

## TDD & Verification Commands

AudioMoments is a **Test-Driven Development (TDD)** project:
- **Red-Green-Refactor**: Write or update the failing table-driven test (`*_test.go`) in an external black-box test package (`package <pkg>_test`, using `t.TempDir()` / `t.Setenv()` for isolation) first, verify it fails for the expected reason, write the minimal stdlib Go implementation to pass, and refactor cleanly.
- **Deterministic Checks over Agentic Rulesets**: Whenever an invariant, boundary, or convention can be enforced deterministically via automated tooling (compiler types, tests, linters, commit/PR checks, or CI gates), enforce it mechanically in tooling rather than relying solely on prose rules in `AGENTS.md`.

All changes must pass the checks defined in `Makefile`, `.github/workflows/ci.yml`, `.github/workflows/pr-title.yml`, `.golangci.yml`, `.yamllint.yml`, and `.coderabbit.yaml`:

- **Bootstrap Local Hooks & Pinned Tools**: `make setup-hooks` installs pinned `bin/gitleaks` (`v8.30.1`) and `bin/commitlint` (`v0.12.0`) and configures `core.hooksPath = .githooks`.
  - `.githooks/pre-commit`: Runs staged `gitleaks` secret scanning and `gofmt` on staged `.go` files.
  - `.githooks/commit-msg`: Enforces Conventional Commits via `bin/commitlint`.
  - `.githooks/pre-push`: Runs `make test` and `make lint` before pushing.
- **Full Local Suite**: `make verify` runs `test`, `lint`, `vulncheck`, and `secrets`:
  - **Tests (`make test`)**: `go test -v -race -cover ./...`
  - **Lint & Format (`make lint`)**: Installs and runs pinned `bin/golangci-lint` (`v2.15.0`, matching CI, v2 config: `gofmt`, `standard` linters, `bodyclose`, `copyloopvar`, `errorlint`, `gocritic`, `gosec` with `G304` excluded for station file paths, `misspell`, `noctx`, `revive`, `unconvert`).
    - Key invariants: package and exported symbol doc comments (`revive`), checked type assertions (`errcheck`), context-aware HTTP requests (`noctx`), closed HTTP response bodies (`bodyclose`), wrapped error checks (`errorlint`).
  - **Vulnerability Scan (`make vulncheck`)**: Installs and runs pinned `bin/govulncheck` (`v1.10.0`) on `./...`.
  - **Secret Scan (`make secrets`)**: Installs and runs pinned `bin/gitleaks` (`v8.30.1`) across git history and staged changes.
- **CI-Enforced Polyglot & PR Gates**:
  - `Config Lint` (`.github/workflows/ci.yml`): Runs `actionlint` (`v1.7.12`), `yamllint --strict .` (against `.yamllint.yml`), and `shellcheck .githooks/*`. Run `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` (and `yamllint` / `shellcheck` if installed) when touching `.github/`, YAML files, or `.githooks/`.
  - `PR Title` (`.github/workflows/pr-title.yml`): Enforces Conventional Commits on PR titles (and single-commit PRs via `validateSingleCommit: true`).

## PR Scope Discipline & Definition of Done

Work is organized around vertical user-value **Stories** (parent issues) decomposed into horizontal labor **Tasks** (sub-issues). Branch names follow `<type>/<issue-id>-<slug>` (e.g., `feat/3-subsonic-client`, `ci/15-static-analysis-pipeline`), and commits/PR titles use Conventional Commits (`feat:`, `fix:`, `ci:`, `chore:`, `docs:`, `refactor:`, `test:`).

A task is complete only when all of the following gates are satisfied:

1. **1-to-1 Sub-Issue Scope & Zero Speculative Abstraction**: The diff implements only the target sub-issue. Do not bundle downstream capabilities (e.g., active stream `getNowPlaying` draining, `last_surfaced` LRU rotation, Phase 2 CLAP cluster synthesis), unpulled struct fields, single-implementation interfaces, or speculative config aliases. If an architectural gap or missing tooling/infrastructure prerequisite surfaces mid-task, stop and surface it as an emergent sub-issue under the active Story rather than patching around it in-flight.
2. **TDD, Verified Wire Contracts & Behavioral Coverage**: Tests are written test-first and exercise observable behavior, boundary values, error branches, and idempotency on repeat execution—not vanity line-coverage numbers. External REST test fixtures (OpenSubsonic, AudioMuse-AI) must reflect real protocol semantics (e.g., OpenSubsonic returning HTTP `200 OK` with `"status": "failed"` and an embedded `"error"` payload).
3. **Executed Verification (No Assumed Passes)**: `make verify` (or each underlying target: `make test`, `make lint`, `make vulncheck`, `make secrets`) and — when modifying `.github/`, YAML files, or `.githooks/` — `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` (plus `yamllint --strict .` / `shellcheck .githooks/*` if locally installed) have been executed and pass with zero findings. Any check that cannot be executed locally must be explicitly flagged at handoff, never silently skipped or assumed passing.
4. **Adversarial Diff & Test Audit**: The staged diff (`git diff --staged`) has been self-reviewed for correctness bugs, resource and transport hygiene (`context.Context` propagation, `http.Client` timeouts, `resp.Body` draining/closing, atomic temp-file cleanup), scope leakage, and test effectiveness before requesting commit or merge.
5. **Documentation, `AGENTS.md` Sync, Atomic ADRs & Linear Merge**: `README.md` is updated whenever user-facing configuration (env vars, defaults), CLI commands, or operational behavior changes. `AGENTS.md` is updated whenever packages, system boundaries, configuration, or verification commands change. Any new or revised architectural decision is committed under `docs/decisions/` in the same PR as the code it governs. The PR rebases cleanly onto `main` with all CI jobs green and CodeRabbit review findings resolved.
