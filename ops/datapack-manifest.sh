#!/bin/sh
# Writes or checks ops/datapack.sha256: the SHA-256 of every file in the
# repository's aCis_datapack/ copy, in byte order of the path. CI runs the
# check, so the copy cannot drift from what was imported: an edit, a
# line-ending conversion, or a missing or extra file fails it.
#
#   ops/datapack-manifest.sh write   after resyncing the copy
#   ops/datapack-manifest.sh check   compare the copy against the manifest
#   ops/datapack-manifest.sh package OUT.tar.zst
#                                    check, then archive exactly the manifest's
#                                    files (needs GNU tar and zstd)
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
package)
	out=${2:?usage: $0 package OUT.tar.zst}
	"$0" check
	# Members are the manifest's paths, relative to the datapack root, in its
	# order; data/geodata is never one of them. Fixed mtime, owner and modes
	# make the same commit give the same bytes.
	# tar writes a file, not a pipe: sh has no pipefail to report its failure.
	cut -c67- "$manifest" >"$out.list"
	tar -C "$root/aCis_datapack" --format=gnu --mtime=@0 --owner=0 --group=0 \
		--numeric-owner --mode=u=rwX,go=rX --verbatim-files-from -T "$out.list" -cf "$out.tar"
	rm "$out.list"
	zstd -q -f -19 -T1 --rm -o "$out" "$out.tar"
	echo "datapack-manifest: packaged $(wc -l <"$manifest") files into $out"
	;;
*)
	echo "usage: $0 write|check|package OUT.tar.zst" >&2
	exit 2
	;;
esac
