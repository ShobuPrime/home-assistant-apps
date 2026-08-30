# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant App for [Lemonade](https://lemonade-server.ai/), a
local LLM server that exposes OpenAI-, Ollama- and Anthropic-compatible APIs.
Its purpose in this repo is to give Home Assistant a private, on-device
conversation agent — Home Assistant's built-in **Ollama** integration talks to
it directly, no custom integration required.

The app uses Home Assistant's S6-overlay init system and follows standard HA
app conventions.

## Essential Commands

### Building and Testing
```bash
# Build the app locally (auto-detects architecture)
./build.sh

# Test the app locally
docker run --rm -it -p 13305:13305 -v lemon-data:/data \
    local/{arch}-addon-local_lemonade:{version}

# Full smoke test with a mock Supervisor (from repo root)
.github/scripts/smoke-test.sh lemonade local/amd64-addon-local_lemonade:11.5.0
```

### Version Management
```bash
# Check for updates (from repo root)
APP_PATH=lemonade CHECK_ONLY=true JSON_OUTPUT=true bash .github/scripts/update-lemonade.sh
```

## Lemonade Version Scheme

Single release stream on GitHub, tagged `vX.Y.Z`. Releases are frequent
(multiple per month). The app version tracks the upstream version exactly.

Upstream is a **C++** project as of v10: `lemond` (the server) and `lemonade`
(the CLI). Older documentation describing a Python/pip package is out of date.

## Architecture and Key Components

### Directory Structure
- **`/rootfs/etc/cont-init.d/`**: S6 init — creates `/data/lemonade` and merges
  app options into Lemonade's `config.json`
- **`/rootfs/etc/services.d/lemonade/`**: the `lemond` server
- **`/rootfs/etc/services.d/lemonade-provision/`**: one-shot that registers,
  downloads and preloads the configured model once the server is live
- **`/rootfs/usr/local/bin/lemonade`**: wrapper that sets `LD_LIBRARY_PATH`
  before running the real CLI

### Critical Files
- **`config.yaml`**: version, ingress on 13305, options schema
- **`build.yaml`**: base images per architecture
- **`Dockerfile`**: two stages — a Debian trixie `glibc-provider` and the Alpine runtime
- **`apparmor.txt`**: security profile

### Architecture Support
- `amd64` (x86_64)
- `aarch64` (arm64)

### Port Configuration
- **13305** (published): Web UI, OpenAI/Ollama/Anthropic APIs, and the realtime
  WebSocket. Everything is on one port, which is what makes ingress work. Owned
  by `ha-lemonade-bridge`, not by lemond.
- **13306** (loopback only): lemond itself, behind the bridge. Never published.
- **10600** (internal): Wyoming speech-to-text, only when `stt_enabled`.

## Development Guidelines

### THE glibc-on-musl ARRANGEMENT — read before touching the Dockerfile

Upstream ships **no musl build**. `lemond` is a glibc binary, and — more
importantly — the llama.cpp backends Lemonade downloads *at runtime* from
`ggml-org/llama.cpp` are glibc builds too. Building Lemonade from source
against musl would therefore not be enough; the runtime-fetched backend would
still need glibc.

So the image stays on the repo-standard Alpine base and bundles a glibc
closure:

1. Stage `glibc-provider` (`debian:trixie-slim`) downloads the Lemonade
   "embeddable" archive and copies the `ldd` closure of `lemond` + `lemonade`
   into `/glibc/lib`, plus the ELF loader.
2. The Alpine runtime stage copies that to `/opt/glibc/lib` and puts the loader
   at the exact path baked into the binaries (`/lib64/ld-linux-x86-64.so.2` on
   amd64, `/lib/ld-linux-aarch64.so.1` on aarch64).

Rules that follow from this:

- **Debian trixie, not bookworm.** `lemond` needs GLIBC_2.38 and
  GLIBCXX_3.4.32; bookworm has glibc 2.36 / libstdc++ 12 and will not run it.
- **`libgomp1` must be copied explicitly.** Every llama.cpp build links OpenMP,
  but those binaries do not exist at build time so `ldd` cannot discover the
  dependency. Without it, model loading fails with
  `libgomp.so.1: cannot open shared object file` (exit 127) while `lemond`
  itself looks perfectly healthy — a genuinely confusing failure mode.
- **Never export `LD_LIBRARY_PATH` globally.** Alpine's own curl/jq/bashio would
  resolve glibc's `libz`/`libssl` and break. It is set per-exec in the S6 run
  script and in the `lemonade` CLI wrapper. `llama-server` is spawned by
  `lemond` and inherits it, which is exactly what it needs.
- The closure is computed with `ldd`, not hand-listed, so a new upstream
  dependency cannot silently go missing.

