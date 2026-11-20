# AudioMoments

AudioMoments is an autonomous situational music concierge for self-hosted music servers (Navidrome, Gonic, Airsonic, and the wider OpenSubsonic ecosystem).

Inspired by Songza, AudioMoments evaluates your current temporal moment (Daypart × Day Type) and automatically synchronizes three distinct situational playlist choices (**Upbeat**, **Focus**, and **Mellow**) into your Subsonic server, eliminating browsing friction and decision fatigue when you open your music app.

---

## Configuration

AudioMoments is configured via environment variables:

| Variable | Description | Default | Required |
| :--- | :--- | :--- | :--- |
| `SUBSONIC_URL` | Base URL of your Subsonic server (e.g. `http://localhost:4533`). | — | **Yes** |
| `SUBSONIC_USER` | Subsonic username. | — | **Yes** |
| `SUBSONIC_PASSWORD` | Subsonic user password. | — | **Yes** |
| `STATION_BANK_PATH` | Path to directory containing station JSON definitions. | `./stations` | No |
| `TIMEZONE` | IANA timezone name (e.g. `America/Chicago`, `Europe/London`). | System Local | No |

---

## Timezone & Travel Notice

AudioMoments evaluates active dayparts against the server-configured `TIMEZONE` variable (falling back to host system time if omitted). 

Because the Subsonic protocol is session-unaware and does not transmit client locality or timezone headers, all connected clients share playlists evaluated in this configured timezone.

> [!NOTE]
> **Traveling Outside Your Home Timezone**:
> If you travel outside your configured `TIMEZONE`, your playlists will reflect your home routine rather than local physical time unless you adjust the server's `TIMEZONE` setting. This intentional design choice (codified in [ADR-0001](docs/decisions/0001-server-configured-timezone-locality.md)) delivers immediate concierge listening with zero network proxying or database thrashing. If multi-client roaming is needed in the future, a virtualizing reverse proxy gateway is documented as our reversible two-way escape door.

---

## Development & Testing

Built with standard Go (`1.27.4`). Requires zero external runtime dependencies.

Bootstrap hooks and run the full verification suite via `Makefile` (`make verify` auto-installs pinned `golangci-lint v2.15.0`, `govulncheck v1.10.0`, and `gitleaks v8.30.1` into `bin/`):

```bash
# Install pinned bin/gitleaks (v8.30.1) and bin/commitlint (v0.12.0) and activate .githooks/
make setup-hooks

# Run all local checks (test, lint, vulncheck, secrets)
make verify
```

- **Local Git Hooks (`.githooks/`)**:
  - `pre-commit`: Scans staged changes for secrets (`gitleaks`) and checks staged `.go` files with `gofmt`.
  - `commit-msg`: Enforces [Conventional Commits](https://www.conventionalcommits.org/) via `commitlint`.
  - `pre-push`: Runs `make test` (`go test -v -race -cover ./...`) and `make lint` (`golangci-lint run`) before pushing non-delete refs.
- **CI Gates (`.github/workflows/`)**:
  - `ci.yml`: Runs `Lint` (`golangci-lint v2.15.0`), `Vulncheck` (`make vulncheck`), `Test` (`make test`), `Secrets` (`make secrets`), and `Config Lint` (`actionlint`, `yamllint`, `shellcheck`).
  - `pr-title.yml`: Enforces Conventional Commits on pull request titles (`PR Title`), including single-commit squash defaults.

---

## Architecture & Decisions

- [ADR-0001: Server-Configured Timezone Locality](docs/decisions/0001-server-configured-timezone-locality.md)
- [ADR-0002: Standalone Subsonic Sidecar in Go](docs/decisions/0002-standalone-go-sidecar.md)
