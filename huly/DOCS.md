# Huly Documentation

## About

Huly is an open-source, all-in-one project management platform that serves as a self-hosted alternative to Linear, Jira, Slack, and Notion. It combines project management, issue tracking, team chat, collaborative documents, and video meetings into a single application. This add-on deploys the complete Huly stack within Home Assistant, orchestrating 14 Docker services via Docker Compose internally.

## Configuration

### Option: `log_level`

The `log_level` option controls the level of log output by the addon:
- `trace`: Show every detail
- `debug`: Shows detailed debug information
- `info`: Normal (usually) interesting events (default)
- `warning`: Exceptional occurrences that are not errors
- `error`: Runtime errors
- `fatal`: Critical errors

### Option: `host_address`

The domain name or IP address used to access your Huly instance.
- **Required**: Must be set before starting the add-on
- Examples: `huly.example.com`, `192.168.1.100`
- Used to configure internal service endpoints and generate external URLs

### Option: `title`

The title displayed in the Huly web interface.
- Default: `Huly`
- Appears in the browser tab and login screen

### Option: `default_language`

The default language for the Huly UI.
- Default: `en` (English)
- Applied to new user accounts and the login screen

### Option: `last_name_first`

Controls name display ordering across the platform.
- `true` (default): Display as "Last First"
- `false`: Display as "First Last"

### Credentials

All credential fields are optional. Leave them blank and the add-on will auto-generate secure random values on first startup.

Set these values if you want credentials to survive an add-on reinstall without losing data. Home Assistant Supervisor stores config values independently of `/data`, so they persist even when the add-on is removed and re-installed — whereas auto-generated secrets stored in `/data` may be lost.

| Option | Description |
|---|---|
| `server_secret` | Huly internal server secret (hex string) |
| `cockroachdb_password` | CockroachDB `selfhost` user password |
| `minio_root_user` | MinIO root access key |
| `minio_root_password` | MinIO root secret key |

> **Warning**: Changing a credential after the initial setup will cause an authentication mismatch with the existing data. If you need to change passwords after data has been created, you must stop the add-on, wipe the corresponding service data directory under `/data/huly/`, and restart.

## Access Methods

1. **Via Sidebar**: Click the Huly icon in Home Assistant (uses ingress)
2. **Direct HTTP**: `http://[your-ip]:4859`

## Port Information

- **4859**: HTTP web interface (default and only exposed port)

## System Requirements

Huly runs 14 internal Docker services and is significantly more resource-intensive than most Home Assistant add-ons.

### Minimum Requirements
- **CPU**: 2 vCPUs
- **RAM**: 8 GB
- **Storage**: 10 GB free disk space

### Recommended Requirements
- **CPU**: 4+ vCPUs
- **RAM**: 16+ GB
- **Storage**: 20+ GB free disk space

Running on hardware below minimum requirements may result in:
- Services failing to start or crashing with OOM errors
- Extremely slow startup times (10+ minutes)
- Degraded performance and unresponsive UI

## Services Architecture

Huly runs 14 internal Docker services, all orchestrated via Docker Compose:

- **nginx**: Reverse proxy routing requests to backend services
- **CockroachDB**: Distributed SQL database for application data
- **Elasticsearch**: Full-text search engine for content indexing
- **MinIO**: S3-compatible object storage for files and attachments
- **Redpanda**: Kafka-compatible event streaming platform
- **Account**: User authentication and workspace management
- **Front**: Web application frontend
- **Collaborator**: Real-time collaboration engine
- **Fulltext**: Full-text search indexing service
- **Rekoni**: Document thumbnail and preview generation
- **Transactor**: Database transaction coordinator
- **Print**: Document export and printing service
- **Sign**: Document signing service
- **Stats**: Analytics and telemetry collection

All services communicate over an internal Docker network. Only nginx (port 4859) is exposed externally.

## Data Persistence

- **CockroachDB data**: Stored in `/data/huly/cockroach`
- **Elasticsearch data**: Stored in `/data/huly/elastic`
- **MinIO data**: Stored in `/data/huly/minio`
- **Redpanda data**: Stored in `/data/huly/redpanda`
- **Huly configuration**: Stored in `/data/huly/config`
- **Generated secrets**: Stored in `/data/huly/secrets`

All data is automatically included in Home Assistant backups.

> **Note**: Backup size may be substantial due to the databases and object storage.

## Features Overview

- **Project Management**: Kanban boards, sprint planning, issue tracking with custom fields and workflows
- **Team Chat**: Channels, direct messages, threads, and file sharing
- **Collaborative Documents**: Rich text editor with real-time co-editing
- **Video Meetings**: Built-in video conferencing without external dependencies
- **Time Tracking**: Log time against issues and generate reports
- **HR Module**: Employee management, vacations, and department structure

## Email Configuration

Huly supports email notifications for workspace invitations and updates. Email configuration is handled through the Huly web interface after initial setup:

1. Navigate to your Huly workspace settings
2. Configure SMTP settings (server, port, credentials)
3. Test the connection and enable notifications

## Security Considerations

### Docker Socket Access
This add-on requires full Docker socket access to orchestrate the internal service stack. This grants the add-on control over your Docker environment.

### Protection Mode
Protection mode must be disabled in the add-on configuration for the Docker socket to be accessible.

### Automatic Secrets
On first startup, the add-on automatically generates secure random secrets for inter-service authentication. These are stored persistently in `/data/huly/secrets`.

### Network Isolation
All 14 internal services communicate over a private Docker network. Only the nginx reverse proxy is exposed on the configured port.

## Troubleshooting

### Services fail to start
- Check that your system meets the minimum requirements (2 vCPUs, 8 GB RAM)
- Review the add-on logs for specific service errors
- Ensure there is sufficient free disk space

### Cannot access Docker socket
Ensure protection mode is disabled in the addon configuration.

### Ingress not working
Ensure the ingress port is set to 4859 (the default Huly proxy port).

### Slow or unresponsive after startup
- Allow 3-5 minutes for all services to fully initialize on first start
- Check system resource usage — Huly may need more RAM or CPU
- Review Elasticsearch and CockroachDB logs for memory pressure warnings

### Database errors after update
- Stop the add-on, wait 30 seconds, then restart
- Check CockroachDB logs for migration status
- If persistent, restore from a backup taken before the update

### WebSocket errors in browser
Ensure `ingress_stream: true` is configured (this should be set automatically). Try clearing browser cache or accessing via direct URL instead of ingress.

## Updating

The addon tracks Huly self-hosted releases. Updates appear in the Home Assistant UI when available. The Huly version is tracked based on the `.template.huly.conf` file in the `huly-selfhost` repository, not GitHub Releases.

> **Important**: Always create a backup before updating. Database migrations may occur during updates.
