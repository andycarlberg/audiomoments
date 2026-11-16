# Server-Configured Timezone Locality

* Status: accepted
* Date: 2026-11-12

## Context and Problem Statement

AudioMoments surfaces situational music playlists matching the listener's immediate temporal context (Daypart × Day Type). However, Navidrome and Subsonic media servers typically run headless in Docker containers or remote servers, frequently configured to UTC or server-host local time, while listeners operate in their local physical time.

The Subsonic API specification is inherently session-unaware:
1. The protocol provides no client timezone, geographic locality, or device time headers in its requests.
2. Navidrome models playlists as globally shared, persisted database entities (`navidrome.db`). It has no native concept of per-client dynamic or ephemeral playlist views. If two clients connect from different timezones, a single database playlist entity cannot satisfy both contexts simultaneously without continuous database thrashing.

## Considered Options

* **Option 1: Server-Configured Timezone Sidecar (Chosen)** — Anchor daypart evaluation to an explicit server environment variable (`TIMEZONE`, e.g., `America/Chicago`), falling back to host system time. Accept that roaming outside the home timezone is an unsupported edge case.
* **Option 2: Virtualizing Reverse Proxy Gateway (LanCache / Strangler Fig Pattern)** — Deploy a lightweight, transparent reverse proxy in front of Navidrome (`audiomoments.example.com`). The proxy passes standard streaming calls to Navidrome, but intercepts `getPlaylists.view`, infers client location/timezone from the client IP (GeoIP) or request headers, and dynamically injects virtual `[Moment]` playlists on the fly without mutating Navidrome's database.
* **Option 3: Upstream OpenSubsonic RFC / Navidrome Plugin** — Propose a client-timezone extension to the OpenSubsonic specification or implement an internal Navidrome WebAssembly (`.ndp`) plugin to virtualize playlists.

## Decision Outcome

Chosen option: **Option 1: Server-Configured Timezone Sidecar**, because 95%+ of personal music listening occurs within the user's home routine. Anchoring to a server-configured `TIMEZONE` allows the walking skeleton (Story 1) to deliver immediate concierge value with minimal operational complexity and zero additional network infrastructure.

### Reversibility (Two-Way Door)

This is explicitly a **two-way door (Type 2 reversible decision)**:
* The domain core—daypart clock evaluation (`time.LoadLocation`), Station Bank schema definitions, Songza 3-archetype slotting, and Subsonic playlist payload generation—is completely decoupled from the delivery transport.
* If production usage demonstrates that playlist database churn or travel roaming creates unacceptable friction, the domain core can be reused directly inside **Option 2 (Virtualizing Reverse Proxy)** as an additive shim without discarding the MVP implementation. If executed, that pivot will be captured in a future superseding ADR.

Option 3 was rejected because Navidrome's Extism Wasm plugin architecture strictly limits hooks to metadata, scrobbling, and scheduled tasks, with no HTTP middleware or response-rewriting capabilities. Pursuing a community-wide OpenSubsonic RFC faces multi-year consensus and client adoption hurdles.

### Consequences

* **Good:** Minimal operational footprint; requires no DNS subdomains, reverse proxy routing, or external GeoIP lookups.
* **Good:** Completely non-invasive; works out-of-the-box with any standard Subsonic client (Symfonium, Subtracks, Feishin, DSub).
* **Good:** Reversible; domain logic is fully reusable if a gateway proxy is introduced later.
* **Bad:** When traveling outside the configured `TIMEZONE`, playlists reflect home time rather than local physical time unless overridden via server configuration.
* **Bad:** Playlists are published to Navidrome's shared database, meaning multi-user households share the same active daypart playlists.
