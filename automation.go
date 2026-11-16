//go:build e2e

package main

// test/vm/smoke.sh drives the real launcher in a sway session through MyGo's automation plugin; release builds leave it out
import _ "github.com/egoist/mygo/plugins/automation"
