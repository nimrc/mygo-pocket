#!/bin/sh
# fetch-art.sh <dir>: the games' Steam library capsules (600x900) as <dir>/<id>.jpg, for the launcher's cards (art/ beside nx2l-launcher).
# They are the publishers' art: fetched for the device, not kept in git.
dir=${1:-art}; mkdir -p "$dir"
for g in silksong:1030300 hollowknight:367520 hades2:1145350 retroarch:1118310; do
  id=${g%%:*} app=${g#*:}
  for base in https://shared.cloudflare.steamstatic.com/store_item_assets/steam/apps https://cdn.cloudflare.steamstatic.com/steam/apps; do
    curl -fsSL "$base/$app/library_600x900.jpg" -o "$dir/$id.jpg" && break
  done
done
