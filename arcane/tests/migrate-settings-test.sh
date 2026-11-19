#!/usr/bin/env bash
# Tests rootfs/usr/share/arcane/migrate-settings.sh against small databases.
#
# Usage: migrate-settings-test.sh [path to migrate-settings.sh]
#
# Needs bash and sqlite3. The smoke test pipes it into the app's container,
# where the last case (files at the old place) can run too.
#
# LEGACY UNTIL 2027-06-01: goes with migrate-settings.sh.
set -u

SCRIPT="${1:-$(cd "$(dirname "$0")/.." && pwd)/rootfs/usr/share/arcane/migrate-settings.sh}"
T=$(mktemp -d)
trap 'rm -rf "${T}"' EXIT
fails=0

check() { # check <what> <got> <want>
    if [ "$2" = "$3" ]; then
        echo "  ok: $1"
    else
        echo "  FAIL: $1"
        echo "    got:  $2"
        echo "    want: $3"
        fails=$((fails + 1))
    fi
}

# The settings table as Arcane 3.1.0 creates it, in WAL mode like the real one.
newdb() {
    sqlite3 "$1" "CREATE TABLE settings (key TEXT NOT NULL PRIMARY KEY, value TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'string', isPublic BOOLEAN DEFAULT FALSE NOT NULL, isInternal BOOLEAN DEFAULT FALSE NOT NULL, createdAt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updatedAt DATETIME); PRAGMA journal_mode=WAL;" >/dev/null
}
put() { sqlite3 "$1" "insert or replace into settings (key, value) values ('$2', '$3');"; }
all() { sqlite3 "$1" "select key || '=' || value from settings order by key;" | tr '\n' ' '; }
run() { out=$(bash "${SCRIPT}" "$1" 2>&1); rc=$?; }

echo "no database yet"
run "${T}/none.db"
check "exit code" "${rc}" "0"
check "says nothing" "${out}" ""
check "creates no file" "$(ls "${T}")" ""

echo "an install from early 2026"
newdb "${T}/old.db"
put "${T}/old.db" projectsDirectory /opt/arcane/data/projects
put "${T}/old.db" diskUsagePath data/projects
put "${T}/old.db" templatesDirectory /app/data/templates
put "${T}/old.db" buildsDirectory /builds
run "${T}/old.db"
check "exit code" "${rc}" "0"
check "reports the move" "${out}" "Arcane setting projectsDirectory moved from /opt/arcane/data/projects to /app/data/projects."
check "only that row changed" "$(all "${T}/old.db")" "buildsDirectory=/builds diskUsagePath=data/projects projectsDirectory=/app/data/projects templatesDirectory=/app/data/templates "
check "still in WAL mode" "$(sqlite3 "${T}/old.db" 'pragma journal_mode;')" "wal"

echo "the same database again"
run "${T}/old.db"
check "exit code" "${rc}" "0"
check "says nothing" "${out}" ""
check "nothing changed" "$(all "${T}/old.db")" "buildsDirectory=/builds diskUsagePath=data/projects projectsDirectory=/app/data/projects templatesDirectory=/app/data/templates "

echo "paths somebody chose"
newdb "${T}/custom.db"
put "${T}/custom.db" projectsDirectory /data/projects
put "${T}/custom.db" templatesDirectory /mnt/stacks/templates
put "${T}/custom.db" diskUsagePath data/projects
put "${T}/custom.db" swarmStackSourcesDirectory /opt/arcane/database/sources
put "${T}/custom.db" buildsDirectory /opt/arcane/data2/builds
put "${T}/custom.db" projectsDirectories '["/srv/a:/opt/arcane/data/projects"]'
before=$(all "${T}/custom.db")
run "${T}/custom.db"
check "exit code" "${rc}" "0"
check "says nothing" "${out}" ""
check "nothing changed" "$(all "${T}/custom.db")" "${before}"

echo "other settings under the old folder"
newdb "${T}/more.db"
put "${T}/more.db" templatesDirectory /opt/arcane/data/templates
put "${T}/more.db" swarmStackSourcesDirectory /opt/arcane/data/swarm/sources
put "${T}/more.db" diskUsagePath /opt/arcane/data
run "${T}/more.db"
check "exit code" "${rc}" "0"
check "all three moved" "$(all "${T}/more.db")" "diskUsagePath=/app/data swarmStackSourcesDirectory=/app/data/swarm/sources templatesDirectory=/app/data/templates "
check "reports three moves" "$(printf '%s\n' "${out}" | grep -c '^Arcane setting .* moved from ')" "3"

echo "a database without a settings table"
sqlite3 "${T}/other.db" "create table something (a text);"
run "${T}/other.db"
check "exit code" "${rc}" "0"
check "says nothing" "${out}" ""

echo "a file that is not a database"
head -c 4096 /dev/urandom > "${T}/broken.db"
run "${T}/broken.db"
check "exit code" "${rc}" "0"
check "says it gave up" "$(printf '%s' "${out}" | head -1 | cut -c1-43)" "Stored directory settings were not checked:"

echo "files at the old place"
if [ -x /opt/arcane/arcane ] && [ -L /app/data ] && [ ! -e /opt/arcane/data ] && [ "$(id -u)" = "0" ]; then
    mkdir -p /opt/arcane/data/projects/zz-migrate-test-a /app/data/projects/zz-migrate-test-b /opt/arcane/data/projects/zz-migrate-test-b
    echo "from the old place" > /opt/arcane/data/projects/zz-migrate-test-a/compose.yaml
    echo "from the old place" > /opt/arcane/data/projects/zz-migrate-test-b/compose.yaml
    echo "already here" > /app/data/projects/zz-migrate-test-b/compose.yaml
    newdb "${T}/files.db"
    put "${T}/files.db" projectsDirectory /opt/arcane/data/projects
    run "${T}/files.db"
    check "exit code" "${rc}" "0"
    check "says it copied" "$(printf '%s\n' "${out}" | head -1)" "Copied what was in /opt/arcane/data to /app/data."
    check "file arrives" "$(cat /app/data/projects/zz-migrate-test-a/compose.yaml 2>&1)" "from the old place"
    check "existing file is kept" "$(cat /app/data/projects/zz-migrate-test-b/compose.yaml 2>&1)" "already here"
    check "setting moved" "$(all "${T}/files.db")" "projectsDirectory=/app/data/projects "
    rm -rf /opt/arcane/data /app/data/projects/zz-migrate-test-a /app/data/projects/zz-migrate-test-b
else
    echo "  skipped: runs as root inside the app's container only"
fi

if [ "${fails}" -eq 0 ]; then
    echo "migrate-settings: all checks passed"
else
    echo "migrate-settings: ${fails} check(s) failed"
    exit 1
fi
