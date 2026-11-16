#!/usr/bin/env bash
# Smoke test of the built launcher in the session it ships with, on Linux arm64 (a Lima VM through run.sh, or CI's arm64 runner).
#
#   test/vm/smoke.sh <bin> <out>
#
# <bin> holds mygo-pocket (built -tags e2e) and mygo-automation (build.sh). dist/sway.conf runs on sway's headless backend, its output
# made the Flip 2's portrait 1080x1920 panel turned 270°. Presses go through the launcher's MYGO_POCKET_CTL FIFO, what it shows is read
# through MyGo's automation plugin, and real child processes start from card directories. Screens and logs land in <out>.
set -euo pipefail

bin=$(realpath "$1")
mkdir -p "$2"
out=$(realpath "$2")
repo=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d)
export XDG_RUNTIME_DIR=$work/run
mkdir -m 700 "$XDG_RUNTIME_DIR"
export PATH=$bin:$PATH MYGO_AUTOMATION_SESSION=smoke LIBGL_ALWAYS_SOFTWARE=1
step=0
sway_pid=

fail() {
	echo "FAIL: $*" >&2
	grim "$out/failed.png" 2>/dev/null || true
	for f in "$out"/launcher.log "$out"/sway.log; do [ -f "$f" ] && { echo "--- $f"; tail -40 "$f"; }; done >&2
	exit 1
}
cleanup() { [ -n "$sway_pid" ] && kill "$sway_pid" 2>/dev/null; rm -rf "$work"; }
trap cleanup EXIT

# waitfor <what> <command...>: until the command succeeds, for 20 s
waitfor() {
	local what=$1
	shift
	for _ in $(seq 100); do
		"$@" >/dev/null 2>&1 && return 0
		sleep 0.2
	done
	fail "$what"
}

shot() {
	step=$((step + 1))
	grim "$out/$(printf %02d $step)-$1.png"
}

press() { printf '%s\n' "$@" > "$work/ctl"; }

shows() { mygo-automation snapshot > "$work/snap" 2>&1 && grep -qF -- "\"$1\"" "$work/snap"; }
expect() { waitfor "\"$1\" on the launcher; it showed: $(cat "$work/snap" 2>/dev/null)" shows "$1"; }
# idle: no game runs (the footer stays in the tree under the overlay, so it can't tell)
idle() {
	for _ in $(seq 100); do
		shows "正在运行" || return 0
		sleep 0.2
	done
	fail "the launcher still shows a game running"
}

# tree_has <jq filter>: a node of sway's tree matches
tree_has() { swaymsg -t get_tree | jq -e "[.. | objects | select($1)] | length > 0"; }

# ---- the cards: one of each kind that runs here ----
games=$work/games
card() { mkdir -p "$games/$1" && cat > "$games/$1/${2:-pocket.json}"; }
card pack nx2a-data.json <<< '{"name": "Pack"}'
printf '#!/bin/sh\necho "$DATA" > "$DATA/ran"\n' > "$games/pack/dmhost"
printf '#!/bin/sh\nexec "$DATA/dmhost"\n' > "$games/launch.sh"
card env <<< '{"kind": "native", "title": "Env", "exec": ["run.sh"], "env": {"FOO": "bar"}, "order": 1}'
printf '#!/bin/sh\necho "$PWD $GAME_DIR $FOO" > ran\n' > "$games/env/run.sh"
card ra <<< '{"kind": "command", "title": "RetroArch", "exec": ["retroarch"], "order": 2}'
card gb <<< '{"kind": "retroarch", "title": "Game Boy", "core": "gambatte", "rom": "game.gb", "order": 3}'
chmod +x "$games"/launch.sh "$games"/*/dmhost "$games"/*/run.sh
export MYGO_POCKET_GAMES=$games MYGO_AUTOMATION=$work/automation.sock RETROARCH_CORES=$(dirname "$(find /usr/lib -name gambatte_libretro.so | head -1)")

