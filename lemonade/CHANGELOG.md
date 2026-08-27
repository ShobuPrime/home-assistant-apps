# Changelog

## 11.8.0

_2026-08-27_

## Headline

- A new experimental `ds4` (DwarfStar) backend serves DeepSeek V4 Flash on AMD Strix Halo GPUs.
- The catalog adds the RPG-HaloTales-V2 chat model, Flux-2-Klein-4B/9B image models, and new OpenMOSS voice-design and sound-effect speech models.
- Cloud providers now support custom authentication headers and byte-for-byte Anthropic Messages passthrough via `wire_format`, and `lemonade cloud list --json` reports provider readiness.
- Interrupted model downloads can be cancelled with Ctrl+C and resumed, capped with a new `download_rate_limit` config key, and long connections stay alive through TCP keepalive and streaming heartbeats.
- The new `lemonade update-models` command and global and per-model auto-update settings keep installed models current.

## Breaking Changes

- `LemonadeServer.exe --port` and `--host` are now ephemeral in-memory overrides and no longer persist to `config.json`; use `lemonade config set port=<P>` / `host=<H>` to persist them.
- Custom `*_args` precedence changed: explicit request `*_args` no longer inherit model/architecture args, and `merge_args=false` now suppresses all inherited backend/machine args and overridable runtime defaults.
- The llama.cpp backend now defaults `--parallel` to 1, removing implicit multi-slot concurrent batching; restore it with `--llamacpp-args "--parallel N"`.
- Legacy telemetry attributes `openinference.session.id` and `openinference.user.id` were removed in favor of the standard `session.id` and `user.id` keys; update downstream dashboards and queries.
- whisper.cpp transcription now defaults an omitted `language` to `auto` (source-language auto-detection) instead of English; pass `language=en` to force English.
- Removed the undocumented HTTP routes `/v1/params`, `/v1/log-level`, `/api/v1/test`, and `/status`; use `/internal/set` and `/internal/config` instead.
- Backend `latest` version resolution now includes pre-releases for all backends.

## Lemonade Server

