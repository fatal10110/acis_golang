## Summary

<!-- What does this change do, and why? One short paragraph. -->

Closes #

## Changes

<!-- The main changes, as a short list. -->

-

## Behavior

<!--
Does anything change for players or operators? Packets, their order, formulas, saved data,
config or commands. If the change makes the Go server behave differently from the aCis Java server
on purpose, link the parity exception or decision record that explains it.
-->

## Testing

<!-- The commands you ran and their results. Say so if a test was skipped or could not run. -->

- [ ] `make test-race`
- [ ] `golangci-lint run ./...`
- [ ] Fuzz target run (packet decoder or framing changes)
- [ ] Geodata or reference tests run locally (when they apply)

## Checklist

- [ ] Tests added or updated, including a behavior test in `tests/` for new features
- [ ] Docs updated (behavior, config, commands or operations)
- [ ] New decision record in `docs/decisions/` if this changes an architecture decision
