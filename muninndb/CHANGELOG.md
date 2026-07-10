# Changelog

## 0.8.0

_2026-07-10_

## What's Changed
* Release v0.5.1 by `scrypster` in https://github.com/scrypster/muninndb/pull/434
* Release v0.6.0 by `scrypster` in https://github.com/scrypster/muninndb/pull/441
* chore: merge develop into main for v0.6.2 release by `scrypster` in https://github.com/scrypster/muninndb/pull/497
* chore: merge develop into main for v0.6.3 release by `scrypster` in https://github.com/scrypster/muninndb/pull/513
* fix(hnsw): rebuild graph from vectors when the restored structure is disconnected by `johanneshauer` in https://github.com/scrypster/muninndb/pull/545
* fix(storage): dedup entity scan by normalized identity, not raw casing by `johanneshauer` in https://github.com/scrypster/muninndb/pull/550
* feat(cli): add `vault plasticity` command to get/set per-vault plasticity (#551) by `timharsch` in https://github.com/scrypster/muninndb/pull/552
* feat(grpc): ListVaults RPC by `scrypster` in https://github.com/scrypster/muninndb/pull/562
* feat(grpc): BatchForget RPC by `scrypster` in https://

---


## 0.7.0

_2026-06-13_

## What's Changed

### Cluster overhaul — HA is now production-ready

The Cortex/Lobe replication layer existed in previous releases but was not reliably functional in real multi-node deployments. Every known correctness issue has been addressed and Docker-validated end-to-end.

**Automatic failover** — when the Cortex goes down, Lobes detect SDOWN via gossip, accumulate votes, and the first node with quorum wins a jittered Raft-style election (#532)

**Returning-primary deference** — a restarted former Cortex probes the cluster before asserting leadership; if a failover leader is in place it defers, receives a snapshot, and follows — no split-brain, no data loss (#537)

**PeerHello discovery mesh** — nodes with no join relationship dial configured seeds and exchange authenticated frames, feeding MSP liveness and elections (#530)

**Equal-epoch tie-break** — two primaries discovering each other at the same epoch converge to a single leader via node-id ordering (#530)

**Per

---


## 0.6.3

_2026-06-12_

## What's Changed
* fix(activation): mean-pool multi-phrase query embedding (#498) by `scrypster` in https://github.com/scrypster/muninndb/pull/504
* fix(entity): prevent merge_entity case-variant data loss (#503) by `scrypster` in https://github.com/scrypster/muninndb/pull/505
* fix(mcp): normalize+coerce entity types on all user-facing write paths (#501) by `scrypster` in https://github.com/scrypster/muninndb/pull/510
* fix(hnsw): don't cache failed loads; log load outcomes; scope iterator to vault (#499) by `scrypster` in https://github.com/scrypster/muninndb/pull/506
* fix(sse): deliver trigger push events to SDK clients (#437) by `scrypster` in https://github.com/scrypster/muninndb/pull/507
* fix(plugins): correct local embedder label to bge-small + surface enrich init errors (#455, #453) by `scrypster` in https://github.com/scrypster/muninndb/pull/508
* fix(engine,mcp): set inline-enrichment digest flags + clean recall serialization (#500, #502) by `scrypster` in https://github.com/scry

---


## 0.6.2

_2026-06-11_

## What's Changed
* fix(cli): auto-detect TLS in muninn status/start (#442) by `johanneshauer` in https://github.com/scrypster/muninndb/pull/444
* style: gofmt-align literals in repl_client_test.go by `johanneshauer` in https://github.com/scrypster/muninndb/pull/445
* chore: polish isLoopbackURL and isTLSCertError by `scrypster` in https://github.com/scrypster/muninndb/pull/446
* chore(consolidation): surface dedup metadata-update errors in report by `scrypster` in https://github.com/scrypster/muninndb/pull/451
* Add Gemini 2.5 Flash enrichment option by `dpearson2699` in https://github.com/scrypster/muninndb/pull/450
* chore(ui): promote gemini-2.5-flash as default Google enrichment model by `scrypster` in https://github.com/scrypster/muninndb/pull/452
* consolidation: phase-2 dedup absorbs AccessCount of merged duplicates into the representative by `schurabot` in https://github.com/scrypster/muninndb/pull/447
* fix(cluster): defer OnLobeJoined callback until JoinResponse + Snapshot complete

---


> _Maintenance (2026-06-10):_ added hassio_role: manager so bashio can read the app config + Supervisor API on base 20.2.0 (fixes "Unable to access the API, forbidden"); migrated bashio::addon.* to bashio::app.*. Rebuild the app to apply the new role.

## 0.6.1

_2026-05-27_

## Bug Fixes

- **fix(cluster)** — defer \`OnLobeJoined\` callback until \`JoinResponse\` + snapshot are fully on the wire; prevents \`NetworkStreamer\` from racing the handshake and corrupting the lobe-side parser (#449, #448 Bug 1)
- **fix(cli)** — auto-detect TLS in \`muninn status\` / \`muninn start\` health probes (#444)

## Improvements

- **feat(consolidation)** — representative node absorbs \`AccessCount\` of merged duplicates during dedup (#447)
- **feat(enrichment)** — Gemini 2.5 Flash added as a Google enrichment option; promoted to default Google model (#450, #452)
- **chore(consolidation)** — dedup metadata-update errors now surfaced in consolidation report (#451)
- **chore** — polish \`isLoopbackURL\` and \`isTLSCertError\` helpers (#446)
- **style** — gofmt-align literals in \`repl_client_test.go\` (#445)

---


## 0.6.0

_2026-05-21_
## New Features

- **Audit logging** — structured audit trail with file, stdout, syslog, and webhook sinks; CLI `audit tail/export/stats` commands (#418)
- **Retrieval annotations** — staleness, conflict, and trust metadata on recall responses (#388)
- **Per-engram trust/taint labels** (#387)
- **Cursor-based pagination** for enrichment candidates
- **MCP initialize instructions** response

## Bug Fixes

- `fix(fts)` — auto-restart worker goroutines after panic; field byte in posting key prevents multi-field overwrite; IDF cache scoped per vault (#430)
- `fix(storage)` — clear last-access (0x22), archived associations (0x25), and dream state (0x27) prefixes on vault delete (#438)
- `fix(storage)` — vault deletion now removes all entity graph data (0x20–0x24, 0x26) and prunes orphaned global entity records (#436, #435)
- `fix(cli)` — `muninn status` and `muninn start` health probes now honour `MUNINNDB_{ADMIN,MCP,UI}_URL` for TLS deployments (#440, #439)
- `fix(engine)` �

---


## 0.5.1

_2026-05-07_
## Bug Fixes

- **fix(fts):** Auto-restart FTS worker goroutines after panic — worker goroutines that panicked were never replaced, eventually making all new writes unsearchable until server restart (#430)
- **fix(fts):** Include field byte in BM25 posting key — terms appearing in multiple fields (e.g. concept + content) had all but the last field's contribution silently overwritten (#430)
- **fix(fts):** Scope IDF cache by (vault, term) — the IDF cache was keyed by term only, causing incorrect BM25 scores in multi-vault setups (#430)

---


## 0.5.0

_2026-04-28_
## What's New

### feat: per-engram trust/taint labels (#387)
- `TrustLevel` enum (`verified`, `inferred`, `external`, `untrusted`) stored at ERF byte offset 71 — zero-migration, backward-compatible with all existing records
- All writes auto-stamp `TrustInferred`; trust is visible in all `muninn_read` and `muninn_recall` responses
- New `muninn_trust` MCP tool for post-write trust mutation
- New `ExcludeUntrusted` per-vault plasticity config to hard-filter untrusted engrams from ACTIVATE results

### feat: enrichment candidates cursor pagination (#362)
- `muninn_get_enrichment_candidates` now supports cursor-based pagination via `after_cursor` / `next_cursor` — large vaults no longer miss candidates

## Bug Fixes
- `fix(engine)`: return 400 for invalid inline association target IDs (#399)
- `fix(rest)`: return 400 instead of 500 for invalid engram IDs in `/api/link` (#395)
- `fix(enrich)`: prevent infinite retry loops that deadlock the circuit breaker (#390)
- `fix(trigger)`: guar

---


## 0.4.10

_2026-04-03_
## What's new

### Added
- **Dashboard activity panel** — selectable timeframe presets (7d–180d), end-date picker, dynamic x-axis tick grouping, raw data table toggle with copy-to-clipboard. Full loading/error/empty-state feedback.
- **`GET /api/activity-counts`** — per-day engram creation counts for a vault. Accepts `days` (1–180, default 7) and optional `until` (YYYY-MM-DD). Backed by an efficient ULID key-header scan.

### Changed
- **Public vault auth** — unauthenticated requests to an open vault now run as `full` instead of `observe`. Public vaults are genuinely open; callers get `full` access unless they present an explicit `observe` key.
- **Web UI tab navigation** — unified bordered-tab style across Memories, Graph, and Settings, replacing the previous mix of underline/button/pill patterns.

### Fixed
- **ACT-R score saturation** — `bLevelCap` prevents base-level overflow in fresh vaults; two-pass normalization keeps all scores in [0, 1].
- **Archived engram leaka

---


## 0.4.9-alpha

_2026-03-31_
## What's Changed
- **fix(mcp):** order JSON Schema properties required-first in `tools/list` (#310)
  - Fixes Python MCP SDK clients crashing with `TypeError: non-default argument follows default argument`
  - Affects 17 tools — unblocks the Python client ecosystem

---


## 0.4.8-alpha

_2026-03-30_
## What's Changed
* feat(dream): memories accumulate but never consolidate -- add dream engine foundation by `5queezer` in https://github.com/scrypster/muninndb/pull/306
* feat(dream): dream engine foundation by `scrypster` in https://github.com/scrypster/muninndb/pull/307

## New Contributors
* `5queezer` made their first contribution in https://github.com/scrypster/muninndb/pull/306

**Full Changelog**: https://github.com/scrypster/muninndb/compare/v0.4.7-alpha...v0.4.8-alpha

---


## 0.4.7-alpha

_2026-03-28_
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


## 0.4.6-alpha

_2026-03-22_
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


## 0.4.4-alpha

_2026-03-17_
## What's Changed
* release: merge develop into main for v0.4.4-alpha by `scrypster` in https://github.com/scrypster/muninndb/pull/272


**Full Changelog**: https://github.com/scrypster/muninndb/compare/v0.4.3-alpha...v0.4.4-alpha

---


## 0.4.3-alpha

_2026-03-16_
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


## 0.4.2-alpha

_2026-03-15_
## What's Changed
* Release: develop → main by @scrypster in https://github.com/scrypster/muninndb/pull/252


**Full Changelog**: https://github.com/scrypster/muninndb/compare/v0.4.1-alpha...v0.4.2-alpha

---


## 0.4.1-alpha

_2026-03-14_
### Initial release

- Initial Home Assistant add-on for MuninnDB
- Cognitive database with Ebbinghaus decay, Hebbian learning, and Bayesian confidence
- Web UI dashboard with decay charts, relationship graphs, and activation logs
- REST, gRPC, MBP, and MCP protocol support
- Configurable embedding providers (local, Ollama, OpenAI, Voyage, Cohere, Gemini, Jina, Mistral)
- Ingress support for sidebar integration
- Automatic version update checks
