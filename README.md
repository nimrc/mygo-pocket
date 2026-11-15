# MyGo probe

A MyGo native UI window as the launcher's shell under sway, to check what the PocketJS launcher could not do outside its sandbox: GTK 3 + GL on Mesa
freedreno, the gamepad (MyGo's game plugin, evdev), touch through sway, the battery from sysfs and the brightness through ROCKNIX's `brightness`.

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o nx2l-mygo .
```

On the device (`/storage/nx2linux/mygo`): `sway -c sway-probe.conf` with ROCKNIX's environment (`/etc/profile`, `WLR_BACKENDS=drm,libinput`).

Measured idle: the probe 82 MB RSS, sway 51 MB; 3.5% of one core together.
