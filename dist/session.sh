#!/bin/bash
# mygo-pocket.service: the launcher's sway session, with ROCKNIX's environment as sway.service and EmulationStation get it.
. /etc/profile
export HOME=/storage XDG_RUNTIME_DIR=/var/run/0-runtime-dir WLR_BACKENDS=drm,libinput WLR_LIBINPUT_NO_DEVICES=1 WLR_OUTPUT_ROTATE_PLANE=1 SDL_AUDIODRIVER=pulseaudio
mkdir -p "$XDG_RUNTIME_DIR"
[ -S "$XDG_RUNTIME_DIR/bus" ] || dbus-daemon --session --address="unix:path=$XDG_RUNTIME_DIR/bus" &
exec sway -c /storage/mygo-pocket/sway.conf > /storage/mygo-pocket/sway.log 2>&1
