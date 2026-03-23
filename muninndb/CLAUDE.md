# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant App for MuninnDB that provides a cognitive database with memory primitives (Ebbinghaus decay, Hebbian learning, Bayesian confidence, semantic triggers) through the Home Assistant interface. The app uses Home Assistant's S6-overlay init system and follows standard HA app conventions.

## Essential Commands

### Building and Testing
```bash
# Build the app locally (auto-detects architecture)
./build.sh

# Test the app locally
docker run --rm -it -p 8474:8474 -p 8475:8475 -p 8476:8476 -p 8477:8477 -p 8750:8750 local/{arch}-addon-local_muninndb:{version}
```

### Version Management
```bash
# Check for updates (from repo root)
.github/scripts/update-muninndb.sh  # with CHECK_ONLY=true
```

## MuninnDB Version Scheme

MuninnDB is currently in alpha. Versions follow the pattern `X.Y.Z-alpha` (e.g., `0.4.1-alpha`). Some releases include patch suffixes like `0.3.14-alpha-1`. The project uses a single release stream — no separate LTS/STS tracks.

**Important**: The update script prefers stable releases (non-prerelease) and only falls back to pre-releases (alpha/beta) when no stable release exists. GitHub's `/releases/latest` endpoint skips pre-releases entirely, so the script uses `/releases` to fetch all releases.

## Architecture and Key Components

### Directory Structure
- **`/rootfs/etc/cont-init.d/`**: S6 initialization scripts that run on container start
- **`/rootfs/etc/services.d/muninndb/`**: Service definition with `run` script and `finish` handler

### Critical Files
- **`config.yaml`**: App configuration (version, ports, ingress, options schema)
- **`build.yaml`**: Build configuration with base images per architecture
- **`Dockerfile`**: Downloads MuninnDB binary from GitHub releases
- **`apparmor.txt`**: Security profile (no Docker socket access needed)

### Architecture Support
- `amd64` (x86_64)
- `aarch64` (arm64)

### Port Configuration
- **8474**: MBP binary protocol (lowest latency)
- **8475**: REST API (HTTP/JSON)
- **8476**: Web UI dashboard (ingress port)
- **8477**: gRPC API
- **8750**: MCP (Model Context Protocol for AI tools)

## Development Guidelines

### S6-Overlay Integration
- Use Bashio library for all configuration reading and logging
- Service scripts must be executable and use proper S6 conventions
- Exit codes: 0 for success, non-zero triggers restart with backoff
- No Docker socket access required — MuninnDB is a standalone database

### Configuration Handling
- Read options using `bashio::config` functions
- Embedding provider API keys are password fields
- Data is stored in `/data/muninndb`
- `MUNINNDB_DATA` env var points to the data directory

### Environment Variables
MuninnDB accepts these environment variables:
- `MUNINNDB_DATA`: Data directory path
- `MUNINN_LOCAL_EMBED`: Enable/disable bundled embedder (0/1)
- `MUNINN_OLLAMA_URL`: Ollama service URL (e.g., `ollama://localhost:11434/nomic-embed-text`)
- `MUNINN_OPENAI_KEY`: OpenAI API key
- `MUNINN_OPENAI_URL`: Optional OpenAI-compatible endpoint override
- `MUNINN_ANTHROPIC_KEY`: Anthropic API key for LLM enrichment
- `MUNINN_ENRICH_URL`: LLM enrichment URL (e.g., `anthropic://claude-haiku-4-5-20251001`)
- `MUNINN_MEM_LIMIT_GB`: Memory limit in GB
- `MUNINN_VOYAGE_KEY`, `MUNINN_COHERE_KEY`, `MUNINN_GOOGLE_KEY`, `MUNINN_JINA_KEY`, `MUNINN_MISTRAL_KEY`: Embedding provider keys

### Binary Download Pattern
The binary is downloaded directly from GitHub releases:
- URL: `https://github.com/scrypster/muninndb/releases/download/v{VERSION}/muninn-linux-{ARCH}`
- ARCH mapping: `aarch64` -> `arm64`, `x86_64` -> `amd64`
- The binary is a raw executable (not a tarball)

### Version Updates
When updating version:
1. Update `ARG MUNINNDB_VERSION` in Dockerfile
2. Update `version` in config.yaml
3. Update version in build.yaml args
4. Test on at least one architecture before committing

### Testing Checklist
- Build completes successfully
- Service starts without errors
- Web UI accessible on port 8476
- REST API responds on port 8475
- Ingress access works through Home Assistant sidebar
- Configuration changes apply correctly (embedding providers, memory limits)
- Data persists across restarts in `/data/muninndb`
- Update script correctly identifies latest version (handles alpha pre-releases)

## Important Notes

- **Never commit changes** to version numbers without testing
- **Ingress** integration requires WebSocket support (ingress_stream: true)
- **AppArmor profile** is critical for security - modifications require careful testing
- **Default credentials** are `root`/`password` — users must change on first login
- **Pre-release handling**: The update script uses `/releases` (not `/releases/latest`) because MuninnDB is in alpha
- **glibc dependency**: MuninnDB is a CGO binary linked against glibc (ONNX runtime for local embedder). Alpine uses musl, so `gcompat` and `libstdc++` are required in the Dockerfile. Without them the binary fails with `cannot execute: required file not found`.

## Common Issues and Troubleshooting

### Issue: Binary Download Fails

**Symptoms:**
- Dockerfile build fails at curl step
- 404 error from GitHub

**Cause:** Version tag format mismatch (tags include `v` prefix and `-alpha` suffix)

**Solution:**
1. Verify the tag exists: `gh api repos/scrypster/muninndb/releases | jq '.[0].tag_name'`
2. Ensure the Dockerfile uses the correct URL pattern with `v` prefix

### Issue: MuninnDB Not Starting

**Symptoms:**
- App starts but service crashes
- Logs show "permission denied" or "exec format error"

**Cause:** Architecture mismatch or corrupted binary

**Solution:**
1. Check the build architecture matches target
2. Verify binary downloaded correctly (check file size)
3. Ensure `chmod +x` was applied
