#!/usr/bin/env bash
# Reads the test shards out of .github/workflows/go.yml, the one place that
# names them, so the shard matrix and what is derived from it cannot drift.
#
#   test-shards.sh suites   print the packages the `suites` shard runs: every
#                           tests/ package no other inline shard runs
#   test-shards.sh check    fail unless every tests/ package runs on the real
#                           pool (a `pool:` list) or is named, with its
#                           reason, in POOL_EXCLUDED, and not both
set -euo pipefail

workflow="$(dirname "${BASH_SOURCE[0]}")/../workflows/go.yml"

# shard_lists KEY prints the packages every shard lists under KEY (`pkgs` or
# `pool`), one per line; the `suites` placeholder is not a package.
shard_lists() {
  awk -v key="$1:" '$1 == key && $2 != "suites" { for (i = 2; i <= NF; i++) print $i }' "$workflow"
}

# pool_excluded prints the name of each tests/ package POOL_EXCLUDED lists,
# one per line: the block's lines read `name: reason`.
pool_excluded() {
  awk '
    $1 == "POOL_EXCLUDED:" { inblock = 1; match($0, /^ */); keyindent = RLENGTH; next }
    inblock && NF == 0 { next }
    inblock {
      match($0, /^ */)
      if (RLENGTH <= keyindent) exit
      if (indent == 0) indent = RLENGTH
      if (RLENGTH < indent) exit
      name = $1
      sub(/:$/, "", name)
      print name
    }
  ' "$workflow"
}

# tests_packages prints every tests/ package as ./tests/<name>.
tests_packages() {
  go list -f '{{.Dir}}' ./tests/... | sed "s|^$PWD/|./|"
}

case "${1:-}" in
suites)
  tests_packages | grep -vxFf <(shard_lists pkgs)
  ;;
check)
  status=0
  pooled="$(shard_lists pool | tr ' ' '\n' | sort -u)"
  excluded="$(pool_excluded | sed 's|^|./tests/|' | sort -u)"
  all="$(tests_packages | sort -u)"
  for pkg in $all; do
    in_pool=0 in_excluded=0
    grep -qxF "$pkg" <<<"$pooled" && in_pool=1
    grep -qxF "$pkg" <<<"$excluded" && in_excluded=1
    if [ "$in_pool" = 0 ] && [ "$in_excluded" = 0 ]; then
      echo "$pkg is on no real-pool shard and not in POOL_EXCLUDED: add it to a pool shard, or list it with its reason" >&2
      status=1
    elif [ "$in_pool" = 1 ] && [ "$in_excluded" = 1 ]; then
      echo "$pkg is on a real-pool shard and also in POOL_EXCLUDED: drop the exclusion" >&2
      status=1
    fi
  done
  for pkg in $pooled $excluded; do
    case "$pkg" in ./tests/*) ;; *) continue ;; esac
    grep -qxF "$pkg" <<<"$all" || { echo "$pkg is named in the workflow but is not a tests/ package" >&2; status=1; }
  done
  exit "$status"
  ;;
*)
  echo "usage: ${0##*/} suites|check" >&2
  exit 2
  ;;
esac
