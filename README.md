# nx2l launcher (MyGo)

The Retroid Pocket Flip 2's home screen on ROCKNIX, in place of EmulationStation: a fullscreen MyGo native UI window (GTK 3, GL on Mesa
freedreno) in a sway session of its own.

- Pages: 游戏 (Silksong, Hollow Knight through `/storage/nx2linux/launch.sh`, RetroArch), 网络 (Wi-Fi on/off, current network, scan, join a
  saved or open network), 蓝牙 (on/off through `rocknix-bluetooth`, paired devices), 显示与声音 (brightness, volume, mute), 性能与散热 (fan
  profile through ROCKNIX's `fancontrol`, fan speed, CPU governor, temperatures), 系统 (SSH, storage, memory, version, EmulationStation,
  reboot, power off). Settings go through ROCKNIX's `get_setting`/`set_setting`, so EmulationStation sees the same values.
- Gamepad (MyGo's evdev plugin, every pad): L/R page, up/down row, left/right value (held: repeats), A do, B back to the games. Touch: a tap
  does a row, − and + step a value; a finger dragging scrolls (MyGo's touch model).
- A game runs as a child process, fullscreen above the launcher (`for_window ... fullscreen enable`); the launcher ignores the gamepad until
  it exits.

Files on the device (`/storage/nx2linux/launcher`): `nx2l-launcher`, `session.sh` (sway with ROCKNIX's environment), `sway-launcher.conf`,
`nx2l-launcher.service` and `launcherctl` (../launcher). Logs: `launcher.log`, `sway.log`.

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o nx2l-launcher .
```
