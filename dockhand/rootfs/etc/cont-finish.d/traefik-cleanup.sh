#!/usr/bin/with-contenv bashio
# Remove Traefik config on app shutdown
APP_SLUG=$(bashio::app.slug)
rm -f "/share/traefik/dynamic/${APP_SLUG}.yml" 2>/dev/null || true
