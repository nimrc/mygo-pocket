# mygo-pocket

A handheld launcher built on [MyGo](https://github.com/egoist/mygo). It replaces ROCKNIX's EmulationStation frontend with a
fullscreen MyGo native UI in a sway session of its own, and it runs MyGo apps as cards next to games. It is developed on a Retroid Pocket
Flip 2 (Snapdragon 865, ROCKNIX).

![Games](docs/screenshots/games.png)

| Performance | Display |
| --- | --- |
| ![Performance](docs/screenshots/performance.png) | ![Display](docs/screenshots/display.png) |

- **Liquid Glass UI** after ColorOS 16 ([suqi8/coui](https://github.com/suqi8/coui)): MyGo's `glass` material, a dark scheme, and
  lights on the wallpaper that follow the chosen game's color. Game cards are 2:3 posters.
- **Cards come from directories.** Nothing is hard-coded. Each directory with a `pocket.json` is one card, so to install a game on another
  handheld you copy its directory over (see [Cards](#cards)).
- **MyGo ecosystem**: a MyGo app is a card of kind `mygo`. Its window opens fullscreen above the launcher in the same sway session. The
  launcher is itself a MyGo app.
- **System settings**:
  - Wi-Fi (scan, join with an on-screen keyboard, forget) and Bluetooth (pair, connect, forget).
  - Brightness, volume and UI zoom (1–2×, default 1.5×, relative to a 1080p panel).
  - Analog stick lights: off, battery level or a color, at any brightness. ROCKNIX's battery mode always lights them at full
    brightness, so the launcher holds the chosen brightness every 2 s, also while a game runs.
  - CPU and GPU governors, fan profile, battery details, time zone.
  - SSH, Samba and Syncthing on/off; reboot and power off.

  These go through ROCKNIX's own tools and `set_setting`, so they hold across reboots.
- **Gamepad and touch**: d-pad, A, B, X and L/R, or a tap. A finger drags a slider; this uses the `TouchPointer` window option from
  [nimrc/mygo](https://github.com/nimrc/mygo), which `go.mod` replaces MyGo with.

## Cards

The launcher scans every directory under `$MYGO_POCKET_GAMES`, which is colon-separated and defaults to
`/storage/games:/storage/nx2linux`. A directory is a card when it holds a `pocket.json`
([schema](schema/pocket.schema.json)) or an [nx2linux](https://github.com/nimrc/nx2linux) data pack (`nx2a-data.json`). The directory's
name is the card's id. The launcher rescans every 30 s, so a copied directory shows up without a restart.

| `kind` | What runs | Required |
| --- | --- | --- |
| `nx2linux` | A Switch port: `launch.sh` at the root runs the pack's `dmhost`. This kind is the default when `nx2a-data.json` is present; the card shows once `dmhost` is in place, so a pack being copied in is not a card yet. | — |
| `mygo` | A MyGo app in the directory; `exec[0]` is relative to it. | `title`, `exec` |
| `native` | Any other program in the directory (an SDL game, a script). | `title`, `exec` |
| `command` | A program on `PATH`, such as `retroarch`. | `title`, `exec` |
| `retroarch` | One retro game: RetroArch runs `core` on `rom`. | `title`, `core`, `rom` |

```json
{
  "$schema": "https://raw.githubusercontent.com/nimrc/mygo-pocket/main/schema/pocket.schema.json",
  "kind": "retroarch",
  "title": "My SNES Game",
  "core": "snes9x",
  "rom": "game.sfc",
  "color": "#047857",
  "order": 50
}
```

- **Shared fields**: every kind also takes `subtitle`, `tag`, `color`, `cover`, `env`, `order` and `hidden`.
- **Environment**: every card runs in its own directory, and `GAME_DIR` is set to that directory.
- **RetroArch cores**: the launcher looks them up in `$RETROARCH_CORES`, then `/tmp/cores`, `/storage/.config/retroarch/cores` and
  `/usr/lib/libretro`.
- **More examples**: see [`examples/`](examples).

**Covers.** The poster is `cover.jpg` or `cover.png` in the card's directory. `dist/fetch-art.sh <dir>...` downloads the Steam library
capsule (600×900) for each card's `steam_appid`. Covers are the publishers' art: keep them on the device, out of git.

## Install on ROCKNIX

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o mygo-pocket .
ssh root@handheld mkdir -p /storage/mygo-pocket
scp mygo-pocket dist/* root@handheld:/storage/mygo-pocket/
ssh root@handheld /storage/mygo-pocket/pocketctl enable   # at boot, instead of EmulationStation
ssh root@handheld /storage/mygo-pocket/pocketctl restart
```

- **`pocketctl enable`**: installs `mygo-pocket.service` as ROCKNIX's `UI_SERVICE`. It also keeps EmulationStation from starting:
  `emustation.service` is masked, and `essway.service` gets a drop-in whose condition never holds. `/usr` is read-only, so this is how
  EmulationStation is removed.
- **`pocketctl disable`**: puts ROCKNIX's defaults back.
- **Updating**: the running binary cannot be overwritten in place. Copy it as `mygo-pocket.new`, `mv` it over `mygo-pocket`, then
  `pocketctl restart`.
- **Another handheld**: `dist/sway.conf` is written for the Flip 2's panel (`DSI-1`, rotated 270°). `session.sh` swaps in the device's
  own DSI or eDP panel and the rotation ROCKNIX gives it, so the same files run on an RG ARC (640×480) too. 界面缩放 is relative to a
  1080p panel: the view's zoom follows the panel's short side.
- **Small panels** (under 720 pixels on the short side, such as the RG ARC's 4-inch 640×480) get a compact layout: it lays out in about
  600×450 at the default 1.5×, so text is 1.5–1.7 times the size the 1080p layout gives it there; the games show three cards at a time,
  and the settings' categories are a row of icons above the rows. Nothing is glass there (see [Performance](#performance)).
- **Panfrost** (the RG ARC's Mali-G52 under ROCKNIX's `gpudriver panfrost`): `session.sh` sets `GDK_GL=gles`, as its desktop OpenGL
  is 3.1, under the 3.3 MyGo needs. On OpenGL ES the window is transparent, so that GTK composites it with GL instead of reading every
  frame back.
- **Logs**: `/storage/mygo-pocket/launcher.log` and `sway.log`.

## Controls

- **L/R**: switch between Games and Settings.
- **Games**: left/right choose a card, A starts it.
- **Settings**:
  - Up/down choose a category (left/right along the icons on a small panel); right or A enters it (down or A on a small panel).
  - Up/down choose a row. A acts on it, X does its second action (forget, for example), and left or B goes back.
  - A value (a choice or a slider) changes only after A: the row lights up, left/right change the value, and A or B is done. So left
    never changes a value by mistake when it was meant as back.
- **On-screen keyboard**: the d-pad and A type, X shifts, B erases, + accepts, − cancels.

A game runs as a child process, fullscreen above the launcher. The launcher reads no input until the game exits.

## Tests

```sh
go test ./...
MYGO_POCKET_SHOTS=shots go test ./...   # also saves the screens the tests pass through as PNGs
```

- **UI end to end** (`e2e_test.go`): `ui.Tester` draws the launcher in memory at the Flip 2's 1920×1080. The tests press buttons as the
  gamepad does and tap as a finger does. They run real games from temporary card directories and drive the `MYGO_POCKET_CTL` FIFO.
  ROCKNIX's tools are faked (`harness_test.go`): the tests check what the launcher asked of them, such as `set_setting` or
  `systemctl reboot`, and nothing on the machine changes.
- **Cards** (`games_test.go`): every kind, the directories that are not cards, and the examples and README manifest against the schema.

- **The session** (`test/vm/smoke.sh`): the linux/arm64 binary, built with MyGo's automation plugin (`-tags e2e`), runs in
  `dist/sway.conf` on sway's headless backend. The output is the Flip 2's 1080×1920 panel turned 270°. The test presses through the
  FIFO and reads the screen through `mygo-automation`. It also starts an nx2linux pack, a native app and RetroArch, which goes
  fullscreen above the launcher. When the launcher exits, the session must end.

  ```sh
  LIMA_HOME=/path/with/space test/vm/run.sh   # on a Mac: an Ubuntu 24.04 arm64 VM in Lima, made once; screens in build/vm/out
  ```

CI runs the Go tests on Linux (with `-race`) and macOS and the session on an arm64 runner (`ubuntu-24.04-arm`). It uploads the screens
from both.

## Testing on the device

`MYGO_POCKET_CTL=/run/mygo-pocket.ctl` is a FIFO that the launcher reads as input. It takes one of these per line:

- A button name: `up`, `down`, `left`, `right`, `a`, `b`, `x`, `l`, `r`, `start`, `back`.
- `find <category> <label>`: selects a settings row.
- `game <id>`: chooses a card on the games screen (then `a` starts it).
- `type <text>`: types into the open keyboard.

Combined with `grim` for screenshots, this drives every screen over SSH. `MYGO_POCKET_PPROF=<file>` writes a CPU profile of the first
20 s.

## Performance

On the Flip 2 at the games screen, the launcher and sway together use 151 MB RSS and 2% of one core. EmulationStation and sway use
217 MB and 7.8%. Moving between cards takes about 0.35 core.

The cards grow by changing their size, not by a transform. MyGo masks a clip or a rounded image under a transform on the CPU, which
took 1.4 cores.

The wallpaper's three lights are drawn into a 192-pixel-wide bitmap that the GPU stretches over the window, redrawn only as the
chosen game's colour moves, instead of three radial gradients the window's size.

On the RG ARC (Mali-G52, Panfrost, OpenGL ES 3.1), measured with Panfrost's per-client GPU time (`/proc/<pid>/fdinfo`):

- **GTK 3's read back**: an opaque `GtkGLArea` is a renderbuffer, which GTK copies to the window only with framebuffer blits; on ES it
  finds none, and read each frame back with `glReadPixels` (about 150 ms a frame). The window is transparent on ES for this.
- **Glass**: each pane reads what is under it back and blurs it, about 25 ms of the GPU's time a pane, so a compact panel has none.
- **MyGo's GL renderer** draws everything with its general fragment shader (13 727 instructions, one thread, 1 569 register fills on
  the G52): a fill the window's size costs about 30 ms. Metal and Direct3D 11 specialize the shader for each `gpu.Class`; GL does not
  yet.

## License

[MIT](LICENSE)
