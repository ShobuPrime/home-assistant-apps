# Changelog

## Version 0.4.9-alpha (2026-03-31)

## What's Changed
- **fix(mcp):** order JSON Schema properties required-first in `tools/list` (#310)
  - Fixes Python MCP SDK clients crashing with `TypeError: non-default argument follows default argument`
  - Affects 17 tools — unblocks the Python client ecosystem

---


## Version 0.4.8-alpha (2026-03-30)

## What's Changed
* feat(dream): memories accumulate but never consolidate -- add dream engine foundation by `5queezer` in https://github.com/scrypster/muninndb/pull/306
* feat(dream): dream engine foundation by `scrypster` in https://github.com/scrypster/muninndb/pull/307

## New Contributors
* `5queezer` made their first contribution in https://github.com/scrypster/muninndb/pull/306

**Full Changelog**: https://github.com/scrypster/muninndb/compare/v0.4.7-alpha...v0.4.8-alpha

---


## Version 0.4.7-alpha (2026-03-28)

## What's Changed
* fix(build): add -tags localassets and fix Docker publish trigger by `scrypster` in https://github.com/scrypster/muninndb/pull/292
* docs: proactive agent prompting guide (credit cmdillon, #293) by `scrypster` in https://github.com/scrypster/muninndb/pull/295
* fix(enrich): handle duplicate JSON output from local LLMs (llama3.2) by `scrypster` in https://github.com/scrypster/muninndb/pull/296
* fix(ui): map created_at to createdAt — fix "Created: unknown" for all memories by `scrypster` in https://github.com/scrypster/muninndb/pull/297
* fix(entity): normalize inline entity types in engine Write path by `scrypster` in https://github.com/scrypster/muninndb/pull/300
* feat(recall): hint on empty results + session-start guidance in muninn_guide by `scrypster` in https://github.com/scrypster/muninndb/pull/301
* docs(integrations): Traefik guide for Claude.com/ChatGPT cloud-hosted MCP by `scrypster` in https://github.com/scrypster/muninndb/pull/302
* feat(ui): add flow diagram

---


## Version 0.4.6-alpha (2026-03-22)

## What's Changed
* fix(plugin): apply MUNINN_OPENAI_URL to openai:// enrichment provider by `scrypster` in https://github.com/scrypster/muninndb/pull/278
* docs(plugins): clarify MUNINN_ENRICH_API_KEY vs MUNINN_OPENAI_KEY separation by `scrypster` in https://github.com/scrypster/muninndb/pull/280
* fix(cluster): retry lobe/observer join with exponential backoff (#281) by `scrypster` in https://github.com/scrypster/muninndb/pull/284
* fix(rest): return 400 for malformed engram IDs in URL paths (#282) by `scrypster` in https://github.com/scrypster/muninndb/pull/285
* fix(import): repair 4 bugs in vault import/reembed pipeline by `scrypster` in https://github.com/scrypster/muninndb/pull/288
* feat(enrich): add Google Gemini as enrichment provider by `scrypster` in https://github.com/scrypster/muninndb/pull/289
* release: merge develop into main by `scrypster` in https://github.com/scrypster/muninndb/pull/290


**Full Changelog**: https://github.com/scrypster/muninndb/compare/v0.4.5-alpha...v0.4

---


## Version 0.4.4-alpha (2026-03-17)

## What's Changed
* release: merge develop into main for v0.4.4-alpha by `scrypster` in https://github.com/scrypster/muninndb/pull/272


**Full Changelog**: https://github.com/scrypster/muninndb/compare/v0.4.3-alpha...v0.4.4-alpha

---


## Version 0.4.3-alpha (2026-03-16)

## What's Changed
* refactor(engine): harden API surface for Stage 2 embedding roadmap by `scrypster` in https://github.com/scrypster/muninndb/pull/240
* engine: seal Store() leaks and fix Filter.Value type mismatches by `scrypster` in https://github.com/scrypster/muninndb/pull/242
* embed: isolate ONNX/CGO behind localassets build tag (Stage 1) by `scrypster` in https://github.com/scrypster/muninndb/pull/243
* feat: Stage 3 — muninn.Open() embedded convenience layer by `scrypster` in https://github.com/scrypster/muninndb/pull/244
* feat(cli): add muninn exec one-shot subcommand (Stage 4) by `scrypster` in https://github.com/scrypster/muninndb/pull/245
* feat(sdks): Stage 6 — wire-format audit, bug fixes, and test suites by `scrypster` in https://github.com/scrypster/muninndb/pull/246
* fix(mcp): muninn_read returns numeric state string instead of human-readable label by `To3Knee` in https://github.com/scrypster/muninndb/pull/249
* fix(rest): statusRecorder does not implement http.Flusher

---


## Version 0.4.2-alpha (2026-03-15)

## What's Changed
* Release: develop → main by @scrypster in https://github.com/scrypster/muninndb/pull/252


**Full Changelog**: https://github.com/scrypster/muninndb/compare/v0.4.1-alpha...v0.4.2-alpha

---


## Version 0.4.1-alpha (2026-03-14)

### Initial release

- Initial Home Assistant add-on for MuninnDB
- Cognitive database with Ebbinghaus decay, Hebbian learning, and Bayesian confidence
- Web UI dashboard with decay charts, relationship graphs, and activation logs
- REST, gRPC, MBP, and MCP protocol support
- Configurable embedding providers (local, Ollama, OpenAI, Voyage, Cohere, Gemini, Jina, Mistral)
- Ingress support for sidebar integration
- Automatic version update checks
