# Changelog

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
