# Arcane Docker Manager Add-on for Home Assistant

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]

Modern Docker management UI - a lightweight, beautiful alternative to Portainer.

## About

Arcane is a modern, open-source Docker management platform designed for everyone. It provides an intuitive web-based interface for managing Docker containers, images, volumes, networks, and Docker Compose stacks. This add-on brings Arcane to Home Assistant, integrating it seamlessly with the sidebar and providing easy access to all Docker management features.

## Features

- Modern, mobile-friendly web interface
- Container management with real-time stats
- Docker Compose stack management with templates
- Resource monitoring with CPU, memory, and network graphs
- Image, volume, and network management
- Automatic container image updates
- Multi-host management via agent architecture
- Discord and email notifications
- OIDC Single Sign-On support
- Built-in Docker system cleanup (prune)
- Ingress support for seamless sidebar integration
- Persistent data storage included in backups

## Installation

1. Add this repository to your Home Assistant instance
2. Search for "Arcane Docker Manager" in the add-on store
3. Click Install
4. Configure the add-on options (if needed)
5. Start the add-on
6. Click "OPEN WEB UI" or access via the sidebar

## Configuration

### Option: `log_level`

The `log_level` option controls the level of log output by the addon:

- `trace`: Show every detail
- `debug`: Shows detailed debug information
- `info`: Normal (usually) interesting events (default)
- `warning`: Exceptional occurrences that are not errors
- `error`: Runtime errors
- `fatal`: Critical errors

### Option: `app_url`

The base URL for the Arcane application. Leave empty for default (`http://localhost:3552`).
Set this if you're accessing Arcane through a reverse proxy or custom domain.

### Option: `puid` / `pgid`

User/Group ID for Docker file permissions. Default is 1000.

### Option: `hide_hassio_containers`

When enabled (default), hides Home Assistant system containers from the Arcane interface.

## Folder Access

This addon has access to the following Home Assistant directories:

- `/ssl` - SSL certificates (read-only)
- `/data` - Addon persistent data (read/write)
- `/media` - Home Assistant media folder (read/write)
- `/share` - Home Assistant share folder (read/write)

## First Time Setup

1. When you first access Arcane, you'll be prompted to create an admin user
2. Default credentials are: `arcane` / `arcane-admin` (change immediately!)
3. Configure your Docker environment and start managing containers

## Support

Got questions or found a bug? Please open an issue on the GitHub repository.

[aarch64-shield]: https://img.shields.io/badge/aarch64-yes-green.svg
[amd64-shield]: https://img.shields.io/badge/amd64-yes-green.svg

## Version

Currently running Arcane 1.16.3
