# Lemonade Documentation

## Overview

Lemonade serves local language models over OpenAI-, Ollama- and
Anthropic-compatible HTTP APIs. This app packages the upstream `lemond` server
so Home Assistant can use a private, on-device LLM as a conversation agent
without any cloud service.

Inference runs through llama.cpp on the CPU. The app is designed around small
models — the default is Liquid AI's LFM2.5-230M — so it stays usable on
Home Assistant Yellow / Green class hardware.

Upstream Lemonade is a general-purpose AI server; this app is set up to be a
Home Assistant conversation agent out of the box. The defaults — CPU inference,
a small preloaded model, a context size matching what Home Assistant asks for —
are chosen for that. It still works as a general server for any OpenAI- or
Ollama-compatible client.

## Configuration

### Option: `log_level`

The `log_level` option controls the level of log output by the app:
- `trace`: Show every detail
- `debug`: Shows detailed debug information
- `info`: Normal (usually) interesting events (default)
- `warning`: Exceptional occurrences that are not errors
- `error`: Runtime errors
- `fatal`: Critical errors

Lemonade's own levels are coarser than Home Assistant's, so `trace` and `debug`
both map to Lemonade's `debug`, and `error`/`fatal` both map to `error`.

### Option: `llamacpp_backend`

Selects which llama.cpp build Lemonade downloads and runs.

| Value    | Use when |
|----------|----------|
| `cpu`    | Default. Correct for Yellow, Green, Raspberry Pi and any machine without a supported GPU. |
| `vulkan` | x86 host with a Vulkan-capable GPU passed into the container. |
| `auto`   | Let Lemonade probe the hardware and choose. |

Changing this downloads a different llama.cpp build on the next start.

### Option: `model_name`

Short name the model is registered under. Lemonade namespaces
manually-registered models, so `LFM2.5-230M` becomes:

- `user.LFM2.5-230M` over the OpenAI API and in the Web UI
- `LFM2.5-230M:latest` over the Ollama API

Leave blank to skip automatic model setup entirely and manage models from the
Web UI.

### Option: `model_checkpoint`

Hugging Face repository and GGUF filename, written `org/repo:filename.gguf`.

Useful values:

| Checkpoint | Size | Notes |
|------------|------|-------|
| `LiquidAI/LFM2.5-230M-GGUF:LFM2.5-230M-Q4_K_M.gguf` | ~150 MB | Default. Fastest. |
| `LiquidAI/LFM2.5-230M-GGUF:LFM2.5-230M-Q8_0.gguf` | ~250 MB | Better quality, roughly half the speed. |
| `LiquidAI/LFM2.5-1.2B-Instruct-GGUF:LFM2.5-1.2B-Instruct-Q4_K_M.gguf` | ~730 MB | Noticeably more capable; still fine on 8 GB+. |

Gated or private repositories are not supported by this option.

### Options: `embedding_model_name` / `embedding_model_checkpoint`

Blank by default. Setting a name makes Lemonade also serve a text-embedding
model alongside the chat model — used by the MuninnDB app to give the
assistant memory that survives across sessions.

| Option | Default |
|--------|---------|
| `embedding_model_name` | *(blank — no embedding model)* |
| `embedding_model_checkpoint` | `LiquidAI/LFM2.5-Embedding-350M-GGUF:LFM2.5-Embedding-350M-Q8_0.gguf` (~360 MB) |

The default is the embedding counterpart of the LFM2.5 chat model, so both
sides of your setup come from the same family.

The embedding model is loaded and **pinned** on start, so it stays in memory
and a memory lookup never waits for a model to load. It does not compete with
the chat model: `max_loaded_models` applies per model *type*, so a chat model
and an embedding model stay resident together.

To use it from MuninnDB, set that app's `ollama_url` to:

```
ollama://<this-app's-hostname>:13305/<embedding_model_name>
```

The hostname is on this app's Info tab, and the app logs the exact line to use
after it finishes downloading. Note the model name is part of the URL — leaving
it out will not work.

> Pick your embedding model **before** storing memories. Vectors from different
> models are not comparable, so changing it later means re-embedding the whole
> vault.

### Option: `extra_models_dir`

