# Dockge App for Home Assistant

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]
![Supports armhf Architecture][armhf-shield]
![Supports armv7 Architecture][armv7-shield]
![Supports i386 Architecture][i386-shield]

A fancy, easy-to-use and reactive self-hosted docker compose.yaml stack-oriented manager.

## About

Dockge is a self-hosted, easy-to-use Docker Compose stack manager. It provides a web interface to manage your Docker Compose stacks with features like an interactive YAML editor, web terminal, and real-time updates. This app brings Dockge to Home Assistant, integrating it seamlessly with the sidebar.

## Features

- 🚀 Easy-to-use: Simple and intuitive web interface
- 📝 Interactive Editor: Edit compose.yaml files with syntax highlighting
- 🖥️ Web Terminal: Access terminal directly from the browser (console disabled by default in 1.5.0+)
- 🔄 Real-time Updates: See container status changes instantly
- 🐳 Docker Compose Management: Start, stop, restart stacks easily
- 🔧 Convert Docker Run: Convert `docker run` commands to compose.yaml
- 💾 Persistent data storage included in backups
- 🎯 Ingress support for seamless sidebar integration
- 🏷️ Option to hide Home Assistant system containers

## Installation

1. Add this repository to your Home Assistant instance
2. Search for "Dockge" in the app store
3. Click Install
4. Configure the app options (if needed)
5. Start the app
6. Click "OPEN WEB UI" or access via the sidebar

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

### Option: `stacks_dir`

The directory where Docker Compose stacks will be stored. Default is `/opt/stacks`.
All stacks are automatically included in Home Assistant backups.

### Option: `hide_hassio_containers`

When enabled (default), hides Home Assistant system containers from the Dockge
interface. This includes supervisor, core, audio, dns, multicast, cli, observer,
and app containers.

## Folder Access

This app has access to the following Home Assistant directories:

- `/data` - App persistent data (read/write)
- `/share` - Home Assistant share folder (read/write)

These folders are accessible from within Dockge and can be used for stack storage or volume mounts.

## First Time Setup

1. When you first access Dockge, you'll be presented with the main dashboard
2. Click "Compose" or the "+" button to create a new stack
3. Enter your docker-compose.yaml configuration
4. Click "Deploy" to start the stack

## Docker Socket Access

This app requires access to the Docker socket to manage containers. For security,
ensure you understand the implications of granting Docker access to this app.

## Known Issues and Limitations

- **Console Feature**: As of Dockge 1.5.0, the console/terminal feature is disabled
  by default for security. Since this app uses the official Docker image, enabling
  the console would require building a custom image with `DOCKGE_ENABLE_CONSOLE=true`
- Full Docker socket access is required
- Ingress port must be set to 5001 (default)

## Updating the App

An update script is included to check for new Dockge versions:

```bash
# Check for updates only
/config/addons/dockge/update-dockge-version.sh --check-only

# Apply update
/config/addons/dockge/update-dockge-version.sh --yes
```

## Support

Got questions or found a bug? Please open an issue on the GitHub repository.

[aarch64-shield]: https://img.shields.io/badge/aarch64-yes-green.svg
[amd64-shield]: https://img.shields.io/badge/amd64-yes-green.svg
[armhf-shield]: https://img.shields.io/badge/armhf-yes-green.svg
[armv7-shield]: https://img.shields.io/badge/armv7-yes-green.svg
[i386-shield]: https://img.shields.io/badge/i386-yes-green.svg

## Version

Currently running Dockge 1.5.0

## License

MIT License

Copyright (c) 2024 Home Assistant Community Apps

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.