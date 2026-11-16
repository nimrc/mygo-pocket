#!/bin/bash
# mygo-pocket.service: the launcher's sway session, with ROCKNIX's environment as sway.service and EmulationStation get it.
. /etc/profile
export HOME=/storage XDG_RUNTIME_DIR=/var/run/0-runtime-dir WLR_BACKENDS=drm,libinput WLR_LIBINPUT_NO_DEVICES=1 WLR_OUTPUT_ROTATE_PLANE=1 SDL_AUDIODRIVER=pulseaudio
mkdir -p "$XDG_RUNTIME_DIR"
[ -S "$XDG_RUNTIME_DIR/bus" ] || dbus-daemon --session --address="unix:path=$XDG_RUNTIME_DIR/bus" &
# sway.conf is written for the Flip 2 (DSI-1 turned 270°): take the panel and its turn as ROCKNIX's 111-sway-init does, from fbcon's rotation
con=DSI-1 angle=270
for s in /sys/class/drm/card*-DSI-*/status /sys/class/drm/card*-eDP-*/status; do
	[ "$(cat "$s" 2>/dev/null)" = connected ] || continue
	c=${s%/status}
	con=${c##*/card?-}
	break
done
case $(cat /sys/class/graphics/fbcon/rotate 2>/dev/null) in 0) angle=0 ;; 1) angle=90 ;; 2) angle=180 ;; 3) angle=270 ;; esac
sed -e "s/DSI-1/$con/g" -e "s/transform 270/transform $angle/" /storage/mygo-pocket/sway.conf > "$XDG_RUNTIME_DIR/mygo-pocket-sway.conf"
# Panfrost (the RG ARC's Mali-G52, ROCKNIX's gpudriver panfrost) has desktop OpenGL 3.1 only, under the 3.3 MyGo draws with: GTK takes
# OpenGL ES 3.1 then, or the launcher draws on the CPU
[ -d /sys/module/panfrost ] && export GDK_GL=gles
exec sway -c "$XDG_RUNTIME_DIR/mygo-pocket-sway.conf" > /storage/mygo-pocket/sway.log 2>&1