A second directory Lemonade scans for GGUF files, on top of the models it
downloads itself. Defaults to `/share/lemonade_models`, which the app creates
on start.

Because `/share` is a Home Assistant shared folder, you can put models there
over Samba, the File editor add-on, or `scp` — no CLI and no Hugging Face
round trip. The folder also survives app updates and reinstalls, since it lives
outside the app's own `/data`.

How discovery works:

- The folder is scanned **recursively** for `.gguf` files, and new files are
  picked up while the app runs — no restart needed.
- **Every file keeps its own name.** A model is named after its file, minus the
  `.gguf` extension — put as many in as you like and each is listed separately.
- **The top-level folder sets the model type.** Files in `embeddings/` serve
  embeddings, files in `reranking/` rerank, and files in `chat/` or at the top
  level chat. The three folders are created for you; the names must match
  exactly (`Embedding/` is just an ordinary folder).
- **Any other folder is one model**, named after the folder. This is how
  multi-shard models (`*-00001-of-00006.gguf`) and multimodal models work. If
  any file in the folder has `mmproj` in its name it is used as the projector
  and the model is labelled `vision`. A folder inside `embeddings/` or
  `reranking/` takes that type.
- These models are **read-only to Lemonade** — it will refuse to delete them
  and tell you the path to remove by hand instead.

Given this folder:

```
/share/lemonade_models/
├── alpha-7b-q4.gguf
├── beta-tiny-instruct.gguf
├── gamma-vision-Q8_0.gguf
├── delta-sharded/
│   └── model-00001-of-00001.gguf
└── embeddings/
    └── nomic-embed-text-v2.gguf
```

you get five independently selectable models:

| File or folder | Model name | Name in the Ollama integration | Type |
|----------------|------------|-------------------------------|------|
| `alpha-7b-q4.gguf` | `alpha-7b-q4` | `alpha-7b-q4:latest` | chat |
| `beta-tiny-instruct.gguf` | `beta-tiny-instruct` | `beta-tiny-instruct:latest` | chat |
| `gamma-vision-Q8_0.gguf` | `gamma-vision-Q8_0` | `gamma-vision-Q8_0:latest` | chat |
| `delta-sharded/` | `delta-sharded` | `delta-sharded:latest` | chat |
| `embeddings/nomic-embed-text-v2.gguf` | `nomic-embed-text-v2` | `nomic-embed-text-v2:latest` | embeddings |

Before Lemonade 11.9.0 an `embeddings/` folder was listed as one chat model
named `embeddings`. That name still works as an alias for the first file in the
folder, alphabetically, so existing automations keep running.

They sit alongside downloaded models such as `LFM2.5-230M`, so you can switch
between any of them from Home Assistant.

Set this to a different path to use another folder, or leave it blank to
disable the feature. If the directory cannot be created the app logs a warning
and carries on with extra models disabled.

### Option: `preload_model`

When on (default), the model is loaded into memory during startup. Turn off to
keep memory free while idle, at the cost of a slow first request.

### Option: `ctx_size`

How much conversation the model can consider at once, in tokens. Defaults to
8192, which matches what Home Assistant's Ollama integration asks for. `0` uses
the model's own default.

**This setting is what actually applies.** If you raise the context size inside
the Home Assistant integration, raise it here too — the value Home Assistant
sends is ignored, and if this one is lower, long conversations get silently cut
short.

### Option: `max_loaded_models`

Number of models Lemonade may keep resident simultaneously. Default 1.

### Option: `api_key`

When set, clients must send this key.

Whether you can use it depends on which Home Assistant integration you connect
with (see "Connecting Home Assistant" below): the **llama.cpp** integration has
an optional API key field, so a key works there. The **Ollama** integration has
no such field, so leave this blank if you connect that way.

Set it only when port 13305 is reachable beyond a trusted network.

### Option: `allowed_origins`

**Leave this blank unless the panel misbehaves in the specific way below.**

Lemonade only accepts requests from web addresses it recognises, so a random
site you visit can't reach it through your browser. It works out your addresses
automatically, including the one Home Assistant itself uses.

**When you need it:** the panel opens and looks fine, but nothing works —
"Failed to load model", or chat that never sends. That means you reached Home
Assistant at an address it doesn't know is its own. Most often that's Nabu Casa
remote access:

```yaml
allowed_origins: "https://abc123def.ui.nabu.casa"
```

The add-on log tells you the exact value to paste — look for `origins: refused`.

Addresses must match exactly, including `http`/`https` and any port number.
Separate several with commas. `*` accepts everything and turns the protection
off; only do that if you've also set an `api_key`.

### Option: `telemetry`

Disabled by default. Enables Lemonade's anonymous OTLP usage tracing when on.

---

## Tool calling (controlling devices)

Turning on "Assist" lets the model control your devices, and tool calling is how
that happens. It is supported and enabled — the model is registered as
tool-capable and Home Assistant configures against it without extra steps.

The default 230M model calls tools more reliably than its size suggests —
measured 80-100% of the time, and it picks the right action (on vs off) very
consistently. Two things decide whether it works for you:

**Always keep a system prompt.** With Home Assistant's normal Assist prompt it
calls tools reliably. With tools attached but *no* system prompt it refuses
around 60% of the time, claiming it has no such capability. Home Assistant
always sends one, so this mainly matters if you call the API yourself.

**Say the device's name.** The model does not reliably turn a casual phrase
into a registered device name. "Switch the Third Reality plug on" works; "turn
on the smart plug" makes it invent a name like "Smart Plug", which Home
Assistant then cannot match. Using the name shown in Home Assistant is the
single biggest thing you can do to make this reliable.

If you want it to handle vaguer phrasing, a larger model helps —
`LiquidAI/LFM2.5-1.2B-Instruct-GGUF:LFM2.5-1.2B-Instruct-Q4_K_M.gguf` is the
smallest worthwhile step up.

> If Assist replies "there are multiple devices called X", that is a Home
> Assistant naming clash, not the model. Two entities share a friendly name and
> Assist cannot choose between them — rename one in Home Assistant.

---

## Long-term memory (MuninnDB)

Lemonade does not remember anything between requests, and Home Assistant keeps
only a short rolling window per conversation (20 messages, expiring after an
hour). So by default your assistant starts every session blank.

Turning on `memory_enabled` changes that: each conversation is saved to a
MuninnDB vault, and the most relevant past exchanges are recalled and given to
the model when you ask something later.

### Setting it up

1. Install and start the **MuninnDB** app. It creates a `home_assistant` vault
   for exactly this purpose, open so no API key is needed.
2. On MuninnDB's **Info** tab, note its hostname.
3. In Lemonade set `memory_enabled` on and `memory_url` to
   `http://<that-hostname>:8475`.
4. Restart. The log states plainly whether MuninnDB was reachable.

### Options

| Option | Default | Meaning |
|--------|---------|---------|
| `memory_enabled` | `false` | Master switch. |
| `memory_url` | *(empty)* | MuninnDB REST endpoint, e.g. `http://local-muninndb:8475`. Blank keeps memory off. |
| `memory_vault` | `home_assistant` | Vault to read and write. |
| `memory_api_key` | *(empty)* | Only needed for a locked vault. |
| `memory_recall_count` | `3` | How many memories to fetch per turn. |
| `memory_recall_tokens` | `300` | Hard cap on injected context. |
| `memory_timeout_ms` | `1000` | Give up on recall after this and answer without it. |

### If MuninnDB isn't there

Memory never gets in the way of an answer. If MuninnDB is not installed,
unreachable, or slow to respond, the question is answered without memory and
the app keeps working normally. With memory switched off, nothing is added to
the request path at all.

Your existing Assist behaviour is unaffected either way — recalled notes are
added alongside Home Assistant's own instructions, not in place of them.

### Effect on speed

A memory lookup costs roughly 300-400 ms, and more when several conversations
overlap. That is slower than it looks because it makes a round trip: MuninnDB
has to turn your question into a vector using this app's own embedding model,
so the lookup comes back here before it can finish.

If voice replies feel sluggish, lower `memory_recall_count` to 1 or 2 first.
Lowering `memory_timeout_ms` also speeds up the worst case, but set it too low
and lookups get abandoned before they finish — memory then quietly stops
working while everything still appears healthy.

## Access Methods