| Operating System | Downloads |
|------------------|-----------|
| **Windows** | [lemonade.msi](https://github.com/lemonade-sdk/lemonade/releases/download/v11.8.0/lemonade.msi) |
| **Ubuntu 24.04+** | [Launchpad PPA](https://launchpad.net/~lemonade-team/+archive/ubuntu/stable) |
| **Debian 13 (x86_64)** | [lemonade-server_11.8.0-debian13_amd64.deb](https://github.com/lemonade-sdk/lemonade/releases/download/v11.8.0/lemonade-server_11.8.0-debian13_amd64.deb) |
| **Debian 13 (ARM64)** | [lemonade-server_11.8.0-debian13_arm64.deb](https://github.com/lemonade-sdk/lemonade/releases/download/v11.8.0/lemonade-server_11.8.0-debian13_arm64.deb) |
| **Fedora 43 (x86_64)** | [lemonade-server-11.8.0-fc43.x86_64.rpm](https://github.com/lemonade-sdk/lemonade/releases/download/v11.8.0/lemonade-server-11.8.0-fc43.x86_64.rpm) |
| **Fedora 43 (ARM64)** | [lemonade-server-11.8.0-fc43.aarch64.rpm](https://github.com/lemonade-sdk/lemonade/releases/download/v11.8.0/lemonade-server-11.8.0-fc43.aarch64.rpm) |
| **Fedora 44 (x86_64)** | [lemonade-server-11.8.0-fc44.x86_64.rpm](https://github.com/lemonade-sdk/lemonade/releases/download/v11.8.0/lemonade-server-11.8.0-fc44.x86_64.rpm) |
| **Fedora 44 (ARM64)** | [lemonade-server-11.8.0-fc44.aarch64.rpm](https://github.com/lemonade-sdk/lemonade/releases/download/v11.8.0/lemonade-server-11.8.0-fc44.aarch64.rpm) |
| **macOS** | [Lemonade-11.8.0-Darwin.pkg](https://github.com/lemonade-sdk/lemonade/releases/download/v11.8.0/Lemonade-11.8.0-Darwin.pkg) |

> **Other platforms?** See our [Installation Options](https://lemonade-server.ai/docs/guide/install/) for [Docker](https://lemonade-server.ai/docs/guide/install/docker/), [Snap](https://lemonade-server.ai/docs/guide/install/ubuntu/#__tabbed_2_3), [Arch](https://lemonade-server.ai/docs/guide/install/arch/), [Debian](https://lemonade-server.ai/docs/guide/install/), and more.

## Embeddable Lemonade

Portable binaries for bundling into your own installer. Run `lemond ./` as a subprocess.

| Platform | Download |
|----------|----------|
| **Ubuntu x64** | [lemonade-embeddable-11.8.0-ubuntu-x64.tar.gz](https://github.com/lemonade-sdk/lemonade/releases/download/v11.8.0/lemonade-embeddable-11.8.0-ubuntu-x64.tar.gz) |
| **Ubuntu arm64** | [lemonade-embeddable-11.8.0-ubuntu-arm64.tar.gz](https://github.com/lemonade-sdk/lemonade/releases/download/v11.8.0/lemonade-embeddable-11.8.0-ubuntu-arm64.tar.gz) |
| **Windows x64** | [lemonade-embeddable-11.8.0-windows-x64.zip](https://github.com/lemonade-sdk/lemonade/releases/download/v11.8.0/lemonade-embeddable-11.8.0-windows-x64.zip) |
| **macOS arm64** | [lemonade-embeddable-11.8.0-macos-arm64.tar.gz](https://github.com/lemonade-sdk/lemonade/releases/download/v11.8.0/lemonade-embeddable-11.8.0-macos-arm64.tar.gz) |

---

## What's Changed

Thanks `Bekhouche`, `GabrielReusRodriguez`, `Geramy`, `NineBallo`, `abn`, `anditherobot`, `bitgamma`, `blackdeathdrow`, `bong-water-water-bong`, `fl0rianr`, `github-actions`, `jeremyfowers`, `jgmelber`, `kenvandine`, `meghsat`, `original4422`, `pwilkin`, `ramkrishna2910`, `sjjh`, `soothill`, `superm1`, `zaneni6` for your awesome contributions to this release!

<details>
<summary>Click to expand changelog</summary>

* Add missing handler for lemonade:// URLs (Closes: #1144656) by `superm1` in https://github.com/lemonade-sdk/lemonade/pull/3195
* fix(GUI app): stop chat from bypassing collection.router routing by `meghsat` in https://github.com/lemonade-sdk/lemonade/pull/3180
* Fix the developer setup page URLs by `superm1` in https://github.com/lemonade-sdk/lemonade/pull/3225
* fix(rocm): only stage amdhip64_7.dll when TheRock runtime is newer than System32 by `blackdeathdrow` in https://github.com/lemonade-sdk/lemonade/pull/3217
* docs: add latest release and Lemonade AI YouTube videos to news feed by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3202
* feat(cloud): configurable auth header name and prefix per provider by `jgmelber` in https://github.com/lemonade-sdk/lemonade/pull/3222
* fix: cancel model download on Ctrl+C (SIGINT) via libcurl progress callback (#1385) by `bong-water-water-bong` in https://github.com/lemonade-sdk/lemonade/pull/2462
* feat(server): record the backend launch command in /health by `anditherobot` in https://github.com/lemonade-sdk/lemonade/pull/3229
* chore: rename app identifiers to ai.lemonadeserver namespace by `abn` in https://github.com/lemonade-sdk/lemonade/pull/1978
* docs: add MCP Gateway to the API Spec nav by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3242
* Update the contribution guide for H2 2026 by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3246
* Add HaloTales-V2 and related models by `bitgamma` in https://github.com/lemonade-sdk/lemonade/pull/3264
* Flm version update by `zaneni6` in https://github.com/lemonade-sdk/lemonade/pull/3266
* fix(server): treat --port and --host CLI flags as ephemeral overrides by `abn` in https://github.com/lemonade-sdk/lemonade/pull/3249
* Move persistent Lemonade JSON state from `.cache` to `.config` by `superm1` with `Copilot` in https://github.com/lemonade-sdk/lemonade/pull/3028
* feat(server): hide backends with no runnable models on this host by `ramkrishna2910` in https://github.com/lemonade-sdk/lemonade/pull/3219
* feat(telemetry): configurable session ID ingestion and OpenInference tool-call capture by `abn` in https://github.com/lemonade-sdk/lemonade/pull/3248
* feat(server): treat config.json as sparse user overrides with deep merging by `abn` in https://github.com/lemonade-sdk/lemonade/pull/3268
* test(telemetry): prevent dangling RuntimeConfig pointer and worker race in telemetry helpers test by `abn` in https://github.com/lemonade-sdk/lemonade/pull/3283
* fix(server): resolve custom args by scope by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3265
* feat(server): enable cross-platform tcp keepalive socket options by `abn` in https://github.com/lemonade-sdk/lemonade/pull/3209
* ci: temporarily disable tts-openmoss integration tests by `Geramy` in https://github.com/lemonade-sdk/lemonade/pull/3291
* test: fix dangling CLI temp path in runtime override test by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3288
* test: move multi-checkpoint download-state coverage to C++ by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3286
* test: move llamacpp system backend policy coverage to C++ by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3287
* test: migrate model registry label validation to C++ by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3285
* feat(server): add context_length to the models endpoints by `anditherobot` in https://github.com/lemonade-sdk/lemonade/pull/3280
* feat(cli): add --json output to lemonade cloud list by `GabrielReusRodriguez` in https://github.com/lemonade-sdk/lemonade/pull/3261
* feat(streaming_proxy): add sse comment heartbeats during prompt prefill by `abn` in https://github.com/lemonade-sdk/lemonade/pull/3210
* test(server): assert CLI parse return status and remove unused header in runtime override test by `abn` in https://github.com/lemonade-sdk/lemonade/pull/3294
* fix(router): handle transient GPU hangs and compute errors in lemond by `abn` in https://github.com/lemonade-sdk/lemonade/pull/2652
* feat(sync): add administrative model synchronization and auto-update configuration by `abn` in https://github.com/lemonade-sdk/lemonade/pull/2779
* Update stable-diffusion.cpp to master-827-97d2990 and CUDA master-827-e2329c3 by `github-actions`[bot] in https://github.com/lemonade-sdk/lemonade/pull/3311
* [Stable Diffusion] Respect the user configurable "Model Options" by `NineBallo` in https://github.com/lemonade-sdk/lemonade/pull/2845
* Handle llama.cpp new release scheme by `bitgamma` in https://github.com/lemonade-sdk/lemonade/pull/3315
* Update faq.md: correcting list format by `sjjh` in https://github.com/lemonade-sdk/lemonade/pull/3299
* fix(server): release large request memory by `soothill` in https://github.com/lemonade-sdk/lemonade/pull/2873
* feat(server): size-filter streaming backends by working set, not full size by `ramkrishna2910` in https://github.com/lemonade-sdk/lemonade/pull/3328
* docs: document POST /v1/install/dry-run by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3324
* feat(router): add min_total_chars / max_total_chars whole-conversation length conditions (#2958) by `meghsat` in https://github.com/lemonade-sdk/lemonade/pull/3181
* docs: document POST /internal/simulate-vram-pressure by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3325
* feat(routing): add expected_output_tokens signal to RouteContext by `Bekhouche` in https://github.com/lemonade-sdk/lemonade/pull/3163
* docs: document POST /v1/routing/validate by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3323
* Retire undocumented /params, /log-level, /api/v1/test, and /status routes by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3322
* Add TheNoise-based upscalers by `bitgamma` in https://github.com/lemonade-sdk/lemonade/pull/3327
* Require 2 reviewers for breaking changes by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3348
* ci: retry apt-get on transient arm64 mirror 404s by `kenvandine` in https://github.com/lemonade-sdk/lemonade/pull/3353
* fix(vllm): scale gpu-memory-utilization to free memory on shared-memory devices by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3344
* test(server): eliminate timing flakiness in StreamingHeartbeatTest by `abn` in https://github.com/lemonade-sdk/lemonade/pull/3338
* fix(openmoss): bump OpenMOSS TTS pin to v0.3.0 and re-enable CI by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3335
* docs: add smart router subject area and two router maintainers by `ramkrishna2910` in https://github.com/lemonade-sdk/lemonade/pull/3351
* Bump project version from 11.7.0 to 11.8.0 by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3354
* Update faq.md removed double tts chapter by `sjjh` in https://github.com/lemonade-sdk/lemonade/pull/3300
* feat(cli): return per-model detail from status --json by `anditherobot` in https://github.com/lemonade-sdk/lemonade/pull/3259
* fix(llamacpp): pin --parallel 1 so ctx_size is not oversubscribed by `jgmelber` in https://github.com/lemonade-sdk/lemonade/pull/3277
* feat(config): add configurable download rate limit by `blackdeathdrow` in https://github.com/lemonade-sdk/lemonade/pull/3281
* fix(whisper): auto-detect omitted transcription language by `original4422` in https://github.com/lemonade-sdk/lemonade/pull/3245
* Feat/openmoss 0.3.0 backport/server side by `pwilkin` in https://github.com/lemonade-sdk/lemonade/pull/2863
* feat(cloud): relay Anthropic-wire-format providers through /v1/messages by `jgmelber` in https://github.com/lemonade-sdk/lemonade/pull/3223
* feat(backends): ds4 (DwarfStar) backend for DeepSeek V4 Flash [experimental] by `ramkrishna2910` in https://github.com/lemonade-sdk/lemonade/pull/3047

</details>

## New Contributors
* `jgmelber` made their first contribution in https://github.com/lemonade-sdk/lemonade/pull/3222
* `sjjh` made their first contribution in https://github.com/lemonade-sdk/lemonade/pull/3299
* `soothill` made their first contribution in https://github.com/lemonade-sdk/lemonade/pull/2873
* `original4422` made their first contribution in https://github.com/lemonade-sdk/lemonade/pull/3245

**Full Changelog**: https://github.com/lemonade-sdk/lemonade/compare/v11.7.0...v11.8.0

---

> Windows installers are signed. Free code signing provided by [SignPath.io](https://signpath.io), certificate by [SignPath Foundation](https://signpath.org). See our [Code Signing Policy](https://github.com/lemonade-sdk/lemonade#code-signing-policy).

---


## 11.7.0

_2026-08-21_

## Headline

- A new `POST /v1/models/register` endpoint registers or updates `user.*` model definitions without downloading any files.
- New `GET/POST/DELETE /v1/models/{id}/options` endpoints save, inspect, and reset per-model recipe options without loading the model, with per-load transient overrides.
- New `GET /v1/stats` and Prometheus `/metrics` endpoints report prefix-cache effectiveness and router route-switch counters.
- The built-in catalog adds the Qwen3.8-27B and NVIDIA Nemotron 3.5 Lightning 30B-A3B GGUF models and the Z-Image-Turbo image model.
- Lemonade can now be installed on Windows with `winget` and on macOS with a Homebrew cask.

## Breaking Changes

- `POST /pull` now returns 400 and registers nothing when a model definition names an unservable or contradictory deployment mode; such labels are no longer silently normalized.
- Stored `user_models.json` entries that violate the new deployment-mode label rule are now skipped at startup with an error instead of being repaired.
- Removed the deprecated `Lite Collection` and `Ultra Collection` model registry entries.
- `/v1/audio/transcriptions` now returns raw text bodies for `text`/`srt`/`vtt` instead of a JSON `{text: ...}` wrapper, and returns 400 for unsupported `response_format` values.
- The TheNoise image backend no longer accepts per-request `upscale` and `lora_dir` recipe options; `lora_dir` is now set server-wide in `config.json` and `upscale` is only honored as a passed-through request parameter.
- `lemonade launch opencode` now emits a `limit` object (`limit.context`/`limit.output`) instead of a top-level `contextWindow` field.
- The `no_broadcast` config key is replaced by an inverted `broadcast` boolean, auto-migrated on load.
- An existing but unreadable `extra_models_dir` now returns 400 on `/internal/set` instead of being silently applied.

## Lemonade Server

| Operating System | Downloads |
|------------------|-----------|
| **Windows** | [lemonade.msi](https://github.com/lemonade-sdk/lemonade/releases/download/v11.7.0/lemonade.msi) |
| **Ubuntu 24.04+** | [Launchpad PPA](https://launchpad.net/~lemonade-team/+archive/ubuntu/stable) |
| **Debian 13 (x86_64)** | [lemonade-server_11.7.0-debian13_amd64.deb](https://github.com/lemonade-sdk/lemonade/releases/download/v11.7.0/lemonade-server_11.7.0-debian13_amd64.deb) |
| **Debian 13 (ARM64)** | [lemonade-server_11.7.0-debian13_arm64.deb](https://github.com/lemonade-sdk/lemonade/releases/download/v11.7.0/lemonade-server_11.7.0-debian13_arm64.deb) |
| **Fedora 43 (x86_64)** | [lemonade-server-11.7.0-fc43.x86_64.rpm](https://github.com/lemonade-sdk/lemonade/releases/download/v11.7.0/lemonade-server-11.7.0-fc43.x86_64.rpm) |
| **Fedora 43 (ARM64)** | [lemonade-server-11.7.0-fc43.aarch64.rpm](https://github.com/lemonade-sdk/lemonade/releases/download/v11.7.0/lemonade-server-11.7.0-fc43.aarch64.rpm) |
| **Fedora 44 (x86_64)** | [lemonade-server-11.7.0-fc44.x86_64.rpm](https://github.com/lemonade-sdk/lemonade/releases/download/v11.7.0/lemonade-server-11.7.0-fc44.x86_64.rpm) |
| **Fedora 44 (ARM64)** | [lemonade-server-11.7.0-fc44.aarch64.rpm](https://github.com/lemonade-sdk/lemonade/releases/download/v11.7.0/lemonade-server-11.7.0-fc44.aarch64.rpm) |
| **macOS** | [Lemonade-11.7.0-Darwin.pkg](https://github.com/lemonade-sdk/lemonade/releases/download/v11.7.0/Lemonade-11.7.0-Darwin.pkg) |

> **Other platforms?** See our [Installation Options](https://lemonade-server.ai/docs/guide/install/) for [Docker](https://lemonade-server.ai/docs/guide/install/docker/), [Snap](https://lemonade-server.ai/docs/guide/install/ubuntu/#__tabbed_2_3), [Arch](https://lemonade-server.ai/docs/guide/install/arch/), [Debian](https://lemonade-server.ai/docs/guide/install/), and more.

## Embeddable Lemonade

Portable binaries for bundling into your own installer. Run `lemond ./` as a subprocess.

| Platform | Download |
|----------|----------|
| **Ubuntu x64** | [lemonade-embeddable-11.7.0-ubuntu-x64.tar.gz](https://github.com/lemonade-sdk/lemonade/releases/download/v11.7.0/lemonade-embeddable-11.7.0-ubuntu-x64.tar.gz) |
| **Ubuntu arm64** | [lemonade-embeddable-11.7.0-ubuntu-arm64.tar.gz](https://github.com/lemonade-sdk/lemonade/releases/download/v11.7.0/lemonade-embeddable-11.7.0-ubuntu-arm64.tar.gz) |
| **Windows x64** | [lemonade-embeddable-11.7.0-windows-x64.zip](https://github.com/lemonade-sdk/lemonade/releases/download/v11.7.0/lemonade-embeddable-11.7.0-windows-x64.zip) |
| **macOS arm64** | [lemonade-embeddable-11.7.0-macos-arm64.tar.gz](https://github.com/lemonade-sdk/lemonade/releases/download/v11.7.0/lemonade-embeddable-11.7.0-macos-arm64.tar.gz) |

---

## What's Changed

Thanks `SlawomirNowaczyk`, `abn`, `anditherobot`, `bitgamma`, `blackdeathdrow`, `bong-water-water-bong`, `fl0rianr`, `ianbmacdonald`, `iswaryaalex`, `jeremyfowers`, `kenvandine`, `mzyy94`, `osimarr`, `popey`, `ramkrishna2910`, `superm1`, `yelliver`, `zaneni6` for your awesome contributions to this release!

<details>
<summary>Click to expand changelog</summary>

* Bump TheNoise to version v0.1.2 by `bitgamma` in https://github.com/lemonade-sdk/lemonade/pull/3079
* docs(backends): drop the model catalog tables from the backend reference by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3103
* fix(alias): report the real reason for alias validation failures by `ramkrishna2910` in https://github.com/lemonade-sdk/lemonade/pull/3111
* feat(server): add model registration endpoint by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3118
* Debian packaging improvements by `superm1` in https://github.com/lemonade-sdk/lemonade/pull/3039
* docs: add winget and Homebrew install to homepage quickstart by `yelliver` in https://github.com/lemonade-sdk/lemonade/pull/2970
* Trigger snap release candidate build from release branches by `kenvandine` in https://github.com/lemonade-sdk/lemonade/pull/2908
* fix(server): compare artifacts, not commit SHAs, when checking for model updates by `blackdeathdrow` in https://github.com/lemonade-sdk/lemonade/pull/3073
* fix(server): normalize thinking controls for llama.cpp by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3132
* fix: stop request during prefill now possible by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3133
* ci: stop building unused targets in the validate workflows by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3052
* fix(server): harden extra models directory handling by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3105
* Fix #1833: Improve transcription response format compatibility by `anditherobot` in https://github.com/lemonade-sdk/lemonade/pull/2088
* ci: stop pinning lite validation legs to the 128gb runner by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3139
* feat(models): add an explicit `chat` label by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3099
* feat(telemetry): capture prefix-cache effectiveness (cache_n, cached_tokens) and route-switch counts by `ramkrishna2910` in https://github.com/lemonade-sdk/lemonade/pull/2968
* Update FLM version by `zaneni6` in https://github.com/lemonade-sdk/lemonade/pull/3144
* Backend Battle Nightly Regression by `iswaryaalex` in https://github.com/lemonade-sdk/lemonade/pull/3069
* Remove emoji from FAQ title by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3154
* Manage per-model recipe options without loading by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3110
* Enable % used context on opencode side panel by `osimarr` in https://github.com/lemonade-sdk/lemonade/pull/1725
* feat(server,cli): add discovery and broadcast controls with config decoupling by `abn` in https://github.com/lemonade-sdk/lemonade/pull/3135
* fix(packaging): add AppStream MetaInfo files for desktop entries by `superm1` in https://github.com/lemonade-sdk/lemonade/pull/3123
* fix(flm): correctly report downloaded model size by `mzyy94` in https://github.com/lemonade-sdk/lemonade/pull/3166
* Minor refactor of new gate code  by `SlawomirNowaczyk` in https://github.com/lemonade-sdk/lemonade/pull/2971
* update TheNoise v0.2.1 by `bitgamma` in https://github.com/lemonade-sdk/lemonade/pull/3171
* fix(flm): stop re-pulling already-downloaded models on every load by `mzyy94` in https://github.com/lemonade-sdk/lemonade/pull/3167
* feat: flag comment slop in a pre-commit rule, and run pre-commit in CI by `ianbmacdonald` in https://github.com/lemonade-sdk/lemonade/pull/2689
* add lemonade version to benchmark results by `bitgamma` in https://github.com/lemonade-sdk/lemonade/pull/3173
* feat(server): support transient null overrides on load by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3172
* fix(macos): reduce log directory permissions to prevent privilege esc… by `superm1` in https://github.com/lemonade-sdk/lemonade/pull/2625
* docs: add pull request template by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/2551
* fix(npu): pre-reject NPU loads when auto-tune can only reserve fallback ctx (#1151) by `bong-water-water-bong` in https://github.com/lemonade-sdk/lemonade/pull/3164
* models: add Qwen3.8 and Nemotron 3.5 GGUF by `yelliver` in https://github.com/lemonade-sdk/lemonade/pull/3177
* Fix pre-commit JSON validation issues by `superm1` in https://github.com/lemonade-sdk/lemonade/pull/3179
* docs: fix multi-file checkpoint rendering by `yelliver` in https://github.com/lemonade-sdk/lemonade/pull/3174
* Remove some MTP default flags by `bitgamma` in https://github.com/lemonade-sdk/lemonade/pull/3187
* ci: wire server eviction tests into CI by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3015
* ci: run watchdog lifecycle tests in backend CI by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3021
* test: replace Python system info replicas with C++ coverage by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3020
* fix(server): let load args override saved args by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3204
* refactor(server): drop load_command from the options endpoint by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3199
* fix(test): new watchdog test concurrency issue by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3201
* fix(pre-commit): make comment-slop runnable on Windows by `blackdeathdrow` in https://github.com/lemonade-sdk/lemonade/pull/3208
* Update to ROCm 7.14 by `superm1` in https://github.com/lemonade-sdk/lemonade/pull/2768
* Fix stale gfx90X-dcgpu TheRock URL mapping for gfx908/gfx90a by `kenvandine` in https://github.com/lemonade-sdk/lemonade/pull/3196
* test: add concurrent chat completion coverage by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/2060
* Fail startup when one resolved address cannot bind by `popey` in https://github.com/lemonade-sdk/lemonade/pull/3197
* fix(ci): resolve test flakes across C++ store, python test harness, and workflows by `abn` in https://github.com/lemonade-sdk/lemonade/pull/2805

</details>

## New Contributors
* `yelliver` made their first contribution in https://github.com/lemonade-sdk/lemonade/pull/2970
* `zaneni6` made their first contribution in https://github.com/lemonade-sdk/lemonade/pull/3144
* `mzyy94` made their first contribution in https://github.com/lemonade-sdk/lemonade/pull/3166

**Full Changelog**: https://github.com/lemonade-sdk/lemonade/compare/v11.6.0...v11.7.0

---

> Windows installers are signed. Free code signing provided by [SignPath.io](https://signpath.io), certificate by [SignPath Foundation](https://signpath.org). See our [Code Signing Policy](https://github.com/lemonade-sdk/lemonade#code-signing-policy).

---


## 11.5.1

_2026-07-31_

> _Maintenance (2026-08-03):_ fixed the panel opening normally but failing on
> every action — "Failed to load model", chat that never sends.
>
> Lemonade only accepts requests from web addresses it recognises, and Home
> Assistant's own address has to be one of them. The add-on looked that address
> up too early in the boot, before Home Assistant was running, so on most
> restarts it never got one. Restarting the add-on by hand fixed it, which is
> why the problem came and went.
>
> The lookup now happens once Home Assistant is up, and refreshes afterwards,
> so it survives a reboot and picks up a changed address without a restart.
> Refused addresses are logged, so if this ever happens again the add-on log
> tells you exactly what to put in the new `allowed_origins` option (now
> documented in DOCS.md — mainly needed for Nabu Casa remote access).
>
> Internally, the bridge process now always runs, since it performs this check.
> Memory and speech-to-text are unchanged and still off unless you enable them.

> _Maintenance (2026-08-03, follow-up):_ fixed the same 403 when opening Home
> Assistant by IP, e.g. `https://192.0.2.10:8123`, while the normal address
> worked fine.
>
> The add-on knew the device's addresses but only trusted them over plain
> `http`. Home Assistant is usually served over `https`, and addresses are
> matched exactly, so the secure form was refused. Both are now accepted.
>
> Address discovery also moved into the running add-on and refreshes, so a new
> IP from your router no longer needs a restart to be trusted.

## Headline

- `lemonade bench` adds an image-generation benchmark mode with capability-aware scenario and model filtering, response capture, and a new `--timeout` flag.
- The Router Builder gains a Test Prompt tab, backed by a new `POST /routing/validate` endpoint, that runs a routing policy against a sample prompt and shows the decision, a step-by-step trace, and a decision-tree view.
- Ten MiniCPM text and vision GGUF models from ModelScope join the built-in llama.cpp catalog.
- Tool-calling requests to llama.cpp with large JSON schema bounds are now accepted, working around a grammar limit that previously rejected valid tool calls.

## Breaking Changes

- llama.cpp non-streaming responses now return the requested or registered model id in the `model` field instead of the local `.gguf` absolute path.
- Registered and imported collections (`user.*`/`extra.*`) now list under their canonical prefixed id on `/v1/models`, Ollama `/api/tags`, and MCP `lemonade_list_models`.
- `auto` backe

---







## 11.5.0

_2026-07-25_

> _Maintenance (2026-07-26):_ **Lemonade can now be your Home Assistant speech-to-text engine, and the helper service is renamed.** Turn on "Enable speech-to-text" and Lemonade appears in Settings > Voice assistants under Speech-to-text, next to options like faster-whisper — discovered automatically, with nothing to type in. Combined with the Ollama integration for the conversation agent and Piper for the voice, that completes a fully local voice assistant: nothing leaves the house.
>
> Defaults to **Moonshine-Small-Streaming**, chosen by measurement on a Raspberry Pi CM5: it made about half as many mistakes as the Tiny model on noisy audio and was both faster *and* more accurate than Medium, while still transcribing quicker than real time. Download the model from the Web UI first. The model is kept loaded by default so the first command after a quiet spell does not wait several seconds for it to load.
>
> Text-to-speech is deliberately **not** offered — this hardware has no supported engine (measured: the only candidate ran about twice as slow as real time, against Piper's four-times-faster-than-real-time), and advertising one would put a broken entry in your dropdown. Keep using Piper.
>
> The helper service that fronts Lemonade is renamed from `memproxy` to **`ha-lemonade-bridge`**: it long ago stopped being only a memory proxy, and now also repairs the Web UI for ingress, handles browser origins, and serves speech-to-text. No user-visible settings changed as a result. Speech-to-text is off by default, so nothing changes until you turn it on.

> _Maintenance (2026-07-26, follow-up):_ **Fix speech-to-text and other ONNX models failing to load.** Loading Moonshine failed with `Failed to load model: moonshine-server failed to start or become ready`, and the real cause was two directories down in the log: `moonshine-server: error while loading shared libraries: libdl.so.2`, exit 127. glibc 2.34 merged `libdl`, `libpthread` and `librt` into the main C library and now ships them only as small compatibility stubs. Lemonade's own server does not reference them, so they never entered the bundled library set — but the backends Lemonade downloads at runtime do (`moonshine-server` needs libdl and libpthread; the ONNX runtime also needs librt). All five stubs are now bundled, ~121 KB, unblocking Moonshine speech-to-text and every ONNX-based model. Rebuild the app to pick this up.

> _Maintenance (2026-07-26):_ **Make the Web UI actually usable: fix chat returning 403, the ingress panel stuck on "connecting", and the Android app redirecting to the Play Store.** Three separate upstream behaviours, all invisible from the API side.
>
> **1. `403 {"error": "Origin not allowed"}` when chatting.** `lemond` only accepts requests from Origins it recognises — sound protection against a random website driving your LLM API via DNS rebinding, but its built-in defaults cover `localhost`/`127.0.0.1` only. Every Home Assistant route in fails: direct LAN access sends `http://<device-ip>:13305`, and the ingress panel (including the Companion app) sends Home Assistant's own URL. GETs pass, so the model list loaded and only chatting broke, which made it look like a chat bug rather than an Origin check. The add-on now derives the allowlist at startup and exports it as `LEMONADE_ALLOWED_ORIGINS`: Home Assistant's `internal_url`/`external_url`, plus every host address at both the add-on's *published* port (reaching this Web UI directly) and Home Assistant's own port (reaching HA by IP or `homeassistant.local` rather than by its configured URL, which makes the panel carry that origin instead). A new `allowed_origins` option covers anything else, e.g. a custom domain. Requires the new `homeassistant_api` permission to read the HA URLs. This is not the API key; the check applies with no key set.
>
> **2. Ingress panel stuck on "connecting", with an empty log view.** In browser mode the app sets its API base to `window.location.origin` — origin only, no path. Under ingress the page lives at `/api/hassio_ingress/<token>/web-app/`, so every API call and the log-stream WebSocket went to the Home Assistant frontend root and never reached the add-on. (The log view is a WebSocket, which is why it failed identically.) A small shim now publishes the correct base — origin plus the ingress prefix, and the bare origin on direct access — and the bundle prefers it.
>
> **3. Android redirected to the Google Play store.** The app's entry module redirects on any Android user-agent, and the branch it skips is the one that mounts the UI — so the panel never rendered at all in the Companion app. Home Assistant only supplies the user-agent; Chrome on Android does the same, so no Companion setting could fix it. The shim hides the "Android" token so the app mounts, leaving "Mobile Safari" intact so it still knows it is on a phone.
>
> Both patches assert their anchors matched and **fail the build** otherwise, since upstream ships often and a silently-skipped patch would quietly restore the bug. Rebuild the app to pick these up.

> _Maintenance (2026-07-25, follow-up):_ **Fix the ingress panel rendering blank.** The `ingress_entry` added below was written `/web-app/`, with a leading slash. Supervisor appends that to a base URL that already ends in one, and the ingress proxy prepends another when forwarding, so the add-on received `//web-app/…`. `lemond` tolerates the doubled slash on the directory route (`//web-app/` → 200) but not on asset paths (`//web-app/renderer.bundle.js` → 404), so the panel fetched its HTML and then rendered nothing, with the 404 visible only in the browser console. The value is now `web-app/` — no leading slash, trailing slash retained (the app references its assets relatively, so it still needs that one).

> _Maintenance (2026-07-25):_ **Ship the actual Web UI.** The app copied only `resources/static/` out of upstream's image — the "Lemonade Server" landing page — and not `resources/web-app/`, which is the chat application itself, served at `/web-app/`. The result looked like a working UI: the root page loaded, nothing 404'd in the log, and the panel showed a plausible page. But the application was absent from the image entirely, `/web-app/` returned 404, and nothing on the landing page links to it, so there was no way to notice from inside the UI. Both directories are now copied, and `ingress_entry: /web-app` points the panel at the application instead of the landing page (its assets are referenced relatively, so it works behind ingress path prefixing). Adds ~4.5 MB. Rebuild the app to pick this up.

### Initial release

- Initial Home Assistant app for Lemonade
- Serves local LLMs over OpenAI-, Ollama- and Anthropic-compatible APIs on port 13305
- Works with Home Assistant's built-in Ollama integration as a conversation agent
- Downloads and preloads Liquid AI's LFM2.5-230M on first start by default
- CPU inference via llama.cpp, suitable for Home Assistant Yellow / Green (CM5)
- Ingress support for sidebar integration
- Scans `/share/lemonade_models` for GGUF files you drop in yourself, listed under their filename
- Optional long-term memory backed by the MuninnDB app (`memory_enabled`, off by default)
- Can also serve a text-embedding model for MuninnDB (`embedding_model_name`, off by default)
- Models, backends and settings persist under `/data/lemonade`
- Automatic version update checks

> _Fix (2026-07-25):_ **Web UI returned "index.html not found" on a fresh
> install.** The upstream "embeddable" archive this app builds from ships the
> binaries and JSON resources but no web assets, so the server answered every
> API call correctly while the ingress panel 404'd. The UI is now copied from
> upstream's container image at the matching version tag. The smoke test opens
> the UI so this cannot ship again.

#### Long-term memory

Optionally pairs with the MuninnDB app to remember conversations across
sessions and recall relevant ones later. Off by default; when off, nothing is
added between Home Assistant and the model. If MuninnDB is missing, unreachable
or slow, answers are served without memory rather than failing.

#### Packaging notes

Built on the repo-standard Alpine base with a bundled glibc runtime, since
upstream ships glibc-only binaries. Context size defaults to 8192 to match
Home Assistant. See CLAUDE.md for the packaging details.
