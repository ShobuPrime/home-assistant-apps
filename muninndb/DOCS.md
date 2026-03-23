# MuninnDB Documentation

## Overview

MuninnDB is a cognitive database that stores engrams — memory traces with built-in decay, association learning, and confidence scoring. This app runs MuninnDB as a Home Assistant service, providing persistent cognitive memory accessible via multiple protocols.

---

## Configuration

### Option: `log_level`

The `log_level` option controls the level of log output by the app:
- `trace`: Show every detail
- `debug`: Shows detailed debug information
- `info`: Normal (usually) interesting events (default)
- `warning`: Exceptional occurrences that are not errors
- `error`: Runtime errors
- `fatal`: Critical errors

---

### Option: `admin_password`

Sets the admin password for the MuninnDB web UI. On first startup, MuninnDB creates a default `root` / `password` account. When this option is set, the app automatically changes the admin password via the REST API after MuninnDB starts. The password must be at least 8 characters.

Leave empty to keep the default credentials. **Strongly recommended to set this** before exposing MuninnDB to the network.

The password change is tracked in `/data/muninndb/.admin_pass_set` — if you change the option value, the app will update the password on the next restart.

---

### Option: `default_vault`

Name of a vault to automatically create on first startup. Default: `homeassistant`. The vault is created as public (no API key required), making it immediately available to AI tools via MCP without additional authentication.

Set to empty to skip vault creation. Additional vaults can be created via the Web UI or REST API.

---

### Backups

MuninnDB supports two layers of backup:

- **Shutdown backups** (`backup_on_shutdown`, default: `true`) — Before the app stops (for updates, restarts, or HA backups), a native MuninnDB point-in-time backup is triggered via the REST API. This creates a verified Pebble checkpoint plus WAL and auth_secret copies. Stored in `/data/muninndb/backups/shutdown-YYYYMMDD-HHMMSS/`. The last 3 shutdown backups are retained automatically.

- **Automated periodic backups** (`backup_interval`) — MuninnDB's built-in backup system runs on a schedule (e.g., `6h`, `30m`). Set `backup_retain` to control how many are kept (default: `5`). Stored in `/data/muninndb/backups/`.

Both backup types are stored within `/data/muninndb/` and are included when Home Assistant creates an app backup, giving you both a native database-consistent snapshot and HA's full app state.

---

### SSL / TLS

To enable HTTPS on all MuninnDB ports:
- `ssl_certfile` — Certificate filename in `/ssl/` (e.g., `fullchain.pem`)
- `ssl_keyfile` — Private key filename in `/ssl/` (e.g., `privkey.pem`)

Both must be set for TLS to activate. When enabled, all ports (REST, Web UI, gRPC, MCP) serve over TLS. If the files are not found at the specified paths, the app falls back to plain HTTP with a log error.

---

### Option: `mem_limit_gb`

Constrains MuninnDB's memory usage. Set to `0` (default) for unlimited. Useful on resource-constrained systems.

---

### Option: `local_embed`

Controls the bundled ONNX Runtime embedder. When enabled (default), MuninnDB can generate embeddings locally without external API calls. Disable if you exclusively use an external provider.

---

### Embedding and Enrichment Providers

All optional, listed alphabetically:

- `anthropic_key` — Anthropic API key (`sk-ant-...`) for LLM enrichment. When configured with `enrich_url`, MuninnDB uses Claude to retroactively enrich existing memories in the background.
- `cohere_key` — Cohere embeddings
- `enrich_url` — LLM enrichment endpoint URL (e.g., `anthropic://claude-haiku-4-5-20251001`). Requires a provider key.
- `google_key` — Google (Gemini) embeddings
- `jina_key` — Jina AI embeddings
- `mistral_key` — Mistral AI embeddings
- `ollama_url` — Ollama instance URL for embeddings (e.g., `http://homeassistant.local:11434`)
- `openai_key` — OpenAI API key for cloud-based embeddings
- `openai_url` — Optional OpenAI-compatible endpoint override (e.g., `http://localhost:8080/v1`). Only takes effect when `openai_key` is also set.
- `voyage_key` — Voyage AI embeddings

