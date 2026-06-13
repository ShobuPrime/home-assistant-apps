#!/usr/bin/env python3
"""
Minimal mock of the Home Assistant Supervisor API.
Responds to the endpoints bashio/S6 base scripts call during app startup.
Run with: python3 mock-supervisor.py <app-dir> [port]
"""

import json
import sys
import os
from http.server import HTTPServer, BaseHTTPRequestHandler

APP_DIR = sys.argv[1] if len(sys.argv) > 1 else "."
PORT = int(sys.argv[2]) if len(sys.argv) > 2 else 80

# When the smoke test runs a real MQTT broker on the network it passes the
# broker's coordinates here, so the app's MQTT auto-detection
# (GET /services/mqtt) succeeds and the real discovery/publish path is
# exercised. Unset (the default) -> /services/mqtt 404s, exactly as before,
# and the app falls back to the REST API.
MQTT_HOST = os.environ.get("MOCK_MQTT_HOST")
MQTT_PORT = int(os.environ.get("MOCK_MQTT_PORT", "1883"))
MQTT_USER = os.environ.get("MOCK_MQTT_USER", "")
MQTT_PASS = os.environ.get("MOCK_MQTT_PASS", "")

# Load app config
with open(os.path.join(APP_DIR, "config.yaml")) as f:
    import re
    content = f.read()
    name = re.search(r'^name:\s*"(.+)"', content, re.M)
    version = re.search(r'^version:\s*"(.+)"', content, re.M)
    slug = re.search(r'^slug:\s*"(.+)"', content, re.M)
    app_name = name.group(1) if name else "Test App"
    app_version = version.group(1) if version else "0.0.0"
    app_slug = slug.group(1) if slug else "test"

# Load options (defaults from config.yaml)
options = {}
try:
    # Simple YAML options parser (avoids pyyaml dependency)
    in_options = False
    in_schema = False
    for line in content.split("\n"):
        if line.startswith("schema:"):
            in_schema = True
            in_options = False
            continue
        if line.startswith("options:"):
            in_options = True
            continue
        if in_options and line.startswith("  ") and not line.startswith("    "):
            key, _, val = line.strip().partition(":")
            val = val.strip().strip('"')
            if val == "true":
                val = True
            elif val == "false":
                val = False
            elif val.isdigit():
                val = int(val)
            elif val == "":
                val = ""
            options[key] = val
        elif in_options and not line.startswith(" "):
            in_options = False
except Exception:
    pass

# API responses
APP_INFO = {
    "result": "ok",
    "data": {
        "name": app_name,
        "slug": app_slug,
        "hostname": app_slug,
        "state": "started",
        "version": app_version,
        "version_latest": app_version,
        "boot": "auto",
        "options": options,
        "arch": ["aarch64", "amd64"],
        "ingress": True,
        "ingress_port": 8099,
        "ingress_entry": f"/api/hassio_ingress/{app_slug}",
        "ip_address": "172.30.33.1",
        "watchdog": True,
    },
}

SUPERVISOR_INFO = {
    "result": "ok",
    "data": {
        "version": "2026.03.0",
        "version_latest": "2026.03.0",
        "channel": "stable",
        "arch": "amd64",
        "supported": True,
        "healthy": True,
        "logging": "info",
    },
}

OS_INFO = {
    "result": "ok",
    "data": {
        "version": "17.1",
        "version_latest": "17.1",
        "board": "generic-x86-64",
    },
}

CORE_INFO = {
    "result": "ok",
    "data": {
        "version": "2026.3.1",
        "version_latest": "2026.3.1",
    },
}

OPTIONS_CONFIG = {"result": "ok", "data": options}


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        path = self.path.rstrip("/")

        routes = {
            "/addons/self/info": APP_INFO,
            "/addons/self/options/config": OPTIONS_CONFIG,
            "/addons/self/options": {"result": "ok", "data": {"options": options}},
            "/supervisor/info": SUPERVISOR_INFO,
            "/supervisor/ping": {"result": "ok", "data": {}},
            "/os/info": OS_INFO,
            "/core/info": CORE_INFO,
            "/info": SUPERVISOR_INFO,
            # hassio-addons base >= 20.2.0 queries the store during startup
            # (banner version check / bashio init); a 404 here is now fatal to
            # cont-init, so serve a valid (empty) store + this app's entry.
            "/store": {"result": "ok", "data": {"addons": [], "repositories": []}},
            "/store/addons": {
                "result": "ok",
                "data": [
                    {
                        "slug": app_slug,
                        "name": app_name,
                        "version": app_version,
                        "version_latest": app_version,
                        "installed": app_version,
                        "update_available": False,
                    }
                ],
            },
        }

        # Only advertise the MQTT service when the smoke test actually started
        # a broker (MOCK_MQTT_HOST set); otherwise a 404 here makes the app use
        # its REST fallback, preserving the original behavior for non-MQTT apps.
        if MQTT_HOST:
            routes["/services/mqtt"] = {
                "result": "ok",
                "data": {
                    "addon": "core_mosquitto",
                    "host": MQTT_HOST,
                    "port": MQTT_PORT,
                    "ssl": False,
                    "protocol": "3.1.1",
                    "username": MQTT_USER,
                    "password": MQTT_PASS,
                },
            }

        if path in routes:
            self._respond(200, routes[path])
        else:
            self._respond(404, {"result": "error", "message": f"Not found: {path}"})

    def do_POST(self):
        # Some bashio calls use POST (e.g., reload)
        self._respond(200, {"result": "ok", "data": {}})

    def _respond(self, code, data):
        body = json.dumps(data).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format, *args):
        # Suppress request logging to keep CI output clean
        pass


if __name__ == "__main__":
    server = HTTPServer(("0.0.0.0", PORT), Handler)
    print(f"Mock Supervisor listening on :{PORT} for app '{app_slug}'", flush=True)
    server.serve_forever()
