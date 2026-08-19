# Changelog

## 11.6.0

_2026-08-19_

## Headline

- The `Muse-Glimmer-30B-GGUF` model joins the built-in llama.cpp catalog with draft decoding, vision, and tool-calling support.
- The llama.cpp ROCm backend now runs on AMD Instinct MI100, MI200, MI210, and MI250 GPUs on Linux.
- A new experimental TheNoise ROCm image-generation backend brings Anima and Krea-2 to AMD Strix Halo and Strix Point iGPUs.
- The new `lemonade alias` commands let you assign your own names to models, with alias-aware `/v1/models` listing and admin endpoints.
- `lemonade bench` adds opt-in vision benchmark scenarios for multimodal models.

## Breaking Changes

- `/v1/audio/speech` with `stream_format=audio` and a non-PCM `response_format` (such as `mp3`) against Kokoro now returns HTTP 400 (only `pcm` is supported) instead of silently returning PCM audio.
- `/v1/models` now lists each GGUF quant in a split `extra_models_dir` subfolder as its own entry rather than one entry per folder; the old folder name still resolves as a hidden request alias.
- The Windows MSI layout changed: `Lemonade_Server_MSI` now contains only `lemonade-server-minimal.msi`, and the desktop `lemonade.msi` now ships in a new `Lemonade_Desktop_MSI` artifact, so automation that reads `lemonade.msi` from the old artifact must be repointed.

## Lemonade Server

