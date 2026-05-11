# Sonuntius

Cast/DIAL → Music Assistant bridge for Sendspin playback.

> **Latin coinage:** *sonus* "sound" + *nuntius* "messenger" — the one
> who carries music between protocols.

## What it does

Sonuntius makes a Music Assistant–controlled Sendspin player visible to
the Android YouTube, YouTube Music, and Tidal apps as a Cast / DIAL
target. The addon does **not** decode or relay audio; it extracts the
intent (track ID, transport command) from the sender's protocol and hands
that off to Music Assistant's REST/WebSocket API, which streams the
actual audio to the Sendspin speaker through its native provider
integrations.

- **YouTube / YouTube Music** — DIAL + lounge protocol via
  [`patrickkfkan/yt-cast-receiver`](https://github.com/patrickkfkan/yt-cast-receiver).
  Works out of the box; no auth replay required.
- **Tidal (proxy mode)** — CASTV2 receiver with the AirReceiver
  device-auth replay (see [shanocast](https://xakcop.com/post/shanocast/)).
  Track IDs are extracted from the Cast `LOAD` message's `customData`.
  Audio path: Tidal Android → Sonuntius (metadata only) → MA Tidal
  provider → Sendspin player. Requires a user-supplied cert under
  `/share/sonuntius/`.
- **Tidal Connect (fallback, opt-in)** — runs the user-supplied iFi Zen
  Stream `tidal_connect_application` binary inside the container and
  forwards its ALSA-loopback output to the Sendspin server.

## Status

**v0.1.0 — all six phases of the plan land in this release.** S6
supervises five services, three are active by default, two are opt-in:

- `ma-bridge` — HA REST + WS bridge, IPC broker, MA-direct WebSocket
  with HA core WS fallback, loopback health endpoint at
  `127.0.0.1:8099/health`.
- `yt-cast` — Full Go 1.26 port of
  [`yt-cast-receiver`](https://github.com/patrickkfkan/yt-cast-receiver)
  v2.1.1 under `internal/ytcast/`. DIAL discovery, YouTube Lounge
  protocol, sonuntius Player adapter that emits `PlayIntent` events into
  the IPC broker.
- `cast-receiver` — Cast (CASTV2) receiver under `internal/castv2/`.
  AirReceiver auth replay, Tidal `customData` parser, Default Media
  Receiver fallback for generic audio URLs. Stays alive and announces
  via mDNS even when the AirReceiver cert is absent.
- `tidal-connect` (opt-in) — iFi Tidal Connect binary, gated on
  `tidal_fallback.enabled = true` + user-supplied tarball.
- `alsa-to-sendspin` (opt-in) — ALSA loopback → Sendspin WebSocket
  forwarder for the Tidal Connect fallback path.

## Installation

1. Add this repository to Home Assistant (Settings → Add-ons → Add-on
   Store → ⋮ → Repositories → `https://github.com/shobuprime/home-assistant-apps`).
2. Install the **Sonuntius** addon.
3. Configure `ma_player_id` with the Music Assistant player ID of the
   Sendspin speaker.
4. (Phase 3+) Drop the AirReceiver cert at
   `/share/sonuntius/airreceiver_cert.pem` / `airreceiver_key.pem` to
   enable the Tidal proxy.
5. (Phase 5+) Optionally enable the Tidal Connect binary fallback —
   see `DOCS.md`.

## Requirements

- Home Assistant OS / Supervised, with Music Assistant installed and at
  least one Sendspin player configured.
- `host_network: true` is required so the addon can announce itself on
  mDNS (`_googlecast._tcp`) and SSDP (DIAL).
- Architecture: `aarch64` or `amd64` (the hassio-addons base image
  dropped armhf/armv7/i386 in v19.0.0).

## Configuration

See [`DOCS.md`](DOCS.md) for the full option reference, cert
provisioning, and Tidal Connect fallback setup.

## License

MIT
