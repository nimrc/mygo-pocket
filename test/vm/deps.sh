#!/bin/sh
# What smoke.sh needs on Ubuntu 24.04: sway with its headless backend, the GTK 3 and EGL that MyGo loads (Mesa's software renderer, as
# there is no GPU), RetroArch with a core, and CJK fonts for the screenshots.
set -eu
command -v sway grim jq retroarch >/dev/null 2>&1 && exit 0
export DEBIAN_FRONTEND=noninteractive
apt-get update -q
apt-get install -yq --no-install-recommends sway grim jq dbus libgtk-3-0t64 libepoxy0 libegl1 libegl-mesa0 libgl1-mesa-dri \
  mesa-vulkan-drivers retroarch libretro-gambatte fonts-noto-cjk fonts-dejavu-core
