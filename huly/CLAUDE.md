# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant Add-on for Huly, an open-source all-in-one project management platform. The add-on deploys the complete Huly self-hosted stack (14 Docker services) within a single Home Assistant add-on container, using Docker Compose for internal orchestration.

### What Huly Provides
- Project management with issue tracking, kanban boards, and sprints
- Team chat with channels and direct messages
- Collaborative documents with real-time co-editing
- Video meetings
- Time tracking and HR modules

## Architecture

### Internal Services (14 total)
The add-on orchestrates these services via Docker Compose:

1. **nginx** - Reverse proxy (only externally exposed service, port 4859)
2. **CockroachDB** - Distributed SQL database
3. **Elasticsearch** - Full-text search engine
4. **MinIO** - S3-compatible object storage
5. **Apache Kafka** - Event streaming (KRaft mode; upstream uses Redpanda but it crashes on Cortex-A76/Pi CM5)
6. **Account** - Authentication and workspace management
7. **Front** - Web frontend
8. **Collaborator** - Real-time collaboration
9. **Fulltext** - Search indexing
10. **Rekoni** - Document thumbnails/previews
11. **Transactor** - Database transaction coordinator
12. **Print** - Document export
13. **Sign** - Document signing
14. **Stats** - Analytics/telemetry

### Docker Compose Orchestration
Unlike simpler add-ons that use a single Docker image, this add-on:
- Uses the hassio-addons base image with S6-overlay
- Installs Docker Compose inside the container
- Generates a `docker-compose.yaml` from templates based on user configuration
- Manages the full service lifecycle through S6 init/service scripts

## Key File Locations

- **`config.yaml`** - Add-on metadata, architecture support, configuration schema
- **`build.yaml`** - Build configuration with base image and architecture args
- **`Dockerfile`** - Container build instructions (installs Docker Compose, copies rootfs)
- **`apparmor.txt`** - AppArmor security profile allowing Docker socket access
- **`rootfs/etc/cont-init.d/`** - S6 initialization scripts (secret generation, config setup)
- **`rootfs/etc/services.d/huly/`** - S6 service run/finish scripts for Docker Compose
- **`rootfs/etc/huly/`** - Docker Compose templates and configuration files

## How Version Updates Work

Huly version tracking is based on the `.template.huly.conf` file in the [huly-selfhost](https://github.com/hcengineering/huly-selfhost) repository, **NOT** GitHub Releases. The update process:

1. Check the `huly-selfhost` repo for changes to `.template.huly.conf`
2. Extract the new version string from the template
3. Update `config.yaml` version field
4. Update Docker Compose template image tags
5. Update documentation version references

> **Important**: Do NOT use GitHub Releases API for version detection. The huly-selfhost repo uses config template files for versioning.

## Testing Checklist

- [ ] Add-on installs without errors
- [ ] Protection mode is disabled and Docker socket is accessible
- [ ] All 14 services start successfully (check logs)
- [ ] Web UI is accessible on port 4859
- [ ] Ingress integration works from HA sidebar
- [ ] WebSocket connections work (real-time updates in UI)
- [ ] Can create a workspace and user account
- [ ] Data persists across add-on restarts
- [ ] Secrets are generated on first run and reused on subsequent starts
- [ ] Add-on stops cleanly (all services shut down via Docker Compose)

## Common Pitfalls

1. **Resource starvation**: Huly needs at least 8 GB RAM. Services will crash with OOM on smaller systems
2. **Startup ordering**: Services have dependencies — CockroachDB, Elasticsearch, MinIO, and Kafka must be healthy before app services start
3. **Secret management**: Secrets are generated once and stored in `/data/huly/secrets`. Regenerating them will break existing data
4. **Docker socket**: Must have `docker_api: true` in config.yaml and protection mode disabled
5. **Host address**: The `host_address` config option is critical — without it, internal service URLs will be misconfigured
6. **First startup time**: Can take 3-5 minutes for all databases to initialize. Do not restart during this window

## Resource Requirements

- **Minimum**: 2 vCPUs, 8 GB RAM, 10 GB storage
- **Recommended**: 4+ vCPUs, 16+ GB RAM, 20+ GB storage
- The largest memory consumers are CockroachDB, Elasticsearch, and the Huly Transactor

## Important Patterns

### S6-Overlay Scripts
- All scripts use `#!/usr/bin/with-contenv bashio` shebang
- Init scripts in `/rootfs/etc/cont-init.d/` run sequentially before services
- Service scripts in `/rootfs/etc/services.d/huly/run` manage Docker Compose
- Use Bashio functions for logging (`bashio::log.info`) and config reading (`bashio::config`)

### Configuration Handling
- User options from `config.yaml` schema are read via Bashio in init scripts
- Values are injected into the Docker Compose template or environment files
- The `host_address` option flows into multiple service configurations (nginx, account, front)

### Data Persistence
- All persistent data lives under `/data/huly/`
- Subdirectories: `cockroach/`, `elastic/`, `minio/`, `kafka/`, `config/`, `secrets/`
- Included automatically in Home Assistant backups
- Backup size can be substantial due to databases and object storage
