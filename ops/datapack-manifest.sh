#!/bin/sh
# Writes or checks ops/datapack.sha256: the SHA-256 of every file in the
# repository's aCis_datapack/ copy, in byte order of the path. CI runs the
# check, so the copy cannot drift from what was imported: an edit, a
# line-ending conversion, or a missing or extra file fails it.
#
#   ops/datapack-manifest.sh write   after resyncing the copy
#   ops/datapack-manifest.sh check   compare the copy against the manifest
#
# A local data/geodata under the copy is ignored (it is not committed).
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
manifest=$root/ops/datapack.sha256

list() {
	cd "$root/aCis_datapack" &&
		find . -type f ! -path './data/geodata/*' | sed 's|^\./||' | LC_ALL=C sort |
		tr '\n' '\0' | xargs -0 sha256sum
}

case "${1:-}" in
write)
	list >"$manifest"
	echo "datapack-manifest: wrote $(wc -l <"$manifest") files to $manifest"
	;;
check)
	if ! out=$(list | diff -u "$manifest" -); then
		printf '%s\n' "$out" | head -40 >&2
		echo "datapack-manifest: FAIL: aCis_datapack/ differs from $manifest" >&2
		exit 1
	fi
	echo "datapack-manifest: ok, $(wc -l <"$manifest") files match"
	;;
*)
	echo "usage: $0 write|check" >&2
	exit 2
	;;
esac