### AppArmor: signals must be receivable

The profile uses `signal (send,receive),` — **not** the
`signal (send) set=(kill,term,int,hup,cont),` line most other apps in this repo
use. AppArmor mediates signal *delivery* as well as sending; with send-only
permission the confined `lemond` never receives s6's SIGTERM, skips its
graceful shutdown, and the container is SIGKILLed after the grace period
(exit 137).

Verified locally on an identical image: send-only → 12.7 s / exit 137 with no
cleanup logged; send+receive → 6.9 s / exit 0 with the full s6 shutdown
sequence.

### Model naming

Models registered by this app live in Lemonade's `user.*` namespace, and the
two APIs spell them differently:

- OpenAI API and Web UI: `user.LFM2.5-230M`
- Ollama API (what Home Assistant sees): `LFM2.5-230M:latest`

Both forms are accepted by `/api/chat`. Documentation should quote the Ollama
form when talking about the Home Assistant integration.

Models discovered from `extra_models_dir` behave differently: each file is
listed under its **own filename without the extension**, and a subdirectory is
listed under the directory name. Verified with four drop-ins at once:
`alpha-7b-q4.gguf` → `alpha-7b-q4`, `beta-tiny-instruct.gguf` →
`beta-tiny-instruct`, `gamma-vision-Q8_0.gguf` → `gamma-vision-Q8_0`,
`delta-sharded/model-00001-of-00001.gguf` → `delta-sharded` (all also
`<name>:latest` over the Ollama API). Document this with a multi-file example —
a single placeholder like `my-model.gguf` reads as though every discovered
model gets one fixed name.
Upstream's `src/cpp/Extra-Models-Dir-Spec.md` documents an `extra.` prefix on
the listed name; that is **not** what 11.5.0 does — verified with a real
drop-in. The `extra.`-prefixed form still resolves as an alias, so both work
for inference, but the model list and the HA Ollama dropdown show the bare
name. Re-verify against a running server before changing this claim.

### The memory proxy (`ha-lemonade-bridge/`)

A small Go reverse proxy giving Lemonade long-term memory in MuninnDB. Stdlib
only — no third-party modules, matching the repo's Go convention.

**Port layout is decided once**, in `cont-init.d/lemonade.sh`, written to
`/run/lemonade/runtime.env`, and unconditional:

```
Home Assistant ──► ha-lemonade-bridge :13305 ──► lemond 127.0.0.1:13306
```

**The bridge always runs**, even with memory and speech-to-text off. This
reverses the old "never run a pass-through proxy" rule: the bridge is also the
browser-Origin gate, and that job cannot be done at container start because the
ingress origin is Home Assistant's own URL, which only Core knows — and
Supervisor starts us before Core. Only something in the request path can hold a
list that corrects itself once Core answers. See the root CLAUDE.md section
"Home Assistant Core is NOT running when your app's init script runs".

Memory and STT remain independently gated *inside* the bridge. "Off" means no
memory lookups and no Wyoming listener, not a removed process. Stop time is
unchanged: 9 s, exit 0.

A bonus: the old "off" path had to *delete* `/etc/services.d/lemonade-bridge`
in cont-init, because a legacy service that exits during startup wedges S6
shutdown (measured: `docker stop` hung past 120 s, exit 137). A service that
always runs cannot hit that.

### The origin gate (`origins.go`)

Checked per request against two halves:

| half | source |
|------|--------|
| static | the user's `allowed_origins`, from cont-init via `BRIDGE_ALLOWED_ORIGINS` |
| derived | everything discoverable — host addresses (`/network/info`), published port (`/addons/self/info`), HA's URLs (`/core/api/config`) — refreshed by `watch()` |

`watch()` fetches each source independently and keeps the last good value, so
Core being down during boot does not also drop the host addresses. Those come
from Supervisor, which is necessarily up because it started us.

**Emit both `http://` and `https://` for every address.** Home Assistant is
commonly served over TLS and matching is exact, so an http-only list 403s
anyone reaching HA at `https://<ip>:8123` while the configured URL works. Safe:
an attacker's page carries its own origin, so trusting the device's own
addresses grants nothing to anyone not already serving from Home Assistant.

- **No `Origin` header passes.** HA's Ollama integration, the provisioning
  script and curl all send none; only browsers are being constrained.
- **Exact match** on scheme+host+port. `null` is a value, not an absence.
- **The 403 body is byte-identical to lemond's**, so existing clients behave
  the same.
- **Rejections log the origin.** Their absence is what made this slow to
  diagnose on a live device.