1. **Via Sidebar**: Click Lemonade in Home Assistant (uses ingress)
2. **Direct HTTP**: `http://[your-ip]:13305`

## Port Information

- **13305**: Web UI plus the OpenAI, Ollama and Anthropic APIs, and the
  WebSocket used for streaming audio transcription. Everything is on this one
  port, which is why ingress works.

If you want Ollama clients that auto-discover `localhost:11434` to find
Lemonade, change the *host* port to 11434 in the app's Network settings.

## API Endpoints

| API | Base path | Example |
|-----|-----------|---------|
| OpenAI | `/api/v1` | `POST /api/v1/chat/completions` |
| Ollama | `/api` | `POST /api/chat`, `GET /api/tags` |
| Anthropic | `/api/v1` | `POST /api/v1/messages` |
| Health | `/live` | returns `{"status":"ok"}` |

Example request:

```bash
curl http://homeassistant.local:13305/api/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"user.LFM2.5-230M","messages":[{"role":"user","content":"Hello"}]}'
```

## Data Persistence

Your settings, registered models and downloaded model files are stored with the
app and included in Home Assistant backups. Because that includes the model
weights, backups are large — if you would rather keep them small, exclude this
app and let it re-download on restore.

Models you put in `/share/lemonade_models` are stored outside the app, so they
are not part of an app backup and they survive uninstalling and reinstalling.

## Configuration Precedence

The app's Configuration tab wins. Settings it manages — context size, backend,
log level, maximum loaded models, telemetry — are reapplied on every start, so
changing those in the Web UI will not stick. Everything else you set in the Web
UI, such as per-model options and installed backends, is preserved.

## Hardware Expectations

On a Raspberry Pi CM5 (Home Assistant Yellow), LFM2.5-230M at Q4_K_M generates
roughly 20-40 tokens per second on CPU, which is comfortable for short Assist
replies. Larger models scale down proportionally: a 1.2B model runs at roughly
a fifth of that speed.

Memory use is approximately the model size plus the context window — about
300 MB resident for the default configuration.

## Security Considerations

- **Protection Mode**: Can stay enabled. This app does not need the Docker
  socket or any elevated access.
- **AppArmor**: A custom profile restricts the app appropriately.
- **Network exposure**: Port 13305 is unauthenticated by default. It is
  intended for your local network. Set `api_key` if that is not acceptable.
- **Browser origins**: requests from unrecognised web addresses are refused,
  so a site you visit can't reach this app through your browser. Works with or
  without an `api_key`. See `allowed_origins` above.

## Troubleshooting

### The model never finishes downloading

**Symptoms:**
- Log stops after "Downloading '<name>' from ..."

**Solution:**
1. Check the app has internet access and that Hugging Face is reachable.
2. Verify the `model_checkpoint` string is `org/repo:filename.gguf` and that
   the file exists in that repository.
3. Restart the app — downloads resume from the Hugging Face cache.

### The model fails to load right after downloading

**Symptoms:**
- `error while loading shared libraries` in the log

**Solution:** This is a packaging bug, not something you can configure around.
Please open an issue with the log line.

### Home Assistant's Ollama integration cannot see the model

**Symptoms:**
- The model dropdown is empty when adding the integration

**Solution:**
1. Confirm `curl http://<ha-ip>:13305/api/tags` lists your model.
2. Make sure `api_key` is blank — the Ollama integration cannot send one.
3. Use the URL without a trailing path, e.g. `http://homeassistant.local:13305`.

### Responses are slow or truncated

**Solution:**
1. Lower `ctx_size` — a smaller context is faster and uses less memory.
2. Keep `preload_model` on so the model is not loaded per request.
3. Consider the Q4_K_M quantization if you switched to Q8_0.

## Updating

The app tracks upstream Lemonade releases automatically. Updates appear in the
Home Assistant UI when available.

## External Resources

- [Lemonade Documentation](https://lemonade-server.ai/docs/)
- [Lemonade GitHub](https://github.com/lemonade-sdk/lemonade)
- [LFM2.5-230M model card](https://huggingface.co/LiquidAI/LFM2.5-230M-GGUF)
- [Home Assistant Ollama integration](https://www.home-assistant.io/integrations/ollama/)
