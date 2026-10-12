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
# started) is printed as it is.
jq -nrj --raw-input '
  [inputs | fromjson? // {Action: "output", Output: (. + "\n")}] as $events
  | ($events
      | map(select(.Test != null and (.Action == "pass" or .Action == "skip"))
            | "\(.Package) \(.Test)")
      | INDEX(.)) as $settled
  | $events[]
  | select(.Action == "output" or .Action == "build-output")
  | select(.Test == null or ($settled["\(.Package) \(.Test)"] | not))
  | .Output
' "$events"

echo
echo "Slowest packages and tests:"
go run ./cmd/testtiming -n 15 <"$events" || true

exit "$status"