**lemond gets `LEMONADE_ALLOWED_ORIGINS=*`** and is bound to loopback. Safe
because the bridge is its only client and gates first — and the wildcard is
what keeps CORS correct: with `*`, lemond 11.5.1 echoes the request origin in
`Access-Control-Allow-Origin` rather than a literal `*`, so cross-origin
clients keep working with no CORS code here. The smoke test asserts wildcard
and loopback together; either alone is a hole.

**GETs are not exempt** — a comment claimed so for months. `GET /api/v1/health`
with a disallowed origin returns 403. The Web UI renders anyway because
navigations and asset loads send no `Origin`; only fetch/XHR/WebSocket do.
Hence "the page works, every action fails".

Hard requirements, all covered by tests in `ha-lemonade-bridge/proxy_test.go`:

- **Fail open, always.** Memory being missing, unreachable, slow or erroring
  must never change the response the client gets. A circuit breaker (3 strikes,
  60 s cooldown) keeps an absent MuninnDB from costing a dial timeout per turn.
- **Never replace the system prompt** — append to it. Home Assistant puts the
  exposed-entity list and tool definitions there; replacing it breaks Assist.
  `TestInjectSystemPreservesExistingPrompt` guards this.
- **Relay streams byte-for-byte and flush per line.** Buffering would destroy
  the token-by-token experience. Capture is a side effect of relaying, never a
  precondition for it.
- **Only chat endpoints are inspected** (`/v1/chat/completions`,
  `/api/v1/chat/completions`, `/api/chat`). Everything else — web UI, `/live`,
  `/api/tags`, `/mcp`, WebSocket upgrades — goes through `httputil.ReverseProxy`
  untouched.
- **Unknown request fields must survive.** The body is decoded to
  `map[string]any` and only `messages` is rewritten, so `tools`,
  `response_format` and any future field round-trip intact.

Both API flavours share one code path because OpenAI and Ollama both use a
top-level `messages` array; they differ only in where the reply sits in the
response (`choices[0].message` vs `message`), handled in `messages.go`.

### Stop timings are load-bearing (HAOS updates and backups)

Home Assistant stops the container for every HAOS update and every backup that
includes app data, so a stop that truncates is a routine-operation failure, not
an edge case. Two values must move together:

| Setting | Where | Value | Default |
|---------|-------|-------|---------|
| `timeout` | `config.yaml` | 30 | 10 (Supervisor `ATTR_TIMEOUT`) |
| `S6_SERVICES_GRACETIME` / `S6_KILL_FINISH_MAXTIME` | `Dockerfile` | 20000 | 3000 / 5000 |

`timeout` must stay **above** the S6 gracetime, or the Supervisor kills the
container while S6 is still waiting.

Measured on this image: start → `/live` ~1.5s, warm start → model ready ~3.7s,
clean stop **9-13s**. The stop is dominated by fixed waits in `lemond` (a 2s
"GPU driver cleanup" pause plus a second unload pass); evicting the model
itself is ~0.1s, so a bigger model does **not** make it much slower. Do not
"optimise" the timeout down on the assumption that stops are fast.

**The failure mode is silent.** With the S6 gracetime too low, S6 kills the
server mid-cleanup and the container still exits 0 — nothing in the exit code
reveals it. The only signal is `s6-svwait: fatal: timed out` in the log, which
`smoke-test.sh` now greps for and fails on.

### Context size and tool calling — verified facts

Two things that look like they should work a certain way, and don't:

- **`options.num_ctx` from the Ollama API is ignored.** Lemonade accepts the
  request (HTTP 200) but does not resize or reload the context; the model keeps
  the window it was loaded with. Verified on a running server by sending
  `num_ctx: 2048` against a model loaded at 8192 — no reload, `n_ctx` unchanged.
  So `ctx_size` in `config.yaml` is the *only* thing that sets the window, and
  it defaults to 8192 to match HA's `DEFAULT_NUM_CTX`. Do not document
  per-request `num_ctx` as working.
- **Tool calling works far better than an earlier note here claimed.** That note
  said the 230M model "rarely uses it" and answered in prose even with
  `tool_choice: "required"`. That was wrong, and the cause was the test: those
  probes sent tools with **no system prompt**. Measured properly on-device
  (26 runs at temp 0, plus 10 repeats per utterance at temp 0.8):

  | Condition | Tool call emitted |
  |-----------|-------------------|
  | tools, no system prompt | ~40% — otherwise claims it has no such capability |
  | tools + HA Assist system prompt | 80-100% |
  | + `tool_choice: "required"` | 100% |

  Action selection is near-perfect (23/23 correct on-vs-off, and 0/5 false
  tool calls on a non-control utterance like "what's the weather?").

  **The real limitation is argument grounding, not tool emission.** If the
  utterance does not contain the device's registered name, the model invents a
  plausible label ("Smart Plug") and HA's matcher rejects it — 0/20 correct at
  temp 0.8 for colloquial phrasing, and listing the entities in the system
  prompt does not fix it. A JSON-schema `enum` on the argument does **not**
  constrain it either; this llama.cpp build's `--jinja` grammar does not enforce
  enum on argument values. Anything dispatching these tool calls should
  fuzzy-match `name` against exposed entities rather than trusting it.

  Never benchmark tool calling without a system prompt — that is what produced
  the wrong conclusion the first time.

