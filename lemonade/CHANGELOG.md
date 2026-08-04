# Changelog

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
