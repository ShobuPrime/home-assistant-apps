# Huly App for Home Assistant

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]
![Doesn't support armhf Architecture][armhf-shield]
![Doesn't support armv7 Architecture][armv7-shield]
![Doesn't support i386 Architecture][i386-shield]

An open-source, all-in-one project management platform — alternative to Linear, Jira, Slack, and Notion.

## About

Huly is a self-hosted project management platform that combines issue tracking, team chat, collaborative documents, and video meetings into a single application. This app brings the complete Huly stack to Home Assistant, orchestrating all 14 required services via Docker Compose internally.

## Features

- 📋 Project Management: Issue tracking, kanban boards, and sprint planning
- 💬 Team Chat: Real-time messaging with channels and direct messages
- 📝 Collaborative Documents: Rich text editor with real-time collaboration
- 🎥 Video Meetings: Built-in video conferencing
- 🔄 Real-time Updates: Live synchronization across all features
- 🐳 Full Stack Deployment: All 14 services orchestrated automatically
- 💾 Persistent data storage included in backups
- 🎯 Ingress support for seamless sidebar integration
- 🔐 Automatic secret generation on first run
- 🌐 Configurable language and display preferences

## System Requirements

Huly is a resource-intensive application that runs 14 internal services including CockroachDB, Elasticsearch, and MinIO.

- **Minimum**: 2 vCPUs, 8 GB RAM
- **Recommended**: 4+ vCPUs, 16+ GB RAM
- **Storage**: At least 10 GB free disk space (more as your data grows)

> **Warning**: Running Huly on underpowered hardware may result in slow startup times, service crashes, or out-of-memory errors.

## Installation

1. Add this repository to your Home Assistant instance
2. Search for "Huly" in the app store
3. Click Install
4. Disable protection mode (required for Docker socket access)
5. Configure the `host_address` option with your domain or IP
6. Start the app
7. Wait for all services to initialize (first start may take several minutes)
8. Click "OPEN WEB UI" or access via the sidebar

## Configuration

### Option: `log_level`

The `log_level` option controls the level of log output by the app and can
be changed to be more or less verbose, which might be useful when you are
dealing with an unknown issue. Possible values are:

- `trace`: Show every detail, like all called internal functions.
- `debug`: Shows detailed debug information.
- `info`: Normal (usually) interesting events.
- `warning`: Exceptional occurrences that are not errors.
- `error`: Runtime errors that do not require immediate action.
- `fatal`: Something went terribly wrong. App becomes unusable.

### Option: `host_address`

The domain name or IP address used to access your Huly instance. This is
**required** and must be set before starting the app. Examples:

- `huly.example.com` (if using a reverse proxy with a domain)
- `192.168.1.100` (if accessing via local IP)

### Option: `title`

The title displayed in the Huly web interface. Default is `Huly`.

### Option: `default_language`

The default language for the Huly UI. Default is `en` (English).

### Option: `last_name_first`

Controls the name display order throughout the platform.
- `true` (default): Display as "Last First"
- `false`: Display as "First Last"

## Folder Access

This app has access to the following Home Assistant directories:

- `/data` - App persistent data (read/write)
- `/share` - Home Assistant share folder (read/write)

All Huly data (databases, object storage, configuration) is stored under `/data` and automatically included in Home Assistant backups.

## First Time Setup

1. Ensure `host_address` is configured with your domain or IP
2. Start the app and wait for all 14 services to initialize
3. Access Huly via the sidebar or direct URL
4. Create your first workspace and admin account
5. Invite team members and begin managing your projects

> **Note**: The first startup may take 3-5 minutes as databases are initialized and services warm up.

## Docker Socket Access

This app requires access to the Docker socket to orchestrate the internal
service stack via Docker Compose. Protection mode must be disabled in the
app configuration for the Docker socket to be accessible.

## Known Issues and Limitations

- **Resource Usage**: Huly runs 14 services and requires significant memory and CPU
- **Startup Time**: First start can take several minutes while services initialize
- **Protection Mode**: Must be disabled for Docker socket access
- Full Docker socket access is required
- Ingress port must be set to 4859 (default)
- Only aarch64 and amd64 architectures are supported

## Support

Got questions or found a bug? Please open an issue on the GitHub repository.

[aarch64-shield]: https://img.shields.io/badge/aarch64-yes-green.svg
[amd64-shield]: https://img.shields.io/badge/amd64-yes-green.svg
[armhf-shield]: https://img.shields.io/badge/armhf-no-red.svg
[armv7-shield]: https://img.shields.io/badge/armv7-no-red.svg
[i386-shield]: https://img.shields.io/badge/i386-no-red.svg

## Version

Currently running Huly 0.7.382

## License

Eclipse Public License - v 2.0 (EPL-2.0)

Copyright (c) 2024 Huly contributors

This program and the accompanying materials are made available under the
terms of the Eclipse Public License v. 2.0 which is available at
http://www.eclipse.org/legal/epl-2.0.

This Source Code may also be made available under the following Secondary
Licenses when the conditions for such availability set forth in the Eclipse
Public License v. 2.0 are satisfied: GNU General Public License, version 2
with the GNU Classpath Exception which is available at
https://www.gnu.org/software/classpath/license.html.
