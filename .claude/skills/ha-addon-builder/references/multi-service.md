# Multi-Service / Compose-Based App Patterns

This reference covers how to build Home Assistant apps that run multiple cooperating processes (e.g., app server + database + cache + reverse proxy). This is more complex than single-binary apps and requires careful handling of service dependencies, inter-process communication, and data isolation.

## Table of Contents

1. [When to Use Multi-Service](#when-to-use-multi-service)
2. [Architecture Options](#architecture-options)
3. [S6-Overlay Multi-Service Pattern](#s6-overlay-multi-service-pattern)
4. [Dockerfile for Multi-Service](#dockerfile-for-multi-service)
5. [Inter-Service Communication](#inter-service-communication)
6. [Database Services](#database-services)
7. [Reverse Proxy (nginx) Pattern](#reverse-proxy-nginx-pattern)
8. [Data Directory Layout](#data-directory-layout)
9. [Health Checks](#health-checks)
10. [Common Pitfalls](#common-pitfalls)

---

## When to Use Multi-Service

Use this pattern when:
- The application requires a database (PostgreSQL, MongoDB, CockroachDB, etc.)
- The application requires a search engine (Elasticsearch, Meilisearch, etc.)
- The application requires a cache (Redis, etc.)
- The application has multiple backend services that must run together
- The application needs a reverse proxy for routing (nginx, Caddy, etc.)

## Architecture Options

### Option A: All-in-One Container with S6 (Recommended)

Run all services inside a single HA app container using S6-overlay to manage multiple processes. This is the standard HA app approach.

**Pros:**
- Follows HA app conventions
- Single container to manage
- S6 handles service dependencies and restart
- Works with HA backup system

**Cons:**
- All services share the same container resources
- More complex Dockerfile (installing multiple packages)

### Option B: Docker Compose Inside Container

Run Docker Compose from within the app container. This requires `docker_api: true` and is more complex but allows using upstream Docker images directly.

**Pros:**
- Can use upstream Docker images as-is
- Closer to upstream's intended deployment
- Easier to keep services updated independently

**Cons:**
- Docker-in-Docker complexity
- More resource overhead
- Harder to debug
- May conflict with HA's own Docker management

**Recommendation:** Use Option A (S6 multi-service) unless the upstream application has many interdependent services that would be very difficult to install from source. Option B should be reserved for complex applications with 4+ tightly-coupled services.

---

## S6-Overlay Multi-Service Pattern

### Directory Structure

```
rootfs/
└── etc/
    ├── cont-init.d/
    │   ├── 00-init-data.sh          # Create directories, set permissions
    │   ├── 01-init-database.sh      # Initialize database
    │   └── 02-init-app.sh           # Configure application
    └── services.d/
        ├── database/
        │   ├── run                   # Start database
        │   └── finish                # Handle database exit
        ├── app/
        │   ├── run                   # Start main application
        │   └── finish                # Handle app exit
        └── nginx/                    # Optional reverse proxy
            ├── run
            └── finish
```

### Init Script Ordering

S6 runs cont-init.d scripts **in alphabetical order**. Use numeric prefixes to control execution order:

- `00-*` - Directory creation, permissions, environment setup
- `01-*` - Database initialization (must happen before the app can start)
- `02-*` - Application configuration (may need database to be initialized first)

### Service Dependencies

S6 starts all services simultaneously. If your app depends on the database being ready, the app's `run` script must wait for the database:

```bash
#!/usr/bin/with-contenv bashio
# ==============================================================================
# Wait for database to be ready before starting app
# ==============================================================================

bashio::log.info "Waiting for database..."

# Wait for PostgreSQL
for i in $(seq 1 30); do
    if pg_isready -h localhost -p 5432 -q 2>/dev/null; then
        bashio::log.info "Database is ready"
        break
    fi
    if [ $i -eq 30 ]; then
        bashio::log.error "Database not ready after 30 seconds"
        exit 1
    fi
    sleep 1
done

# Now start the application
exec /opt/app/server
```

For other databases:
- **MongoDB**: `mongosh --eval "db.runCommand({ping: 1})" --quiet`
- **Redis**: `redis-cli ping | grep -q PONG`
- **Elasticsearch**: `curl -sf http://localhost:9200/_cluster/health`
- **CockroachDB**: `cockroach sql --insecure -e "SELECT 1"`

---

## Dockerfile for Multi-Service

When installing multiple services, organize the Dockerfile into clear stages:

```dockerfile
ARG BUILD_FROM
FROM $BUILD_FROM

ARG APP_VERSION=1.0.0

# Always upgrade first
RUN apk upgrade --no-cache

# ============================================
# Install database (e.g., PostgreSQL)
# ============================================
RUN apk add --no-cache \
        postgresql16 \
        postgresql16-client

# ============================================
# Install cache (e.g., Redis)
# ============================================
RUN apk add --no-cache \
        redis

# ============================================
# Install reverse proxy (e.g., nginx)
# ============================================
RUN apk add --no-cache \
        nginx

# ============================================
# Install main application
# ============================================
RUN apk add --no-cache \
        ca-certificates \
        curl \
        jq \
    && mkdir -p /opt/app \
    && ARCH="$(uname -m)" \
    && if [ "${ARCH}" = "aarch64" ]; then \
        APP_ARCH="arm64"; \
    elif [ "${ARCH}" = "x86_64" ]; then \
        APP_ARCH="amd64"; \
    else \
        echo "Unsupported architecture: ${ARCH}"; \
        exit 1; \
    fi \
    && curl -L -f -S -o /tmp/app.tar.gz \
        "https://github.com/<owner>/<repo>/releases/download/v${APP_VERSION}/app-${APP_ARCH}.tar.gz" \
    && tar -xzf /tmp/app.tar.gz -C /opt/app \
    && rm /tmp/app.tar.gz

# Copy root filesystem (all S6 scripts)
COPY rootfs /

# Ensure ALL scripts are executable
RUN chmod a+x /etc/cont-init.d/*.sh \
    && chmod a+x /etc/services.d/*/run \
    && chmod a+x /etc/services.d/*/finish

# Standard build args and labels...
```

### Package Size Considerations

Alpine packages are small, but for large applications consider:
- PostgreSQL 16: ~15MB
- Redis: ~5MB
- nginx: ~3MB
- Elasticsearch: NOT available via apk - must download binary (~300MB+)
- MongoDB: NOT available via apk on Alpine - must download binary

For services not in Alpine's package repos, download binaries in the Dockerfile (same pattern as single-binary apps).

---

## Inter-Service Communication

All services run in the same container, so they communicate via **localhost**:

```bash
# Database connection
DATABASE_URL="postgresql://user:pass@localhost:5432/dbname"

# Redis connection
REDIS_URL="redis://localhost:6379"

# App server (for nginx to proxy)
APP_UPSTREAM="http://localhost:8080"
```

### Port Allocation

- **External ports** (exposed in config.yaml): Only the main web UI port and any required external ports
- **Internal ports** (not exposed): Database, cache, internal API ports stay internal

Example:
```yaml
# config.yaml
ports:
  8080/tcp: 8080     # Web UI (exposed)
# PostgreSQL 5432, Redis 6379 stay internal - NOT in ports list
```

---

## Database Services

### PostgreSQL Init Script

```bash
#!/usr/bin/with-contenv bashio
# 01-init-database.sh

DB_DATA="/data/<app>/postgresql"
DB_NAME="<app>"
DB_USER="<app>"

# Initialize PostgreSQL data directory if needed
if [[ ! -d "${DB_DATA}" ]]; then
    bashio::log.info "Initializing PostgreSQL database..."
    mkdir -p "${DB_DATA}"
    chown -R postgres:postgres "${DB_DATA}"
    su - postgres -c "initdb -D ${DB_DATA}"

    # Start temporarily to create user/database
    su - postgres -c "pg_ctl start -D ${DB_DATA} -l /tmp/pg_init.log -w"

    su - postgres -c "createuser ${DB_USER}" || true
    su - postgres -c "createdb -O ${DB_USER} ${DB_NAME}" || true

    # Set password if configured
    if bashio::config.has_value 'db_password'; then
        DB_PASS=$(bashio::config 'db_password')
        su - postgres -c "psql -c \"ALTER USER ${DB_USER} PASSWORD '${DB_PASS}';\""
    fi

    su - postgres -c "pg_ctl stop -D ${DB_DATA} -m fast -w"
    bashio::log.info "PostgreSQL initialized"
else
    bashio::log.info "PostgreSQL data directory exists"
fi
```

### PostgreSQL Run Script

```bash
#!/usr/bin/with-contenv bashio
# services.d/database/run

DB_DATA="/data/<app>/postgresql"

bashio::log.info "Starting PostgreSQL..."

# Run as postgres user
exec su - postgres -c "postgres -D ${DB_DATA} \
    -c listen_addresses=localhost \
    -c port=5432 \
    -c max_connections=50 \
    -c shared_buffers=128MB"
```

### Redis Run Script

```bash
#!/usr/bin/with-contenv bashio
# services.d/redis/run

bashio::log.info "Starting Redis..."

exec redis-server \
    --bind 127.0.0.1 \
    --port 6379 \
    --dir /data/<app>/redis \
    --maxmemory 64mb \
    --maxmemory-policy allkeys-lru
```

---

## Reverse Proxy (nginx) Pattern

If the application needs nginx as a reverse proxy (e.g., for routing multiple services, WebSocket handling, or serving static files):

### nginx Configuration Template

Create as `rootfs/etc/nginx/<purpose>.conf` — **not** `nginx.conf` — and run it with
`exec nginx -c /etc/nginx/<purpose>.conf`. Do not overwrite or extend Alpine's stock
`/etc/nginx/nginx.conf`: it already defines `$connection_upgrade` (a second `map` for the same
variable is a fatal duplicate) and sets `client_max_body_size 1m`, which 413s image builds and
large uploads. A standalone config also survives base-image updates, since nothing is patched.
The live example is `portainer_ee_lts/rootfs/etc/nginx/docker-shim.conf` (PR #195).


```nginx
worker_processes auto;
error_log /dev/stderr warn;
pid /var/run/nginx.pid;

events {
    worker_connections 1024;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    access_log /dev/stdout;

    sendfile on;
    keepalive_timeout 65;

    upstream app {
        server 127.0.0.1:8080;
    }

    server {
        listen <ingress_port>;

        location / {
            proxy_pass http://app;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;

            # WebSocket support (if needed)
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
        }
    }
}
```

### Important: Guard Against Directory Collision

Docker auto-creates directories for missing bind-mount file sources. If nginx.conf might not exist yet, guard against a stale directory:

```bash
# In init script
NGINX_CONF="/etc/nginx/nginx.conf"
if [[ -d "${NGINX_CONF}" ]]; then
    # Docker created a directory instead of a file - remove it
    rm -rf "${NGINX_CONF}"
fi
# Now write the config file
cat > "${NGINX_CONF}" << 'NGINX_EOF'
# ... config content ...
NGINX_EOF
```

---


### nginx `run` and `finish` Scripts

```bash
#!/usr/bin/with-contenv bashio
# services.d/<name>/run

bashio::log.info 'Starting nginx...'

# nginx chmods its unix listen sockets to 0666 unconditionally, so a 0700
# parent directory is the only thing restricting who can reach them. cont-init
# creates it too; repeating it here keeps a bare service restart self-sufficient.
mkdir -p /run/<name>
chmod 700 /run/<name>

# MANDATORY for a unix listener. nginx removes its socket only on a clean
# shutdown and never unlinks a pre-existing one, so a SIGKILL, an OOM kill on a
# memory-tight CM5, or an unclean container stop leaves the path behind and
# every S6 respawn then dies with "bind() ... (98: Address in use)" — forever,
# because /run in these containers is the image layer, not a tmpfs. The failure
# is invisible: the app keeps serving its own port, so the Dockerfile
# HEALTHCHECK and the config.yaml watchdog both stay green while the proxy is
# gone. Measured before this line existed in portainer_ee_lts: nginx dead,
# Docker API returning 000, container still reported "healthy".
rm -f /run/<name>/<name>.sock

exec nginx -c /etc/nginx/<purpose>.conf
```

```bash
#!/usr/bin/with-contenv bashio
# services.d/<name>/finish

if [[ "${1}" -ne 0 ]] && [[ "${1}" -ne 256 ]]; then
    bashio::log.warning "nginx crashed with exit code ${1}. Respawning..."
    # nginx exits immediately on a bad config, so S6 would otherwise respawn it
    # about once a second and bury the rest of the log. Back off — but stay
    # well inside S6_KILL_FINISH_MAXTIME (5s), or a crash-looping proxy slows
    # the whole container's shutdown down with it.
    sleep 2
fi
```

Alpine's nginx binary is `/usr/sbin/nginx`, which is not covered by the
`/bin/** ix,` and `/usr/bin/** ix,` rules the app profiles carry, so adding
nginx means adding `/usr/sbin/** ix,` to `apparmor.txt`. (A bare `file,` rule
does grant exec — measured — but every profile here lists its exec paths
explicitly, and HAOS's kernel has diverged from CI's on AppArmor before.)
### Proxying the Docker API (`docker_api: true`)

dockerd rejects any `POST /containers/{id}/start` whose body length is unknown
with `starting container with non-empty request body was deprecated since API
v1.22 and removed in v1.24`. The check runs in dockerd's HTTP handler before it
looks at the container, so the 400 names none and *masks* the real start error.
Requests arriving through Home Assistant's streaming ingress are chunked, so an
app that forwards the transfer encoding it received hits this on every start.
HAOS's dockerd is not configurable, so the app layer is the only place to fix
it. `portainer_ee_lts`/`portainer_ee_sts` do it with an nginx shim
(`rootfs/etc/nginx/docker-shim.conf` + `services.d/docker-shim/`); any app that
proxies the Docker API to a browser needs the same treatment.

- **Strip the body only where a body is illegal** —
  `^/(?:v[0-9.]+/)?containers/[^/]+/(?:start|stop|restart|kill|pause|unpause|wait)$`,
  with `proxy_pass_request_body off;` and `proxy_set_header Content-Length "";`.
  `/exec/{id}/start` carries a real JSON body and `…/attach` needs the upgrade
  tunnel; stripping either breaks the console with no error anywhere.
- **Pass everything else verbatim** — a `proxy_pass` with no URI part is what
  makes nginx forward the request URI exactly as it arrived, with no
  normalisation or re-encoding.
- **Workers must run as root** (`user root;`) — the socket is `root:docker`
  0660 and the `nginx` user gets EACCES, which surfaces as every request 502ing.
- **Redirect the path the app already uses; do not point the app at a new one.**
  Portainer stores its environment address in BoltDB and ignores `--host` once
  an environment exists, so a new socket path fixes nothing for existing
  installs and would force everyone to re-create their environment, orphaning
  their stacks. cont-init instead replaces the base image's `/var/run -> /run`
  symlink with a real directory holding
  `docker.sock -> /run/docker-shim/docker.sock`. That is safe only because
  s6-overlay's `preinit` restores the symlink before stage0 on every boot, and
  the one base-image writer under `/var/run` (`base-addon-log-level`) is ordered
  ahead of `legacy-cont-init`. The base sets `S6_BEHAVIOUR_IF_STAGE2_FAILS=2`,
  so adding another cont-init script or service that writes under `/var/run/`
  would not degrade the app — it would stop the container booting.

`.github/scripts/smoke-test.sh` enforces the whole contract: it reproduces the
400 on the raw socket, requires 304 through the shim on both the versioned and
bare API paths, starts and stops a real container, runs a hijacked exec, pushes
a 2 MB body, and SIGKILLs nginx to prove the socket unlink brings it back. Full
rationale: root `CLAUDE.md` § "dockerd rejects Docker API calls that arrive
through HA's ingress" and `portainer_ee_lts/CLAUDE.md`.
A sibling that listens on a **unix socket** is the same problem with a
different probe — and usually a different failure policy. `portainer_ee_lts`'s
Portainer service waits for the nginx shim's socket, then degrades instead of
dying if it never turns up:

```bash
# Wait for the socket the sibling service binds (100ms x 300 = 30s)
declare -i waited=0
while [[ ! -S /run/docker-shim/docker.sock ]] && (( waited < 300 )); do
    sleep 0.1
    waited=$((waited + 1))
done

if [[ ! -S /run/docker-shim/docker.sock ]]; then
    bashio::log.error "Docker socket shim did not start within 30s — falling back to the raw Docker socket."
    ln -sfn /run/docker.sock /var/run/docker.sock
fi
```

Prefer this shape wherever a degraded mode exists: `exit 1` puts the service
into an S6 respawn loop, and an app that starts with one feature broken beats
an app that never starts at all. Poll in fractions of a second — 1s granularity
adds latency to every boot, not just the slow ones.

## Data Directory Layout

For multi-service apps, organize persistent data under `/data/<app>/`:

```
/data/<app>/
├── postgresql/          # Database data files
├── redis/               # Redis persistence
├── elasticsearch/       # Search index data
├── app/                 # Application data, config, uploads
├── logs/                # Application logs (optional)
└── .secrets             # Generated secrets (chmod 600)
```

Init script should create all directories:

```bash
#!/usr/bin/with-contenv bashio
# 00-init-data.sh

APP_DATA="/data/<app>"

bashio::log.info "Creating data directories..."
mkdir -p "${APP_DATA}/postgresql"
mkdir -p "${APP_DATA}/redis"
mkdir -p "${APP_DATA}/app"

# Database dirs need specific ownership
chown -R postgres:postgres "${APP_DATA}/postgresql"
chmod 700 "${APP_DATA}/postgresql"

# Redis
chmod 755 "${APP_DATA}/redis"

# App data
chmod 755 "${APP_DATA}/app"
```

---

## Health Checks

For multi-service apps, the watchdog should check the main web UI port:

```yaml
# config.yaml
watchdog: tcp://[HOST]:[PORT:<main-port>]/health
```

But the init script should also verify individual services:

```bash
# In the app's run script, before exec
bashio::log.info "Verifying services..."

# Check database
if ! pg_isready -h localhost -p 5432 -q; then
    bashio::log.error "PostgreSQL is not ready"
    exit 1
fi

# Check Redis
if ! redis-cli -h localhost ping | grep -q PONG; then
    bashio::log.error "Redis is not ready"
    exit 1
fi

bashio::log.info "All services ready, starting application"
```

---

## Common Pitfalls

### 1. Service Start Order

S6 starts all services simultaneously. Your app MUST wait for its dependencies. Don't assume the database is ready just because its service is defined.

### 2. Filesystem Permissions on HAOS

Home Assistant OS mounts `/data` with specific ownership. Services like PostgreSQL and Elasticsearch that need specific user ownership must `chown` their directories in the init script.

### 3. Elasticsearch User ID

Elasticsearch requires running as a non-root user (UID 1000). In the S6 service script:

```bash
exec s6-setuidgid 1000:1000 /opt/elasticsearch/bin/elasticsearch
```

Or create a dedicated user in the Dockerfile:

```dockerfile
RUN adduser -D -u 1000 elasticsearch
```

### 4. Memory Limits

HA apps don't have memory limits by default, but services like Elasticsearch and PostgreSQL can consume a lot of memory. Configure conservative defaults:

- PostgreSQL `shared_buffers`: 128MB
- Redis `maxmemory`: 64MB
- Elasticsearch heap: 256MB-512MB (`-Xms256m -Xmx512m`)

The target hardware (HA Yellow with CM5, 16GB RAM) has plenty of room, but be respectful of shared resources.

### 5. Signal Handling

Each service's `run` script MUST use `exec` so the process receives signals directly from S6. Without `exec`, S6 sends signals to the shell, which may not forward them properly, leading to zombie processes.

### 6. Graceful Shutdown

For databases, the `finish` script should handle graceful shutdown:

```bash
#!/usr/bin/with-contenv bashio
# services.d/database/finish

if [[ "${1}" -ne 0 ]] && [[ "${1}" -ne 256 ]]; then
    bashio::log.warning "Database crashed with exit code ${1}"
    # Give database time to recover
    sleep 2
fi
```

### 7. Log Output

All services should log to stdout/stderr so logs appear in the HA app log viewer. Avoid writing to log files unless necessary. If a service insists on file logging, consider:

```bash
exec /opt/app/server 2>&1
```

Or redirect a log file to stdout:

```bash
tail -f /var/log/app.log &
exec /opt/app/server
```