# ---- the session: dist/sway.conf with the panel and paths of this machine ----
{
	echo 'output HEADLESS-1 mode --custom 1080x1920@60Hz'
	sed -e 's/DSI-1/HEADLESS-1/g' -e '/mako/d' -e '/allow_tearing/d' \
		-e "s#/storage/mygo-pocket/mygo-pocket #$bin/mygo-pocket #" \
		-e "s#/run/mygo-pocket.ctl#$work/ctl#" \
		-e "s#/storage/mygo-pocket/launcher.log#$out/launcher.log#" "$repo/dist/sway.conf"
} > "$work/sway.conf"
WLR_BACKENDS=headless WLR_RENDERER=pixman WLR_LIBINPUT_NO_DEVICES=1 dbus-run-session -- sway -c "$work/sway.conf" > "$out/sway.log" 2>&1 &
sway_pid=$!
waitfor "sway's socket" sh -c 'ls "$XDG_RUNTIME_DIR"/sway-ipc.*.sock'
SWAYSOCK=$(ls "$XDG_RUNTIME_DIR"/sway-ipc.*.sock)
WAYLAND_DISPLAY=$(basename "$(ls "$XDG_RUNTIME_DIR"/wayland-* | grep -v lock | head -1)")
export SWAYSOCK WAYLAND_DISPLAY

echo "== the launcher fills the turned panel"
waitfor "the launcher's window" tree_has '.name == "mygo-pocket" and .fullscreen_mode == 1 and .rect.width == 1920 and .rect.height == 1080'
waitfor "the control FIFO" test -p "$work/ctl"
waitfor "the automation socket" test -S "$MYGO_AUTOMATION"
mygo-automation open > /dev/null
expect "Pack"
expect "SWITCH · NX2LINUX"
expect "Game Boy"
shot games

echo "== cards: left/right choose, a retroarch card finds its core"
press right right right
expect "RETRO · GAMBATTE"
press left left left
expect "SWITCH · NX2LINUX"

echo "== an nx2linux card runs launch.sh with its pack"
press a
waitfor "the pack's dmhost" test -f "$games/pack/ran"
[ "$(cat "$games/pack/ran")" = "$games/pack" ] || fail "dmhost saw DATA=$(cat "$games/pack/ran")"

echo "== a native card runs in its directory with its environment"
idle
press "game env" a
waitfor "the env card" test -f "$games/env/ran"
[ "$(cat "$games/env/ran")" = "$games/env $games/env bar" ] || fail "the env card saw $(cat "$games/env/ran")"

echo "== RetroArch goes fullscreen above the launcher, which reads no presses until it ends"
idle
press "game ra" a
waitfor "RetroArch's window" tree_has '((.app_id // "") | test("retroarch"; "i")) and .fullscreen_mode == 1'
expect "正在运行"
sleep 2
shot retroarch
press right r
pkill -x retroarch
idle
expect "←→ 选择    A 开始    L/R 设置"
expect "RetroArch"
expect "SYSTEM"

echo "== settings: a value changes only after A"
press r
expect "↑↓ 分类    → / A 进入    B 游戏    L/R 游戏"
press "find 显示 界面缩放" left
expect "1.5x"
press "find 显示 界面缩放" a
expect "←→ 调节    A / B 完成"
press right
expect "1.75x"
shot zoom
press b left
expect "↑↓ 分类    → / A 进入    B 游戏    L/R 游戏"

echo "== a tap on a settings category"
ref=$(mygo-automation snapshot | grep -F '"电源"' | grep -o '@e[0-9]*' | head -1)
[ -n "$ref" ] || fail "no 电源 category"
mygo-automation click "$ref" > /dev/null
expect "重启"
shot power

echo "== the session ends with the launcher"
pkill -x mygo-pocket
waitfor "sway to exit" sh -c "! kill -0 $sway_pid"
sway_pid=
if grep -E 'panic|fatal error' "$out/launcher.log"; then fail "the launcher panicked"; fi
echo "PASS: screens in $out"