---

## Access Methods

1. **Via Sidebar**: Click the brain icon in Home Assistant (uses ingress)
2. **Direct HTTP**: `http://[your-ip]:8476`
3. **REST API**: `http://[your-ip]:8475`
4. **gRPC**: `[your-ip]:8477`
5. **MBP Protocol**: `[your-ip]:8474` (lowest latency, for production agents)
6. **MCP**: `[your-ip]:8750` (for AI tool integration)

---

## Port Information

| Port | Protocol | Use Case |
|------|----------|----------|
| 8474 | MBP | Production agents — lowest latency (<10ms ACK) |
| 8475 | REST | HTTP/JSON — testing, integration, health checks |
| 8476 | Web UI | Dashboard with decay charts and relationship graphs |
| 8477 | gRPC | Polyglot team support |
| 8750 | MCP | AI tool integration (Claude, Cursor, VS Code, etc.) |

---

## Data Persistence

All data is stored in `/data/muninndb` and included in Home Assistant backups.

MuninnDB stores:
- Engram data (memory traces with concepts, content, confidence, and relevance scores)
- Vault definitions and configurations
- Association weights (Hebbian learning state)
- Trigger subscriptions

---

## Core Concepts

### Engrams

The fundamental storage unit. Each engram contains:
- **Concept**: The topic or category
- **Content**: The actual memory data
- **Confidence**: Bayesian posterior (0.0–1.0) tracking reliability
- **Relevance**: Temporal priority score computed at query time

### Vaults

Namespaces for engrams. Typically one vault per AI agent or user. Use vaults to isolate memory contexts.

### ACTIVATE

The primary query mechanism. Accepts a context string and returns the N most cognitively relevant engrams, ranked by a combination of recency, frequency, confidence, and semantic similarity.

---

## First Time Setup

1. Set `admin_password` in the app configuration (at least 8 characters)
2. Optionally change `default_vault` (defaults to `homeassistant`)
3. Optionally configure embedding providers for semantic search
4. Start the app — it will automatically set your password and create the vault
5. Open the Web UI and log in with `root` and your configured password
6. Connect AI tools via the MCP endpoint at port 8750

---

## Security Considerations

- **Default Credentials**: The default login is `root`/`password` — change this immediately
- **AppArmor**: Custom profile restricts app permissions appropriately
- **API Keys**: Embedding provider keys are stored as password fields and not displayed in the UI
- **Network Access**: All ports are exposed on the host — consider firewall rules if needed

---

## Troubleshooting

### MuninnDB Not Starting

**Symptoms:**
- App fails to start
- Logs show binary not found or permission errors

**Solution:**
1. Check app logs for specific error messages
2. Verify architecture compatibility (amd64/aarch64 only)
3. Try reinstalling the app

---

### Web UI Not Accessible

**Symptoms:**
- Cannot reach port 8476
- Ingress shows blank page

**Solution:**
1. Check that the app is running (green status)
2. Try direct access via `http://[your-ip]:8476`
3. Check app logs for binding errors

---

### Memory Usage High

**Symptoms:**
- System running slow
- MuninnDB consuming excessive RAM

**Solution:**
1. Set `mem_limit_gb` to constrain memory usage
2. Disable `local_embed` if not using local embeddings
3. Monitor engram count and prune unused vaults

---

## Updating

The app automatically tracks releases. Updates appear in the Home Assistant UI when available.

---

## External Resources

- [MuninnDB Documentation](https://muninndb.com/docs)
- [MuninnDB GitHub](https://github.com/scrypster/muninndb)
- [MuninnDB Getting Started](https://muninndb.com/getting-started)
