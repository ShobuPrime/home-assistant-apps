# Traefik Reverse Proxy Integration

This repository's apps support automatic Traefik reverse proxy configuration via the **Traefik File Provider**. Each app generates a dynamic YAML config file that Traefik watches and hot-reloads — no Docker labels needed.

## How It Works

Home Assistant app containers are managed by the Supervisor, which doesn't support custom Docker labels. Instead, each app writes a Traefik dynamic configuration file to `/share/traefik/dynamic/` when `traefik_enable` is set to `true` in the app's configuration.

This approach is **additive** — it works alongside your existing Docker label-based Traefik routing (e.g., Authelia, other services). Traefik merges dynamic configs from all providers automatically.

## One-Time Traefik Setup

### 1. Add the File Provider to your Traefik static config

```yaml
# traefik.yaml (your existing static config)
providers:
  docker:                              # Your existing Docker provider — unchanged
    endpoint: "unix:///var/run/docker.sock"
    exposedByDefault: false
    network: traefik_proxy
  file:                                # Add this block
    directory: "/etc/traefik/dynamic"
    watch: true
```

### 2. Mount the shared directory in your Traefik container

```yaml
# docker-compose.yml for Traefik
services:
  traefik:
    volumes:
      # ... your existing volumes ...
      - /mnt/data/supervisor/share/traefik/dynamic:/etc/traefik/dynamic:ro
```

On standard HAOS, the share directory is at `/mnt/data/supervisor/share/`. The `:ro` mount is sufficient — Traefik only reads these files.

## Per-App Configuration

Each app has these configuration options (set via the HA UI):

| Option | Default | Description |
|--------|---------|-------------|
| `traefik_enable` | `false` | Enable Traefik config generation |
| `traefik_domain` | `""` | Domain name (e.g., `portainer.example.com`) |
| `traefik_entrypoints` | `"websecure"` | Traefik entrypoint name |
| `traefik_certresolver` | `"cloudflare"` | TLS certificate resolver name |

### Example: Enable Traefik for Arcane

1. Go to **Settings > Apps > Arcane > Configuration**
2. Set:
   - `traefik_enable`: true
   - `traefik_domain`: `arcane.yourdomain.com`
3. Restart the app

A config file appears at `/share/traefik/dynamic/arcane.yml` and Traefik starts routing immediately.

## Multi-Port Apps

**MuninnDB** exposes multiple services. Additional domain options are available:

| Option | Port | Description |
|--------|------|-------------|
| `traefik_domain` | 8476 | Web UI |
| `traefik_api_domain` | 8475 | REST API |
| `traefik_grpc_domain` | 8477 | gRPC API |
| `traefik_mcp_domain` | 8750 | MCP (AI tools) |

Set only the domains you need — unconfigured ports are not exposed.

## HTTPS Backend (Portainer)

Portainer can use HTTPS internally (port 9443). When `ssl` is enabled in the Portainer app config, the generated Traefik config automatically:
- Routes to `https://host:9443` instead of `http://host:9000`
- Adds `insecureSkipVerify: true` for Portainer's self-signed certificate

## Generated Config Format

Each app writes a file like `/share/traefik/dynamic/<slug>.yml`:

```yaml
http:
  routers:
    arcane:
      rule: "Host(`arcane.example.com`)"
      entryPoints:
        - websecure
      service: arcane
      tls:
        certResolver: cloudflare
  services:
    arcane:
      loadBalancer:
        servers:
          - url: "http://192.168.1.100:3552"
```

The host IP is auto-detected from the Supervisor network API. The port is the app's configured host-mapped port.

## Lifecycle

- **App start**: Config file is generated during initialization
- **App stop**: Config file is removed (Traefik stops routing immediately)
- **App restart**: Config file is regenerated (Traefik hot-reloads)
- **Traefik restart**: Traefik re-reads all files in the directory on startup

## Troubleshooting

### Config file not generated
- Check the app logs for Traefik-related messages
- Verify `traefik_enable` is `true` and `traefik_domain` is set
- Ensure the app has `share:rw` in its volume mappings

### Traefik not picking up changes
- Verify `watch: true` is set in your Traefik File Provider config
- Check the volume mount path is correct
- Check Traefik logs for file provider errors

### Wrong host IP in generated config
- The app auto-detects the LAN IP via the Supervisor network API
- If it detects a Docker bridge IP (172.x), check your HAOS network configuration
- You can verify the generated config at `/share/traefik/dynamic/<slug>.yml`

### WebSocket connections failing
- Traefik handles WebSocket upgrades natively on HTTP routers — no special middleware needed
- Verify the app's port is correct in the generated config

## Apps with Traefik Support

| App | Default Port | Slug |
|-----|-------------|------|
| Arcane | 3552 | `arcane` |
| Dockge | 5001 | `dockge` |
| Dockhand | 3000 | `dockhand` |
| Huly | 4859 | `huly` |
| MuninnDB | 8476 (+ API/gRPC/MCP) | `muninndb` |
| Portainer EE LTS | 9000/9443 | `portainer_ee_lts` |
| Portainer EE STS | 9000/9443 | `portainer_ee_sts` |

The HAY CM5 Fan Controller has no web UI and does not support Traefik.