### Configuration Handling

`config.json` lives at `$HOME/.config/lemonade/config.json` — lemond 11.8.0
moved the config dir out of `.cache/lemonade` (XDG split; resolution order
`STATE_DIRECTORY` → `$XDG_CONFIG_HOME/lemonade` → `$HOME/.config/lemonade`).
lemond migrates legacy files only when the new path is missing, and cont-init
runs before lemond, so cont-init moves the legacy `config.json` itself before
writing anything. The cache dir (`.cache/lemonade`, downloaded backends) and
the HF model cache (`.cache/huggingface`) did not move.

App options are the source of truth. `cont-init.d/lemonade.sh` **merges** them
into `config.json` with `jq`, asserting only the keys the app exposes
(`host`, `port`, `log_level`, `ctx_size`, `max_loaded_models`,
`llamacpp.backend`, `telemetry.enabled`) and preserving everything else the
user or Lemonade wrote there. Do not overwrite the file wholesale — that would
discard installed backends and per-model recipe options on every restart.

`lemond` only accepts `--host`, `--port` and `--version` on the command line;
everything else must go through `config.json`.

### Version Updates
When updating version:
1. Update `ARG LEMONADE_VERSION` in Dockerfile
2. Update `version` in config.yaml
3. Update version in build.yaml args
4. Test on at least one architecture before committing

Watch for upstream renaming release assets — the app installs
`lemonade-embeddable-<version>-ubuntu-{arm64,x64}.tar.gz`, and the update
script verifies that asset exists before proposing a bump.

Upstream documents breaking path/layout changes per release on the migration
page: https://github.com/lemonade-sdk/lemonade/wiki/Migration. The update
workflow checks it automatically on every bump via
`.github/scripts/check-lemonade-migration.sh`: the PR gets `needs-review`
(blocking auto-merge) when the page documents a migration anywhere in the
range **(current, target]** — not just at the target, because a PR that skips
releases still crosses the boundaries in between — or when the page cannot be
fetched. Series mentions (`v11.7.x`) count as their whole interval, which
over-flags the "from" side of a heading; that bias is deliberate (a spurious
review costs a glance, a missed migration costs a device). When it fires:

- The page describes the **systemd packaging's** point of view. Translate to
  this container: HOME=/data/lemonade, no CACHE_DIRECTORY / STATE_DIRECTORY /
  HF_HOME set.
- Do not trust its scope claims — verify against lemond's path resolution
  source (`src/cpp/server/utils/platform/path_linux.cpp`, `path_utils.cpp`).
  Its "embedded installs are unaffected" note on the 11.8.0 config-dir move
  was wrong for Linux: the XDG split applies to any Linux process, and the
  smoke test caught the add-on's options being orphaned.

The tripwire only covers what upstream chooses to document; the smoke test
remains the backstop for undocumented breakage.

### Testing Checklist
- Build completes successfully
- `lemond --version` runs in the build (proves glibc closure is complete)
- Service starts and `/live` returns `{"status":"ok"}`
- Model downloads, loads, and answers a chat completion
- Ollama `/api/tags` lists the model
- Container stops with exit code 0 **while confined by apparmor.txt**
- Ingress access works through the Home Assistant sidebar
- Data persists across restarts

## Important Notes

- **Never commit changes** to version numbers without testing
- **Ingress** works because Lemonade serves the UI, the APIs and the WebSocket
  all on port 13305 (`ingress_stream: true` handles the upgrade)
- **First start downloads ~200 MB** (llama.cpp backend + default model). The
  `HEALTHCHECK` uses a 120 s start period to accommodate this.

## Common Issues and Troubleshooting

### Issue: model loads fine but the app is SIGKILLed on stop

**Symptoms:** exit code 137, no "Cleanup complete" in the log

**Cause:** AppArmor signal rule missing `receive`

**Solution:** ensure `apparmor.txt` has `signal (send,receive),`

### Issue: `llama-server` exits 127 right after the backend downloads

**Symptoms:** `error while loading shared libraries: <lib>`

**Cause:** the bundled glibc closure is missing a library the runtime-fetched
llama.cpp build needs

**Solution:** add the package to the `glibc-provider` stage and copy the
library explicitly, the way `libgomp1` is handled
