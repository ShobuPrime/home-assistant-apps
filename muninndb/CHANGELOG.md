# Changelog

## 0.10.0

_2026-08-02_

## What's Changed
* chore(deps): bump golang.org/x/text to v0.39.0 (GO-2026-5970) by `scrypster` in https://github.com/scrypster/muninndb/pull/649
* fix(enrich): preserve transient provider failures by `dpearson2699` in https://github.com/scrypster/muninndb/pull/643
* feat(grpc,engine): AdjustConfidence RPC with contradiction signaling (#559) by `madeinoz67` in https://github.com/scrypster/muninndb/pull/625
* Close three documented gaps: upgrade checksums (#600), Windows CI model drift, and drift guards that never ran by `scrypster` in https://github.com/scrypster/muninndb/pull/666
* build(deps): bump golang.org/x/crypto from 0.50.0 to 0.52.0 by `dependabot`[bot] in https://github.com/scrypster/muninndb/pull/667
* chore(deps): bump esbuild, vite and vitest in /web by `dependabot`[bot] in https://github.com/scrypster/muninndb/pull/664
* chore(deps-dev): bump vitest from 3.2.4 to 3.2.6 in /sdk/node by `dependabot`[bot] in https://github.com/scrypster/muninndb/pull/665
* build(deps-dev): bump picomatch from 4.0.3 to 4.0.5 in /sdk/node by `dependabot`[bot] in https://github.com/scrypster/muninndb/pull/670
* build(deps): bump google.golang.org/grpc from 1.79.3 to 1.82.1 by `dependabot`[bot] in https://github.com/scrypster/muninndb/pull/668
* build(deps): bump golang.org/x/net from 0.54.0 to 0.55.0 by `dependabot`[bot] in https://github.com/scrypster/muninndb/pull/669
* Migrate Tailwind v3 → v4, unblocking #662 by `scrypster` in https://github.com/scrypster/muninndb/pull/671
* Clear the last 9 Dependabot alerts, and gate the Node SDK in CI by `scrypster` in https://github.com/scrypster/muninndb/pull/672
* Ignore the third maintainer-private skill directory by `scrypster` in https://github.com/scrypster/muninndb/pull/673
* Move GitHub Actions to their Node 24 runtimes by `scrypster` in https://github.com/scrypster/muninndb/pull/674
* fix(fts): Stop() must drain the queue even before workers are scheduled by `scrypster` in https://github.com/scrypster/muninndb/pull/675
* Add Worker.Flush and make awaitFTS actually await by `scrypster` in https://github.com/scrypster/muninndb/pull/676
* Correct two stale code comments; close out fixed drift entries by `scrypster` in https://github.com/scrypster/muninndb/pull/677
* Pin plasticity preset names across Go and the web console by `scrypster` in https://github.com/scrypster/muninndb/pull/678
* fix(engine): inherit MemoryType and TypeLabel across evolve by `isaac-ranger` in https://github.com/scrypster/muninndb/pull/655
* docs: fix Title Case heading capitalization in CONTRIBUTING.md by `amir-rezaei` in https://github.com/scrypster/muninndb/pull/652
* Run the web console's vitest suite in CI by `scrypster` in https://github.com/scrypster/muninndb/pull/679
* fix(engine): carry entity links and relationships across evolve, funding the mention ledgers by `isaac-ranger` in https://github.com/scrypster/muninndb/pull/646
* feat(reflex): session-start orientation primitives — reinforcement, reminders, trust tiering (S0-S8) by `scrypster` in https://github.com/scrypster/muninndb/pull/685
* Land reflex stack: append-mode, tag_filter, dedup-guard, supersedes-aware recall, valid-time by `scrypster` in https://github.com/scrypster/muninndb/pull/688
* fix(engine): rarity-weight entity boost, cap accumulation, gate injection on threshold by `isaac-ranger` in https://github.com/scrypster/muninndb/pull/570
* feat(cognition): importance dimension + pruning protection by `scrypster` in https://github.com/scrypster/muninndb/pull/689
* feat(mcp,engine): optional inline entities on evolve, replacing the carried set by `isaac-ranger` in https://github.com/scrypster/muninndb/pull/680
* fix(engine,storage): startup repair for evolve-stripped successors — atomic, funded, watermarked by `isaac-ranger` in https://github.com/scrypster/muninndb/pull/681
* docs(skill): /increment — the repeatable build loop by `scrypster` in https://github.com/scrypster/muninndb/pull/690
* test(engine): entity-boost lease-fail-closed coverage (#570 follow-up) by `scrypster` in https://github.com/scrypster/muninndb/pull/691
* feat(prospective): THE PUSH increment 1 — armed intentions + notices over MCP by `scrypster` in https://github.com/scrypster/muninndb/pull/694
* fix(trigger): carry the full 8-byte vault prefix on trigger events (#692) by `scrypster` in https://github.com/scrypster/muninndb/pull/697
* fix(recall): deterministic top-N ordering — ULID tie-break at equal scores (#698) by `scrypster` in https://github.com/scrypster/muninndb/pull/699
* fix(prospective): exclude an intention's own engram from its focality (#693) by `scrypster` in https://github.com/scrypster/muninndb/pull/703
* fix(recall): RRF vaults no longer return silently-empty default recall (ranking-honesty R1) by `scrypster` in https://github.com/scrypster/muninndb/pull/705
* fix(contradict): stop fabricating contradictions from same-relation-type/different-target shape (ranking-honesty R2) by `scrypster` in https://github.com/scrypster/muninndb/pull/707
* fix(recall): calibrated FTS relevance → recall can abstain (reliable-colleague #711) by `scrypster` in https://github.com/scrypster/muninndb/pull/715
* fix(recall): recall-mode presets no longer bypass the rrf mode-aware threshold default (#704) by `isaac-ranger` in https://github.com/scrypster/muninndb/pull/710
* fix(engine): shared visibility gate for post-pipeline injections; supersession substitutes atomically under the caller's view by `isaac-ranger` in https://github.com/scrypster/muninndb/pull/701
* docs: calibration is per-vault and self-derived, never hardcoded (principle #11) by `scrypster` in https://github.com/scrypster/muninndb/pull/717
* feat(recall): semantic-abstention floor — anisotropy-calibrated cosine (COG-26) by `scrypster` in https://github.com/scrypster/muninndb/pull/718
* docs(guide): use muninn_evolve, not repeated remember, for superseding facts by `scrypster` in https://github.com/scrypster/muninndb/pull/723
* fix(recall): restore post-load cosine — GetEngrams omits ERF v2 embeddings (#714) by `scrypster` in https://github.com/scrypster/muninndb/pull/722
* docs(internals): test hermeticity guide — drain async before asserting by `scrypster` in https://github.com/scrypster/muninndb/pull/727
* chore(test,docs): use neutral fixtures, remove vault-specific identifiers by `scrypster` in https://github.com/scrypster/muninndb/pull/734
* feat(recall): per-vault exclude-tags knob (#713) by `scrypster` in https://github.com/scrypster/muninndb/pull/735
* feat(recall): advisory version-cluster annotation — currency (#712, COG-25) by `scrypster` in https://github.com/scrypster/muninndb/pull/738
* feat(provenance): record the real write verb instead of hardcoded "create" by `scrypster` in https://github.com/scrypster/muninndb/pull/739
* fix(recall): degrade loudly on embedding failure (semantic_degraded) by `scrypster` in https://github.com/scrypster/muninndb/pull/740
* feat(guide): curator reflex — treat MuninnDB as living memory, reconcile at write by `scrypster` in https://github.com/scrypster/muninndb/pull/741
* test(engine): stop TestCurrencyAnnotation_Latency flaking on CI runner noise by `scrypster` in https://github.com/scrypster/muninndb/pull/744
* fix(mcp): never silently swallow an unrecognized memory type by `scrypster` in https://github.com/scrypster/muninndb/pull/742
* fix(mcp): drop the entity-type enum from the write paths (it cost 64pp of entity coverage) by `scrypster` in https://github.com/scrypster/muninndb/pull/743
* fix(mcp): never silently discard an unrecognized link relation by `scrypster` in https://github.com/scrypster/muninndb/pull/745
* fix: agent-experience hardening — six silent-substitution defects found by hands-on AI evaluation by `scrypster` in https://github.com/scrypster/muninndb/pull/746
* fix(engine): declaring a contradiction no longer destroys both memories by `scrypster` in https://github.com/scrypster/muninndb/pull/747
* fix(cli): handle stale PID file in 'muninn stop' by `ad-astra-bot` in https://github.com/scrypster/muninndb/pull/650
* docs(decision-record): associative surprise — killed, refuted at its premise by `scrypster` in https://github.com/scrypster/muninndb/pull/751
* test(engine): stop TestStartImport_OrphanedVaultCleanup misreporting a timeout as a cleanup bug by `scrypster` in https://github.com/scrypster/muninndb/pull/753
* fix: solid ground — consolidate data loss, inverted abstention, contradictions surface, and the write-effort cliff by `scrypster` in https://github.com/scrypster/muninndb/pull/754
* fix: full-confidence weight destruction, evolve provenance, and the measured recall ceiling by `scrypster` in https://github.com/scrypster/muninndb/pull/757
* fix(engine): currency advisory precision — universal marker gate + declared-chain suppression by `scrypster` in https://github.com/scrypster/muninndb/pull/758
* fix(storage,engine): startup repair for pre-#757 full-weight association keys (#756) by `scrypster` in https://github.com/scrypster/muninndb/pull/759
* fix(storage,auth,engine): time-normalize association decay — 13.5-minute half-life becomes a real forgetting curve (#762) by `scrypster` in https://github.com/scrypster/muninndb/pull/766
* feat(engine,activation,mbp): resolve declared version chains to their head before ranking (#763) by `scrypster` in https://github.com/scrypster/muninndb/pull/767
* fix(cognitive,engine,mcp,mbp,rest): declaring a contradiction changes what recall returns (#764) by `scrypster` in https://github.com/scrypster/muninndb/pull/772
* chore(privacy): deep-review triage, vault-naming rule, and the increment-skill scrub by `scrypster` in https://github.com/scrypster/muninndb/pull/775
* feat(engine,mcp,mbp,rest): a first-class relevance band — recall stops dressing weak matches as certainties (#773) by `scrypster` in https://github.com/scrypster/muninndb/pull/778
* test: drain the activation log before the saturating-arm recall by `scrypster` in https://github.com/scrypster/muninndb/pull/782
* docs(changelog): 0.10.0 — the trust release by `scrypster` in https://github.com/scrypster/muninndb/pull/781
* test: drain write-time workers before the currency zero-writes snapshots (#777) by `scrypster` in https://github.com/scrypster/muninndb/pull/784
* Release v0.10.0 — the trust release by `scrypster` in https://github.com/scrypster/muninndb/pull/783

## New Contributors
* `dependabot`[bot] made their first contribution in https://github.com/scrypster/muninndb/pull/667
* `amir-rezaei` made their first contribution in https://github.com/scrypster/muninndb/pull/652

**Full Changelog**: https://github.com/scrypster/muninndb/compare/v0.9.0...v0.10.0

---


## 0.9.0

_2026-07-21_

> _Maintenance (2026-07-25):_ **Fix the app restarting every ~2 minutes in an endless boot loop.** The container healthcheck probed `/` on port 8475, which MuninnDB does not serve — it returns 404 — so the app never reported healthy. The Supervisor gates the STARTUP -> STARTED transition on that healthcheck, so it timed out after 120s and the watchdog restarted a perfectly healthy app, over and over (`Timeout while waiting for app MuninnDB to start` followed by `Watchdog found app MuninnDB is unhealthy, restarting...`). The probe now uses `/api/health`. Diagnosed on-device against 0.9.0; the database itself was starting cleanly and both vaults provisioned every cycle.

> _Maintenance (2026-07-25):_ **Documented how to use an embedding model other than the bundled one.** The `ollama_url` option accepts any Ollama-compatible server and the model name is part of the URL (`ollama://host:port/model`) — leaving it out silently does not work, and the old option text did not say so. DOCS.md now covers pairing with the Lemonade app, which can host the embedding model and manage its download; MuninnDB itself never fetches models. Choose an embedding model before storing memories: vectors from different models are not comparable, and dimensions differ (bundled 384, LFM2.5-Embedding-350M 1024).

> _Maintenance (2026-07-25):_ **Two vaults are now provisioned on first start instead of one.** `default_vault` is your general-purpose vault (default `default`) and the new `ha_vault` is dedicated to Home Assistant (default `home_assistant`). MuninnDB ranks memories by how recently and often they are recalled, so a voice assistant — which writes far more often than a person does — would otherwise crowd out your own notes in a shared vault. Both are created open, so add-ons need no API key; vaults created any other way start locked. Existing installs are unaffected: `default_vault` keeps whatever value you already set, and if the two options end up with the same name only one vault is created and the app logs a warning.

> _Maintenance (2026-07-25):_ **Fix the app being SIGKILLed instead of shut down cleanly.** The AppArmor profile granted `signal (send) set=(...)` but not `receive`. AppArmor mediates signal *delivery* as well as sending, so s6's SIGTERM never reached the app: the graceful shutdown was skipped and the container was killed after the grace period (exit 137). Because the shutdown path never ran, `backup_on_shutdown` silently never took its pre-stop backup. The rule is now `signal (send,receive),`. Measured on one identical image with only this rule varied: no signal rule -> 13.2s/exit 137; `signal (send) set=(...)` -> 12.7s/exit 137 with no cleanup logged; `signal (send,receive),` -> 6.9s/exit 0 with the full s6 shutdown sequence. Now enforced by `.github/scripts/validate-apparmor.sh` (rule 4) and by the smoke test, which fails on exit 137 under confinement. Rebuild/reinstall the app to pick up the corrected profile.

## What's Changed
* feat(auth): add working plasticity preset (RFC #597) by `madeinoz67` in https://github.com/scrypster/muninndb/pull/599
* ci: rename stale 'minilm-v2' embed-asset cache keys to bge-small-en-v1.5 (#601) by `johanneshauer` in https://github.com/scrypster/muninndb/pull/603
* fix(storage): serialize DeleteEngram with CompareAndSet's stripe lock by `ad-astra-bot` in https://github.com/scrypster/muninndb/pull/594
* fix(activation): preserve explicit search threshold under RRF + expose Search Scoring setting by `gitssie` in https://github.com/scrypster/muninndb/pull/590
* feat(mcp): capability tokens + muninn_create_workflow_vault (RFC #597) by `madeinoz67` in https://github.com/scrypster/muninndb/pull/612
* feat(mcp): toolset profiles for tools/list — env default + per-connection header override (#588) by `isaac-ranger` in https://github.com/scrypster/muninndb/pull/604
* docs: AI-agent working guide, internals reference, and code-review agent by `scrypster` in https://github.co

---


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



## 0.6.1

_2026-05-27_

> _Maintenance (2026-06-10):_ added hassio_role: manager so bashio can read the app config + Supervisor API on base 20.2.0 (fixes "Unable to access the API, forbidden"); migrated bashio::addon.* to bashio::app.*. Rebuild the app to apply the new role.

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
- `fix(engine)` — content-hash dedup race, enrichment ghost queue deadlock, trigger nil metadata crash
- `fix(activation)` — BFS/RRF score fix for traversed candidates
- `fix(rest)` — 400 instead of 500 for invalid engram IDs; phantom vault auth config cleanup
- `fix(import)` — pipe deadlock and orphaned vault name on failed import
- `fix(auth)` — validate Bearer token before parsing body to prevent DoS amplification
- `fix(security)` — gRPC bumped to v1.79.3, govulncheck added to CI

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
