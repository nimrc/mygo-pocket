#!/bin/sh
# Builds what smoke.sh runs, for linux/arm64, into $1: the launcher with the automation plugin (-tags e2e) and its client mygo-automation
set -eu
out=${1:-build/vm/bin}
mkdir -p "$out"
export GOOS=linux GOARCH=arm64 CGO_ENABLED=0
go build -tags e2e -o "$out/mygo-pocket" .
go build -o "$out/mygo-automation" github.com/egoist/mygo/plugins/automation/cmd/mygo-automation
