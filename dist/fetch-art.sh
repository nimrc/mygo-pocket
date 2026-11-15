#!/bin/sh
# fetch-art.sh <game dir>...: each card's Steam library capsule (600x900) as <dir>/cover.jpg, from the steam_appid in its pocket.json.
# They are the publishers' art: fetched onto the device, not kept in git.
for dir in "$@"; do
  app=$(sed -n 's/.*"steam_appid" *: *\([0-9]*\).*/\1/p' "$dir/pocket.json" 2>/dev/null)
  [ -n "$app" ] || { echo "$dir: no steam_appid"; continue; }
  for base in https://shared.cloudflare.steamstatic.com/store_item_assets/steam/apps https://cdn.cloudflare.steamstatic.com/steam/apps; do
    curl -fsSL "$base/$app/library_600x900.jpg" -o "$dir/cover.jpg" && echo "$dir: $app" && break
  done
done
