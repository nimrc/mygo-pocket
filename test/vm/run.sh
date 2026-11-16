#!/usr/bin/env bash
# Runs smoke.sh on a Mac in a Lima VM (lima.yaml), as CI runs it on an arm64 runner; screens and logs land in build/vm/out.
# The VM is made the first time and kept: LIMA_HOME says where (its disk takes ~4 GB).
set -euo pipefail
cd "$(dirname "$0")/../.."
vm=${MYGO_POCKET_VM:-mygo-pocket}
test/vm/build.sh build/vm/bin
if ! limactl list -q 2>/dev/null | grep -qx "$vm"; then
	limactl start --name="$vm" --tty=false --mount-none test/vm/lima.yaml
elif [ "$(limactl list -f '{{.Status}}' "$vm")" != Running ]; then
	limactl start --tty=false "$vm"
fi
COPYFILE_DISABLE=1 tar --no-xattrs --no-mac-metadata -c test/vm dist build/vm/bin | limactl shell "$vm" sh -c 'rm -rf /tmp/pocket && mkdir /tmp/pocket && tar -x -C /tmp/pocket'
limactl shell "$vm" sudo /tmp/pocket/test/vm/deps.sh
rm -rf build/vm/out
status=0
limactl shell "$vm" sh -c 'cd /tmp/pocket && test/vm/smoke.sh build/vm/bin out' || status=$?
limactl shell "$vm" tar -c -C /tmp/pocket out | tar -x -C build/vm
exit $status
