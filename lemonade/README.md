# Lemonade App for Home Assistant

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]

Run a private, local LLM on your Home Assistant hardware and use it as a conversation agent.

## About

[Lemonade][lemonade] is a local AI server that serves language models over
OpenAI-, Ollama- and Anthropic-compatible APIs. Because it speaks both APIs
Home Assistant ships a client for, HA's built-in **llama.cpp** or **Ollama**
integration can use it directly — no custom integration, no cloud account, and
nothing leaves your network.

This app ships with [Liquid AI's LFM2.5-230M][lfm] configured by default: a
230-million-parameter model that is small and fast enough to run comfortably on
a Home Assistant Yellow or Green (Raspberry Pi CM5), while still handling
"turn on the kitchen lights"-style requests.

> **Built for Home Assistant.** Upstream Lemonade is a general-purpose AI
> server; this app comes set up to be a Home Assistant conversation agent, so
> the defaults suit that job. It still works as a general server for any
> OpenAI- or Ollama-compatible client.

## Features

- OpenAI-, Ollama- and Anthropic-compatible APIs on a single port
- Works with Home Assistant's built-in Ollama integration
- Downloads and loads a model for you on first start
- Web UI for chatting, browsing and managing models
- Runs entirely on CPU — no GPU or NPU required
- Advertised as tool-capable so Assist can offer it device-control actions
- Ingress support for seamless sidebar integration
- Models and settings live in `/data` and are included in backups

## Installation

1. Add this repository to your Home Assistant instance
2. Search for "Lemonade" in the app store
3. Click Install
4. Start the app — the first start downloads the inference backend and the
   default model (about 200 MB total), so give it a few minutes
5. Click "OPEN WEB UI" or use the sidebar to chat with the model

## Connecting Home Assistant to Lemonade

Lemonade speaks both APIs Home Assistant ships a client for, so there are two
routes. Neither needs a custom integration.

**Option 1 — llama.cpp integration (recommended).** It is built for exactly
this: a local OpenAI-compatible server, and it accepts an optional API key.

1. **Settings → Devices & Services → Add Integration → llama.cpp**
2. URL: `http://homeassistant.local:13305/v1` (the `/v1` suffix matters)
3. API key: leave empty, or match the app's `api_key` option if you set one
4. Pick your model, e.g. `user.LFM2.5-230M`

**Option 2 — Ollama integration.**

1. **Settings → Devices & Services → Add Integration → Ollama**
2. URL: `http://homeassistant.local:13305` (no suffix)
3. Pick **`LFM2.5-230M:latest`** from the model list

Then enable **Assist** on the integration to use it as a conversation agent.

Note the two APIs spell model names differently: a model configured here as
`LFM2.5-230M` is `user.LFM2.5-230M` over the OpenAI API and
`LFM2.5-230M:latest` over the Ollama API.

> Home Assistant's llama.cpp and Ollama integrations are **clients** — they
> connect to a server you run, they do not serve models themselves. That server
> is what this app provides.

> A 230M model is small. It is good at short, direct instructions and will not
> reason its way through complicated multi-step requests. If you have RAM to
> spare, `LiquidAI/LFM2.5-1.2B-Instruct-GGUF:LFM2.5-1.2B-Instruct-Q4_K_M.gguf`
> is a noticeably stronger drop-in replacement.

## Configuration

### Option: `log_level`

The `log_level` option controls the level of log output by the app:

- `trace`: Show every detail
- `debug`: Shows detailed debug information
- `info`: Normal (usually) interesting events (default)
- `warning`: Exceptional occurrences that are not errors
- `error`: Runtime errors
- `fatal`: Critical errors

### Option: `llamacpp_backend`

Which llama.cpp build to run models with. `cpu` (default) is correct on
Home Assistant Yellow, Green and Raspberry Pi hardware. Use `vulkan` only on an
x86 machine with a supported GPU, or `auto` to let Lemonade probe the hardware.

### Option: `model_name`

The short name the model is registered under. It becomes `user.<name>` in
Lemonade and `<name>:latest` over the Ollama API. Leave blank to skip automatic
model setup and add models yourself from the Web UI.

### Option: `model_checkpoint`

The Hugging Face repository and GGUF file to download, as
`org/repo:filename.gguf`. Defaults to LFM2.5-230M at Q4_K_M (~150 MB).

### Options: `embedding_model_name` / `embedding_model_checkpoint`

Blank by default. Set a name to have Lemonade also serve a text-embedding model
(default: LFM2.5-Embedding-350M, ~360 MB) — used by the MuninnDB app for
long-term memory. It is pinned in memory and does not compete with the chat
model. Point MuninnDB's `ollama_url` at
`ollama://<this-hostname>:13305/<name>`; the app logs the exact line to use.

### Option: `extra_models_dir`

A folder Lemonade also scans for GGUF files, defaulting to
`/share/lemonade_models` (created on start). Copy as many `.gguf` files in as
you like over Samba or the File editor and restart the app — each is listed
separately under its own filename minus the extension, so
`alpha-7b-q4.gguf` and `beta-tiny-instruct.gguf` become `alpha-7b-q4` and
`beta-tiny-instruct` (and `…:latest` in the Ollama integration's model list). A
subdirectory is treated as one model named after the folder, which is how
sharded and multimodal models work. Leave blank to disable.

### Option: `preload_model`

When on (default), the model is loaded into memory at startup so the first
request from Home Assistant answers immediately.

### Long-term memory options

Off by default. When `memory_enabled` is on and `memory_url` points at the
MuninnDB app, conversations are saved to a vault and relevant past exchanges are
recalled on later questions.

| Option | Default | Meaning |
|--------|---------|---------|
| `memory_enabled` | `false` | Master switch. When off, nothing is added to the request path at all. |
| `memory_url` | *(empty)* | MuninnDB REST endpoint, e.g. `http://local-muninndb:8475`. |
| `memory_vault` | `home_assistant` | Vault to read and write. |
| `memory_api_key` | *(empty)* | Only needed for a locked vault. |
| `memory_recall_count` | `3` | Memories fetched per turn. |
| `memory_recall_tokens` | `300` | Hard cap on injected context. |
| `memory_timeout_ms` | `400` | Give up on recall after this and answer anyway. |

If MuninnDB is not installed, unreachable or slow, chat keeps working and memory
is simply skipped. See DOCS.md for the full behaviour.

### Option: `ctx_size`

How much conversation the model can consider at once, in tokens. Defaults to
8192 to match Home Assistant. If you raise it in the integration, raise it here
too. Set to 0 to use the model's own default.

### Option: `max_loaded_models`

How many models may be resident at once. Leave at 1 on constrained hardware.

### Option: `api_key`

Optional key API clients must present. Leave blank for an open server on your
local network.

### Option: `telemetry`

Off by default. Turn on only to share anonymous usage tracing with the Lemonade
developers.

## Folder Access

This app has access to the following Home Assistant directories:

- `/ssl` - SSL certificates (read-only)
- `/data` - App persistent data (read/write)
- `/media` - Home Assistant media folder (read/write)
- `/share` - Home Assistant share folder (read/write)

## First Time Setup

Start the app and watch the log. On a first start you will see the llama.cpp
backend download, then the model download, then `Model 'user.<name>' loaded and
ready`. After that, add the Ollama integration as described above.

## Support

Got questions or found a bug? Please open an issue on the GitHub repository.

[aarch64-shield]: https://img.shields.io/badge/aarch64-yes-green.svg
[amd64-shield]: https://img.shields.io/badge/amd64-yes-green.svg
[lemonade]: https://lemonade-server.ai/
[lfm]: https://www.liquid.ai/blog/lfm2-5-230m

## Version

Currently running Lemonade 11.5.2
