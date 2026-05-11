# Sonuntius — Configuration

As of v0.1.0 all six phases of the addon are functional. Five S6
services are supervised — three active by default (`ma-bridge`,
`yt-cast`, `cast-receiver`) and two opt-in (`tidal-connect`,
`alsa-to-sendspin`).

## Options

### `log_level`

The log level for the addon's services. One of:
`trace`, `debug`, `info`, `notice`, `warning`, `error`, `fatal`.
Default: `info`.

### `ma_player_id`

The Music Assistant player ID of the Sendspin speaker that should receive
playback. This is the player's stable identifier as shown in MA (not the
HA `media_player.*` entity ID; the bridge converts as needed). Required
once Phase 1 lands.

### `friendly_name_youtube`

The advertised friendly name for the YouTube / YouTube Music DIAL
receiver. Appears in the Android cast picker.
Default: `Sonuntius (YouTube)`.

### `friendly_name_tidal`

The advertised friendly name for the Cast (CASTV2) endpoint used by the
Tidal proxy. Appears in the Android cast picker.
Default: `Sonuntius (Tidal)`.

### `enable_youtube`

Whether to advertise the YouTube / YouTube Music DIAL receiver.
Default: `true`.

### `enable_tidal_proxy`

Whether to advertise the CASTV2 Cast endpoint used by the Tidal proxy.
Requires the user-supplied AirReceiver cert/key (see below).
Default: `true`.

### `yt_cast_dial_port`

TCP port the `yt-cast` service binds for its DIAL HTTP listener.
Default: `8008` (the Chromecast reference DIAL port). The upstream
`yt-cast-receiver` library defaults to 3000, but on a Home Assistant
host that also runs **Music Assistant** (which binds 3000 for its
frontend with `host_network: true`) that port is already taken — so
the addon picks 8008. Change this if 8008 conflicts with anything else
on your host. DIAL discovery does not require a specific port; the
SSDP advertisement carries the actual port via the `LOCATION` header.

### `cast_receiver_tls_port`

TCP port the `cast-receiver` service binds for its CASTV2 TLS listener.
Default: `8009` — the Chromecast standard. Change this only if 8009 is
already in use on your host. Cast senders discover the port via the
mDNS `_googlecast._tcp` SRV record we publish, so any port works.

### `volume_step`

Quantisation increment for volume changes received from the cast
sender. Every event from the phone is rounded to the nearest
multiple of this step and forwarded to MA. There is no dedup —
press-and-hold streams pass through, and repeated identical values
are idempotent on MA's end. Default: `5`. Set `10` for coarser
quantisation, or `1` to disable rounding entirely.

### `cast_cert_path` / `cast_key_path`

Filesystem paths inside the container for the AirReceiver
certificate/key. The default points at `/share/sonuntius/` so the user
can drop the files on the host without rebuilding the image.
Defaults:
- `/share/sonuntius/airreceiver_cert.pem`
- `/share/sonuntius/airreceiver_key.pem`

### `ha_base_url` / `ha_token` (optional)

Override the Home Assistant REST endpoint and authentication identity.
All four override fields are empty by default; the addon then uses the
Supervisor proxy at `http://supervisor/core/api` with the auto-injected
`SUPERVISOR_TOKEN`. Set these only when you want to talk to HA as a
named user (long-lived token) or to a different HA instance.

| Field | Default | Purpose |
| --- | --- | --- |
| `ha_base_url` | `""` (uses Supervisor proxy) | Full URL prefix, e.g. `http://homeassistant.local:8123` |
| `ha_token` | `""` (uses `$SUPERVISOR_TOKEN`) | Long-lived access token. Hidden in the addon UI (`password?` schema). |

When `ha_base_url` is set, the HA core WebSocket URL is derived
automatically (`http://...` → `ws://.../core/websocket`,
`https://...` → `wss://.../core/websocket`).

### `ma_ws_url` / `ma_token` (optional)