| Operating System | Downloads |
|------------------|-----------|
| **Windows** | [lemonade.msi](https://github.com/lemonade-sdk/lemonade/releases/download/v11.6.0/lemonade.msi) |
| **Ubuntu 24.04+** | [Launchpad PPA](https://launchpad.net/~lemonade-team/+archive/ubuntu/stable) |
| **Debian 13 (x86_64)** | [lemonade-server_11.6.0-debian13_amd64.deb](https://github.com/lemonade-sdk/lemonade/releases/download/v11.6.0/lemonade-server_11.6.0-debian13_amd64.deb) |
| **Debian 13 (ARM64)** | [lemonade-server_11.6.0-debian13_arm64.deb](https://github.com/lemonade-sdk/lemonade/releases/download/v11.6.0/lemonade-server_11.6.0-debian13_arm64.deb) |
| **Fedora 43 (x86_64)** | [lemonade-server-11.6.0-fc43.x86_64.rpm](https://github.com/lemonade-sdk/lemonade/releases/download/v11.6.0/lemonade-server-11.6.0-fc43.x86_64.rpm) |
| **Fedora 43 (ARM64)** | [lemonade-server-11.6.0-fc43.aarch64.rpm](https://github.com/lemonade-sdk/lemonade/releases/download/v11.6.0/lemonade-server-11.6.0-fc43.aarch64.rpm) |
| **Fedora 44 (x86_64)** | [lemonade-server-11.6.0-fc44.x86_64.rpm](https://github.com/lemonade-sdk/lemonade/releases/download/v11.6.0/lemonade-server-11.6.0-fc44.x86_64.rpm) |
| **Fedora 44 (ARM64)** | [lemonade-server-11.6.0-fc44.aarch64.rpm](https://github.com/lemonade-sdk/lemonade/releases/download/v11.6.0/lemonade-server-11.6.0-fc44.aarch64.rpm) |
| **macOS** | [Lemonade-11.6.0-Darwin.pkg](https://github.com/lemonade-sdk/lemonade/releases/download/v11.6.0/Lemonade-11.6.0-Darwin.pkg) |

> **Other platforms?** See our [Installation Options](https://lemonade-server.ai/docs/guide/install/) for [Docker](https://lemonade-server.ai/docs/guide/install/docker/), [Snap](https://lemonade-server.ai/docs/guide/install/ubuntu/#__tabbed_2_3), [Arch](https://lemonade-server.ai/docs/guide/install/arch/), [Debian](https://lemonade-server.ai/docs/guide/install/), and more.

## Embeddable Lemonade

Portable binaries for bundling into your own installer. Run `lemond ./` as a subprocess.

| Platform | Download |
|----------|----------|
| **Ubuntu x64** | [lemonade-embeddable-11.6.0-ubuntu-x64.tar.gz](https://github.com/lemonade-sdk/lemonade/releases/download/v11.6.0/lemonade-embeddable-11.6.0-ubuntu-x64.tar.gz) |
| **Ubuntu arm64** | [lemonade-embeddable-11.6.0-ubuntu-arm64.tar.gz](https://github.com/lemonade-sdk/lemonade/releases/download/v11.6.0/lemonade-embeddable-11.6.0-ubuntu-arm64.tar.gz) |
| **Windows x64** | [lemonade-embeddable-11.6.0-windows-x64.zip](https://github.com/lemonade-sdk/lemonade/releases/download/v11.6.0/lemonade-embeddable-11.6.0-windows-x64.zip) |
| **macOS arm64** | [lemonade-embeddable-11.6.0-macos-arm64.tar.gz](https://github.com/lemonade-sdk/lemonade/releases/download/v11.6.0/lemonade-embeddable-11.6.0-macos-arm64.tar.gz) |

---

## What's Changed

Thanks `Bekhouche`, `SlawomirNowaczyk`, `Yigtwxx`, `ZMXJJ`, `abn`, `anditherobot`, `bitgamma`, `blackdeathdrow`, `ckuethe`, `duggiefresh`, `fl0rianr`, `github-actions`, `hogeheer499-commits`, `jeremyfowers`, `kenvandine`, `popey`, `ramkrishna2910`, `sreeram-11`, `storm1er`, `superm1` for your awesome contributions to this release!

<details>
<summary>Click to expand changelog</summary>

* fix(ci): prevent MSI verify flake from stale ARP entries on shared runners by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/2895
* docs: add testing guide by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/2891
* fix(build): patch libwebsockets GENHDR OUTPUT to silence MSB8065 by `blackdeathdrow` in https://github.com/lemonade-sdk/lemonade/pull/2893
* ci: stop re-running the full suite on push to main by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/2899
* fix(cli): respect explicit --port when --host includes a scheme by `kenvandine` in https://github.com/lemonade-sdk/lemonade/pull/2914
* README.md - Fix download links by `duggiefresh` in https://github.com/lemonade-sdk/lemonade/pull/2827
* ci: run packaging, PPA, distro, macOS and backend-validation jobs in the merge queue by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/2911
* New helper combining add_test() and register_cpp_ci_test() by `SlawomirNowaczyk` in https://github.com/lemonade-sdk/lemonade/pull/2877
* [llamacpp] replace deprecated flags, better IO flag handling by `bitgamma` in https://github.com/lemonade-sdk/lemonade/pull/2833
* Add /rerank as the reranking endpoint path (alias /reranking) by `sreeram-11` in https://github.com/lemonade-sdk/lemonade/pull/2924
* fix(server): keep interrupted variantless model downloads resumable by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/2876
* test: cover embeddings missing model error by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/2061
* fix(docs): Added group_add configuration for ROCm support in Docker. by `storm1er` in https://github.com/lemonade-sdk/lemonade/pull/2857
* ci: run .exe/.deb inference suites in the merge queue by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/2950
* ci: isolate llama.cpp validation cleanup on self-hosted runners by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/2915
* Reclaim routing helpers when a router collection's policy changes by `SlawomirNowaczyk` in https://github.com/lemonade-sdk/lemonade/pull/2795
* [backends] Add support for image generation through TheNoise by `bitgamma` in https://github.com/lemonade-sdk/lemonade/pull/2927
* fix(ci): refactor cleanup  and improve logging llama.cpp validation by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/2992
* fix(server): frame backend errors as SSE events on the streaming path by `Yigtwxx` in https://github.com/lemonade-sdk/lemonade/pull/2975
* ci: cut the longest test jobs via parameter tuning by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/2953
* docs: add new engine logos to homepage engine ticker by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/2920
* Show all local model versions in one folder by `anditherobot` in https://github.com/lemonade-sdk/lemonade/pull/2107
* test: run the committed model-type classifier test in CI by `Yigtwxx` in https://github.com/lemonade-sdk/lemonade/pull/2976
* fix(server): return the router error status on rerank, slots and tokenize by `Yigtwxx` in https://github.com/lemonade-sdk/lemonade/pull/2974
* ci(test): speed up server job suite by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3007
* Add complete vision benchmark support by `ckuethe` in https://github.com/lemonade-sdk/lemonade/pull/2869
* ci: cut installer build time on the PR/merge-queue critical path by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/2989
* fix(server): report backend errors on the Anthropic messages bridge by `Yigtwxx` in https://github.com/lemonade-sdk/lemonade/pull/3006
* docs: add Muse Glimmer 30B blog post by `jeremyfowers` in https://github.com/lemonade-sdk/lemonade/pull/3033
* fix: add GITHUB_TOKEN / GH_TOKEN support to GitHub API requests by `blackdeathdrow` in https://github.com/lemonade-sdk/lemonade/pull/2995
* ci: fail fast in merge queue matrices by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3014
* feat(rocm): enable AMD Instinct MI100 (gfx908) and MI200 (gfx90a) in llama.cpp ROCm by `kenvandine` in https://github.com/lemonade-sdk/lemonade/pull/2092
* feat(router): report estimated cost on collection.router decisions by `Bekhouche` in https://github.com/lemonade-sdk/lemonade/pull/2763
* Allow explicit system llama.cpp backend by `popey` in https://github.com/lemonade-sdk/lemonade/pull/3016
* feat(server): add model alias system with /v1/models listing and /internal/aliases endpoints by `abn` in https://github.com/lemonade-sdk/lemonade/pull/2818
* fix(server): sum sibling shards when computing on-disk GGUF size by `blackdeathdrow` in https://github.com/lemonade-sdk/lemonade/pull/2973
* fix(tts): stop dropping response_format on the streaming path by `ZMXJJ` in https://github.com/lemonade-sdk/lemonade/pull/3029
* fix: honor custom backend binary environment variables by `hogeheer499-commits` in https://github.com/lemonade-sdk/lemonade/pull/3004
* Update llama.cpp to b10360 by `github-actions`[bot] in https://github.com/lemonade-sdk/lemonade/pull/3053
* Add support for a default model source by `superm1` in https://github.com/lemonade-sdk/lemonade/pull/3036
* feat(llamacpp): auto-detect draft GGUF companions by `fl0rianr` in https://github.com/lemonade-sdk/lemonade/pull/3051
* Bump project version from 11.5.2 to 11.6.0 by `ramkrishna2910` in https://github.com/lemonade-sdk/lemonade/pull/3085
* feat(models): add Muse Glimmer 30B to the model catalog by `ramkrishna2910` in https://github.com/lemonade-sdk/lemonade/pull/3090
* Update llama.cpp to b10375 by `github-actions`[bot] in https://github.com/lemonade-sdk/lemonade/pull/3097

</details>

## New Contributors
* `duggiefresh` made their first contribution in https://github.com/lemonade-sdk/lemonade/pull/2827
* `sreeram-11` made their first contribution in https://github.com/lemonade-sdk/lemonade/pull/2924
* `Yigtwxx` made their first contribution in https://github.com/lemonade-sdk/lemonade/pull/2975
* `Bekhouche` made their first contribution in https://github.com/lemonade-sdk/lemonade/pull/2763
* `popey` made their first contribution in https://github.com/lemonade-sdk/lemonade/pull/3016
* `hogeheer499-commits` made their first contribution in https://github.com/lemonade-sdk/lemonade/pull/3004

**Full Changelog**: https://github.com/lemonade-sdk/lemonade/compare/v11.5.2...v11.6.0

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
