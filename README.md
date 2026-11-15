# nx2l launcher (MyGo)

The Retroid Pocket Flip 2's home screen on ROCKNIX, in place of EmulationStation: a fullscreen MyGo native UI window (GTK 3, GL on Mesa
freedreno) in a sway session of its own, started at boot by `nx2l-launcher.service` (`../launcher/launcherctl enable`).

## Screens

- **Games**: one card each (Silksong and Hollow Knight through `/storage/nx2linux/launch.sh`, RetroArch). A game runs as a child process,
  fullscreen above the launcher (`for_window ... fullscreen enable`); the launcher reads no input until it exits.
- **Settings**: categories on the left, their rows on the right.

| Category | Rows |
| --- | --- |
| 网络 | Wi-Fi on/off, current network and IP, scan, join (saved or open at once, secured with the on-screen keyboard), forget (X); SSH, Samba, Syncthing |
| 蓝牙 | on/off (`rocknix-bluetooth`), paired devices (connect/disconnect, forget with X), scan and pair new devices |
| 显示 | brightness (slider) |
| 声音 | volume (slider), mute, output (PipeWire switches to the headphones while they are plugged in) |
| 性能 | CPU governor (all clusters), GPU governor (devfreq), fan profile (ROCKNIX `fancontrol`); temperatures, clocks, fan speed |
| 电源 | charge, power, voltage, current, battery temperature; reboot and power off, each confirmed |
| 系统 | time zone (`steamos-set-timezone`), network time; IP, storage, memory, uptime, ROCKNIX version, kernel |

Settings go through ROCKNIX's `get_setting`/`set_setting`, so they hold across reboots as ROCKNIX applies them.

EmulationStation is gone from the boot path: `launcherctl enable` masks `emustation.service` and gives `essway.service` a drop-in whose
condition never holds (ROCKNIX's `111-sway-init` unmasks essway at every boot; drop-ins survive it). `/usr` is read-only, so this is how ES is
removed; `launcherctl disable` restores ROCKNIX's defaults. There is no switch back to ES in the launcher.

## Controls

The built-in gamepad reaches the launcher as InputPlumber's virtual DualSense (InputPlumber hides `/dev/input/event6` and exposes a DS5
and a keyboard); MyGo's evdev plugin reads every pad.

- L/R: Games and Settings. Games: left/right choose, A starts.
- Settings: up/down the categories, right or A enters; then up/down rows, left/right change a value (held: repeats), A does, X the row's
  second action, B back.
- On-screen keyboard: d-pad and A type, X shifts, B erases, + accepts, − cancels.
- Touch: a tap does what A does; a finger drags a slider (the window sets `TouchPointer` from `nimrc/mygo`'s `touch-pointer` branch, which
  `go.mod` replaces MyGo with).

## Testing on the device

`NX2L_CTL=/run/nx2l.ctl` (set in `sway-launcher.conf`) is a FIFO of button names (`up down left right a b x l r start back`) and
`find <category> <label>`, which selects a row. With `grim` for screenshots, every screen and setting can be driven over SSH.

## Covers

`fetch-art.sh <dir>` fetches each game's Steam library capsule (600x900) as `<id>.jpg`; the launcher reads them from `art/` beside its
executable. They are the publishers' art, so they stay on the device and out of git.

## Performance

A clip or a rounded image under a transform (`Scale`) is masked on the CPU by MyGo's raster: the cards grew by `Scale` inside a `Clip`
and took 1.4 cores while the choice moved. They grow by their size instead, so every rounded rectangle stays axis-aligned and on the GPU
(0.35 core while moving, 3% idle). `NX2L_PPROF=<file>` writes a CPU profile of the launcher's first 20 s.

## Build and deploy

```sh
. /Volumes/FANXINAG/nx2linux/env.sh   # Go and caches on the external drive
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $NX2L_BUILD/nx2l-launcher .
```

Device folder `/storage/nx2linux/launcher`: `nx2l-launcher`, `session.sh`, `sway-launcher.conf`, `nx2l-launcher.service`, `launcherctl`.
Logs: `launcher.log`, `sway.log`.

Idle at the games screen: the launcher and sway together 151 MB RSS and 2% of one core (EmulationStation and sway: 217 MB, 7.8%).
