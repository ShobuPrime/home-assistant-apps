# Dockge App for Home Assistant

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]

A fancy, easy-to-use and reactive self-hosted docker compose.yaml stack-oriented manager.

## About

Dockge is a self-hosted, easy-to-use Docker Compose stack manager. It provides a web interface to manage your Docker Compose stacks with features like an interactive YAML editor, web terminal, and real-time updates. This app brings Dockge to Home Assistant, integrating it seamlessly with the sidebar.

## Features

- 🚀 Easy-to-use: Simple and intuitive web interface
- 📝 Interactive Editor: Edit compose.yaml files with syntax highlighting
- 🖥️ Web Terminal: a terminal into a stack's containers
- 🔄 Real-time Updates: See container status changes instantly
- 🐳 Docker Compose Management: Start, stop, restart stacks easily
- 🔧 Convert Docker Run: Convert `docker run` commands to compose.yaml
- 💾 Stacks kept in the app's data folder and included in its backups
- 🎯 Works in the Home Assistant sidebar
- 🏷️ Leaves out what Home Assistant manages

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

### Option: `hide_hassio_containers`

When enabled (default), Compose projects whose containers Home Assistant
manages are left out of Dockge, so it lists your own. See the documentation
for details.

## Support

Got questions or found a bug? Please open an issue on the GitHub repository.

[aarch64-shield]: https://img.shields.io/badge/aarch64-yes-green.svg
[amd64-shield]: https://img.shields.io/badge/amd64-yes-green.svg

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