Override the Music Assistant direct WebSocket endpoint. When
`ma_ws_url` is empty, the bridge auto-discovers MA's hostname via the
Supervisor `/addons` listing and uses `ws://<host>:8095/ws`. If MA is
unreachable at the resolved URL the bridge falls back to the HA core
WebSocket subscription path.

| Field | Default | Purpose |
| --- | --- | --- |
| `ma_ws_url` | `""` (auto-discover) | Full WebSocket URL to MA's `/ws` endpoint |
| `ma_token` | `""` (no auth — addon-local trust) | Auth token used when MA's schema version is ≥ 28. Hidden in the addon UI. |
| `ma_queue_id` | `""` (auto-discover via `players/all`) | MA's internal `player_id` — the value MA uses as `queue_id` for `player_queues/play_media`. Set this only when auto-discovery doesn't find your speaker. The startup log lists every visible MA player at info — copy the `player_id` of the row whose `display_name` matches your speaker. |

> **`ma_token` is required for rich metadata in the MA UI.** On MA
> schema ≥ 28 the addon must authenticate before issuing
> `player_queues/play_media` — which is the only call path that
> carries title / artist / thumbnail through to the MA UI for URL
> playbacks (YouTube watch URLs). Without a token the bridge still
> plays audio via the HA REST fallback, but the MA UI shows the raw
> stream URL instead of the title.
>
> To create one: open Music Assistant → **Settings** → **Security** →
> **API Tokens**, mint a token, and paste it into the `ma_token`
> field of the Sonuntius addon options. Restart the addon. Look for
> `ma: authenticated` in the log.

### `tidal_fallback.*`

Opt-in iFi Tidal Connect binary fallback. Disabled by default. See the
**Tidal Connect fallback** section below for provenance and setup.

| Key | Purpose |
| --- | --- |
| `enabled` | Master switch (`false` by default). |
| `binary_tarball_path` | Path to a user-supplied tarball that contains the iFi `tidal_connect_application` binary and its bundled certificate. |
| `cert_filename` | Filename of the cert inside the tarball. |
| `friendly_name` | Friendly name shown in Tidal's device picker. |
| `sendspin_server_url` | URL of the Sendspin server (the MA-side endpoint that ingests the loopback audio). |

## Architecture

```
Phone (YT / YTM / Tidal)
        │  Cast / DIAL
        ▼
Sonuntius (this addon)        ← extracts intent, never relays audio
        │  REST / WS
        ▼
Music Assistant
        │  Sendspin
        ▼
Sendspin speaker
```

In **proxy mode** (Phases 1–4) the addon extracts the track / video ID
from the sender's protocol and asks Music Assistant to play it via its
own YouTube Music or Tidal provider. Audio never traverses the addon.

In **Tidal Connect fallback** (Phase 5, opt-in) the addon runs the
iFi `tidal_connect_application` binary, which receives actual decoded
PCM via the kernel's ALSA loopback, and a small GStreamer pipeline
pushes that audio onto the Sendspin server.

## Cert provisioning (Tidal proxy mode)

The CASTV2 receiver in proxy mode needs an AirReceiver certificate so
Cast senders accept its device-auth response. We **do not** redistribute
that cert in this repository or in the addon image.

To enable the Tidal proxy:

1. Obtain the AirReceiver cert and key (publicly circulated for years on
   GitHub gists and Reddit threads under search terms such as
   "AirReceiver cert dump" and "shanocast"). Provenance and legal
   posture: see the [shanocast write-up](https://xakcop.com/post/shanocast/).
2. Place them on the HA host at:
   - `/share/sonuntius/airreceiver_cert.pem`
   - `/share/sonuntius/airreceiver_key.pem`
3. Restart the addon.

If the cert is missing the addon logs a warning and disables the Cast
endpoint. The YouTube path (DIAL) is unaffected.

## Tidal Connect fallback (opt-in)

The `tidal_connect_application` binary was extracted from iFi Audio's
Zen Stream firmware. It is widely used in the DIY audio community
(HiFiBerryOS, Volumio, moOde, dietpi-allo). Tidal and iFi have not taken
action against this use in over five years, but the binary is **not
officially licensed for redistribution**. We do not ship it; you obtain
it yourself, package it as a tarball, and drop it at the path you set in
`tidal_fallback.binary_tarball_path`.

Upstream community sources:

- [`shawaj/ifi-tidal-release`](https://github.com/shawaj/ifi-tidal-release)
- [`TonyTromp/tidal-connect-docker`](https://github.com/TonyTromp/tidal-connect-docker)

Tidal could revoke the embedded certificate at any time without notice.

### Architecture caveat

The binary is ARMv7. On `aarch64` (HA Yellow, RPi 4/5) it runs via the
kernel's compat layer plus `libc6-compat`. On `amd64` it requires
`qemu-user-static` and incurs a substantial CPU cost; we recommend
`amd64` users stick with the proxy.

### ALSA loopback prerequisite

The fallback needs the kernel's `snd-aloop` module loaded on the host.

```sh
# HA OS loads snd-aloop by default. Verify:
ha hardware audio
# On a custom Linux host:
echo snd-aloop > /etc/modules-load.d/snd-aloop.conf
```

## Health endpoint

The addon exposes a loopback HTTP endpoint at `http://127.0.0.1:8099/health`
(hosted inside the container by `ma-bridge`) that returns the
aggregated status of every component as JSON:

```json
{
  "status": "ok",
  "started_at": "2026-05-11T05:00:00Z",
  "uptime_seconds": 12.3,
  "components": [
    {"name": "config",     "healthy": true, "detail": "log_level=info, ha_token=supervisor", ...},
    {"name": "dispatcher", "healthy": true, "detail": "entity=media_player.sendspin", ...},
    {"name": "ipc",        "healthy": true, "detail": "listening on /run/sonuntius/events.sock", ...},
    {"name": "state",      "healthy": true, "detail": "direct MA WebSocket: ws://music-assistant:8095/ws", ...}
  ]
}
```

`status` flips to `degraded` when any component is unhealthy (for
example, when `ma_player_id` is unset and the dispatcher would drop
events). The HTTP response code stays `200` so the HA watchdog only
restarts the addon on hard failures — the body is the source of truth
for finer-grained health distinctions.

## Troubleshooting

- **Phone doesn't see Sonuntius.** Verify `host_network: true` (it
  should be — that's the default). On the addon host, run
  `avahi-browse -art` and confirm `_googlecast._tcp.local.` includes the
  Sonuntius friendly name. If not, check the host's mDNS firewall rules.
- **Tidal proxy plays the wrong / no track.** Set `log_level: debug` and
  capture the raw `LOAD` payload from the addon logs. Tidal's
  `customData` schema is reverse-engineered and may change without
  notice; refining the parser is straightforward once a sample is in
  hand.
- **Tidal proxy fails entirely.** Switch to the fallback by enabling
  `tidal_fallback.enabled: true` and providing the iFi tarball.
- **Audio drops in fallback mode.** Adjust the ALSA buffer in the
  GStreamer pipeline (TBD once Phase 5 lands).

## References

External references:

- Music Assistant Sendspin: https://www.music-assistant.io/player-support/sendspin/
- yt-cast-receiver (upstream of the Phase 2 port): https://github.com/patrickkfkan/yt-cast-receiver
- shanocast (AirReceiver auth replay write-up): https://xakcop.com/post/shanocast/
- Chromium openscreen Cast spec: https://chromium.googlesource.com/openscreen/+/refs/heads/main/cast/
- `ifi-tidal-release` (Phase 5 binary source): https://github.com/shawaj/ifi-tidal-release
- `tidal-connect-docker` (Phase 5 binary source): https://github.com/TonyTromp/tidal-connect-docker
- Home Assistant addon docs: https://developers.home-assistant.io/docs/add-ons
- hassio-addons base image: https://github.com/hassio-addons/addon-base
