# Stdlib-Only OpenSubsonic REST Client

* Status: accepted
* Date: 2026-11-20

## Context and Problem Statement

AudioMoments needs an OpenSubsonic/Subsonic REST client to verify connectivity and manage playlists (`ping`, `getPlaylists`, `createPlaylist`, `updatePlaylist`) using Subsonic v1.13+ MD5 salt+token authentication. Before building `internal/subsonic`, we audited the Go Subsonic ecosystem to decide whether to adopt an existing third-party client library or implement a focused standard-library client.

## Considered Options

* **Option 1: `github.com/supersonic-app/go-subsonic`** — The only actively maintained Go Subsonic client library (Supersonic's fork of `delucks/go-subsonic`). It supports Subsonic and OpenSubsonic JSON payloads, including `formPost`.
* **Option 2: `github.com/delucks/go-subsonic` or `github.com/mdlayher/gosubsonic`** — Earlier standalone Go Subsonic libraries. `delucks/go-subsonic` targets Go 1.13 (`io/ioutil`), supports XML only, hardcodes API version `1.8.0`, and has had no pushes since August 2024; `mdlayher/gosubsonic` has been archived since 2017. Note also that the `opensubsonic` GitHub organization publishes only the API specification (`opensubsonic.github.io`), `navidrome/navidrome` and `sentriz/gonic` are server implementations without client packages, and active Go Subsonic clients (`Explo`, `SubTUI`, `stmp`, `go-navi-smart-playlist`, `rocksonic`, `must`, `nd`) hand-roll their HTTP calls.
* **Option 3: Thin Stdlib `internal/subsonic` Client (Chosen)** — Implement a lightweight standard-library client (`net/http`, `encoding/json`, `crypto/rand`, `crypto/md5`) scoped strictly to the four required endpoints and MD5 salt+token authentication.

## Decision Outcome

Chosen option: **Option 3: Thin Stdlib `internal/subsonic` Client**, for the following reasons:

1. **Context Propagation & Transport Hygiene**: Neither `supersonic-app/go-subsonic` nor `delucks/go-subsonic` accepts `context.Context` (`http.NewRequest` throughout), violating our `noctx` static-analysis gate and preventing clean cancellation/timeouts. They also skip `resp.StatusCode` checks and read bodies unbounded with `io.ReadAll`.
2. **Dependency & Release Hygiene**: `supersonic-app/go-subsonic` publishes no tagged semver releases (requiring a pseudo-version pin) and pulls in `golang.org/x/image` solely for cover-art helpers AudioMoments does not use. Keeping `internal/subsonic` stdlib-only preserves zero external runtime dependencies and minimal `govulncheck` surface per [ADR-0002](0002-standalone-go-sidecar.md).
3. **Auth, Nil Safety & Error Semantics**: `go-subsonic` generates salts with `math/rand.Intn` and caches a static `(salt, token)` inside `Authenticate` (forcing an eager `ping`), logs to stderr via `log.Println` in `Ping()`, dereferences `resp.Playlists.Playlist` without a nil check in `GetPlaylists`, ignores `"status": "failed"` when `"error"` is omitted, and returns untyped error strings. Our stdlib client generates a fresh `crypto/rand` salt per request, scrubs auth query parameters from `*url.Error`, guards missing JSON containers, and exposes a typed `*APIError` for `errors.As`.
4. **Cohesive Playlist Methods**: `go-subsonic` splits playlist operations awkwardly (`CreatePlaylist(map[string]string)` cannot send repeated `songId` values; `CreatePlaylistWithTracks` discards the returned `*Playlist` and performs a hidden `getOpenSubsonicExtensions` call; `UpdatePlaylist` and `UpdatePlaylistTracks` separate metadata and track updates).

**Transport Choice (`GET /rest/<endpoint>.view`)**: Methods issue `GET /rest/<endpoint>.view` with the `.view` suffix retained for compatibility across Navidrome, Gonic, and Airsonic without introducing an extra `getOpenSubsonicExtensions` roundtrip. OpenSubsonic `formPost` support can be added incrementally later if playlist sizes ever approach URL length limits.

### Reversibility (Two-Way Door)

* All HTTP and OpenSubsonic JSON wire details remain encapsulated inside `internal/subsonic`. Adding `getNowPlaying`, `deletePlaylist`, or `formPost` (or swapping the transport implementation) requires no changes to callers outside the package.

### Consequences

* **Good:** Zero external runtime dependencies and minimal `govulncheck` surface, consistent with [ADR-0002](0002-standalone-go-sidecar.md).
* **Good:** Native `context.Context` cancellation, `crypto/rand` per-request salts, bounded response reads/draining, and typed `*APIError` errors complying with `noctx`, `bodyclose`, `errorlint`, and `gosec`.
* **Bad:** AudioMoments maintains its own Subsonic struct tags and endpoint parameter bindings as additional Subsonic endpoints are added in future stories.
