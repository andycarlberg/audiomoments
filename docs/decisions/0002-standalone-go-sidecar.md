# Standalone Subsonic Sidecar in Go

* Status: accepted
* Date: 2026-11-16

## Context and Problem Statement

AudioMoments was initially sketched as a lightweight Python script (`dispatcher.py`) running in an Unraid user script environment, assuming tight adjacency to AudioMuse-AI (also written in Python) and targeting Navidrome.

During story refinement and mobbing for Task 1, we examined whether this runtime and deployment coupling was an unexamined exploratory placeholder ("anchoring hazard") or a genuine requirement. We evaluated where the concierge capability belongs in the personal audio ecosystem: within the acoustic clustering engine (AudioMuse), inside the streaming server (Navidrome core or plugin), inside mobile/desktop clients (Symfonium/Feishin), or as a standalone sidecar service (per [ADR-0001](0001-server-configured-timezone-locality.md) Option 1).

## Considered Options

* **Option 1: Python Sidecar (Initial Placeholder)** — Implement the dispatcher in Python using `uv`, `pytest`, and `httpx`.
* **Option 2: Navidrome Extism WebAssembly Plugin (`.ndp`)** — Implement the dispatcher directly as an in-process Navidrome plugin using the Extism PDK and compile to WebAssembly.
* **Option 3: Rust Sidecar** — Implement a standalone daemon in Rust using Axum/Reqwest.
* **Option 4: Standalone Go Sidecar (Chosen)** — Implement AudioMoments in Go as a standalone sidecar service that interfaces with station providers (seed bank, AudioMuse) and media servers (Navidrome, Gonic, Subsonic) strictly over standard HTTP/REST boundaries.

## Decision Outcome

Chosen option: **Option 4: Standalone Go Sidecar**, for the following reasons:

1. **Protocol Universality over Single-Server Lock-in**: Navidrome's `.ndp` plugin system is proprietary to Navidrome. By building a standalone sidecar speaking OpenSubsonic REST, AudioMoments serves the entire self-hosted ecosystem (Navidrome, Gonic, Airsonic, LMS/Lyrion, Jellyfin) out of the box.
2. **Operational Footprint on Homelabs**: AudioMoments does zero acoustic analysis or embedding math; it is pure scheduling, catalog matching, and HTTP communication. A statically linked Go binary produces a ~15MB executable running in a `scratch` container with negligible memory footprint (~15–20MB) and instant startup, outperforming Python environments on low-spec home servers (Raspberry Pi, TrueNAS, Unraid).
3. **Natural Fit for Future Reverse Proxying (ADR-0001 Two-Way Door)**: [ADR-0001](0001-server-configured-timezone-locality.md) identified a virtualizing reverse-proxy gateway (Option 2) as the two-way escape door if dynamic client timezone roaming is needed later. Go's standard library `net/http/httputil.ReverseProxy` is built specifically for low-memory, zero-copy HTTP streaming and range requests, making a future transition from out-of-band sidecar to in-line proxy seamless.
4. **Clean API Boundary**: Playlist generation (AudioMuse) is decoupled from situational presentation (Subsonic). Meeting both sides strictly at API boundaries prevents accidental coupling to AudioMuse's internal Python libraries.

### Reversibility (Two-Way Door)

* If a native Navidrome plugin is ever prioritized, Go code is already native to Navidrome's codebase and compiles directly to Wasm via TinyGo and Extism Go PDK without rewriting business logic.
* The domain core (Daypart × Day Type clock math, 3-slot archetype mapping, and station selection) remains strictly isolated from transport, allowing easy porting or embedding into any future runtime.

### Consequences

* **Good:** Universal compatibility across all Subsonic-compliant servers (Navidrome, Gonic, Airsonic).
* **Good:** Zero-dependency static binary distribution with tiny memory and CPU footprint.
* **Good:** Native foundation for both the current out-of-band sidecar dispatcher (ADR-0001 Option 1) and a future in-line reverse proxy (ADR-0001 Option 2).
* **Bad:** Cannot import Python helper code directly from AudioMuse; must integrate with AudioMuse via standard REST/JSON contracts.
