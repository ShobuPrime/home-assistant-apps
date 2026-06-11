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

Create as `rootfs/etc/nginx/nginx.conf` or generate in init script:

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
