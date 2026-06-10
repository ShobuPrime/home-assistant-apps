#!/usr/bin/with-contenv bashio
# Remove Traefik config on addon shutdown
ADDON_SLUG=$(bashio::app.slug)
rm -f "/share/traefik/dynamic/${ADDON_SLUG}.yml" 2>/dev/null || true
