#!/usr/bin/env bash
# Runs `go test -json` with the given arguments and keeps the log readable:
# it prints each package's result line, the output of every test that did not
# pass or skip (a failure, a race report, or the test a timeout caught
# running), and then the slowest packages and top-level tests. It exits with
# go test's status.
set -uo pipefail

events="$(mktemp)"
trap 'rm -f "$events"' EXIT

go test -json "$@" >"$events"
status=$?

# A line that is not a test event (a build error printed before the stream
# started) is printed as it is. A passed or skipped test's output is dropped
# only up to its own --- PASS or --- SKIP line: go test hands runtime output
# that belongs to no test (a timeout panic and its goroutine dump, a panic in
# a server goroutine) to whichever test printed last, which in a parallel
# suite has usually already passed.
jq -nrj --raw-input '
  [inputs | fromjson? // {Action: "output", Output: (. + "\n")}] as $events
  | (reduce range(0; $events | length) as $i ({};
      $events[$i] as $e
      | if $e.Test != null and $e.Action == "output"
           and ($e.Output | test("^\\s*--- (PASS|SKIP): "))
        then .["\($e.Package) \($e.Test)"] = $i else . end)) as $settled
  | range(0; $events | length) as $i
  | $events[$i]
  | select(.Action == "output" or .Action == "build-output")
  | select(.Test == null or ($settled["\(.Package) \(.Test)"] // -1) < $i)
  | .Output
' "$events"

echo
echo "Slowest packages and tests:"
go run ./cmd/testtiming -n 15 <"$events" || true

exit "$status"
