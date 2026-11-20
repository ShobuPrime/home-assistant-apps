#!/usr/bin/env bash
# ==============================================================================
# Moves Arcane's stored directory settings off /opt/arcane/data.
#
# Until early 2026 this app ran Arcane from /opt/arcane. Arcane turned its
# relative default into /opt/arcane/data/projects and stored that. The app has
# since run Arcane from /data/arcane as an unprivileged user, so the stored
# path can be neither created nor kept: the log reads "mkdir /opt/arcane/data:
# permission denied" and creating a project fails.
#
# The same folder under /app/data is where Arcane keeps it today. Only values
# under /opt/arcane/data are changed. Any other value is somebody's choice.
#
# LEGACY UNTIL 2027-06-01: then delete this file, its call in
# cont-init.d/arcane.sh, sqlite in the Dockerfile, tests/migrate-settings-test.sh
# and the migration block in smoke-test.sh. DOCS.md says how to set the path
# by hand.
#
# Usage: migrate-settings.sh <database>     (run as Arcane's user)
# ==============================================================================
set -u

DB="${1:?usage: migrate-settings.sh <database>}"
OLD_ROOT="/opt/arcane/data"
NEW_ROOT="/app/data"

# A new install has no database yet.
[ -f "${DB}" ] || exit 0

sql() { sqlite3 -batch -bail -cmd '.timeout 5000' "${DB}" "$1"; }

# Arcane must start whatever happens here.
give_up() {
    echo "Stored directory settings were not checked: $1"
    exit 0
}

columns=$(sql "select count(*) from pragma_table_info('settings') where name in ('key', 'value');" 2>&1) \
    || give_up "${columns}"
[ "${columns}" = "2" ] || exit 0

stale=$(sql "select key || char(9) || value from settings where value = '${OLD_ROOT}' or value like '${OLD_ROOT}/%';" 2>&1) \
    || give_up "${stale}"
[ -n "${stale}" ] || exit 0

# Copies what is under $1 to $2 and never replaces a file that is already
# there. Not `cp -n`: busybox skips a folder that exists instead of filling it.
copy_missing() {
    ( cd "$1" && find . -mindepth 1 ) | while IFS= read -r entry; do
        from="$1/${entry#./}"
        to="$2/${entry#./}"
        if [ -d "${from}" ] && [ ! -L "${from}" ]; then
            mkdir -p "${to}" || exit 1
        elif [ ! -e "${to}" ] && [ ! -L "${to}" ]; then
            cp -a "${from}" "${to}" || exit 1
        fi
    done
}

# Files move before the setting does. In this app there are none: the old
# folder was part of the container, which is rebuilt on every start.
if [ -d "${OLD_ROOT}" ] && [ -n "$(ls -A "${OLD_ROOT}" 2>/dev/null)" ]; then
    { mkdir -p "${NEW_ROOT}" && copy_missing "${OLD_ROOT}" "${NEW_ROOT}"; } \
        || give_up "could not copy ${OLD_ROOT} to ${NEW_ROOT}"
    echo "Copied what was in ${OLD_ROOT} to ${NEW_ROOT}."
fi

sql "update settings set value = '${NEW_ROOT}' || substr(value, ${#OLD_ROOT} + 1) where value = '${OLD_ROOT}' or value like '${OLD_ROOT}/%';" >/dev/null 2>&1 \
    || give_up "could not update the settings table"

while IFS=$'\t' read -r key value; do
    echo "Arcane setting ${key} moved from ${value} to ${NEW_ROOT}${value#"${OLD_ROOT}"}."
done <<< "${stale}"
