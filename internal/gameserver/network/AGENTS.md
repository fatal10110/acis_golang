# Network package agent rules

These rules apply to every change under `internal/gameserver/network`. Repository-wide rules in
[`../../../AGENTS.md`](../../../AGENTS.md) still apply.

## Packet impact is part of scope

Before planning, before each meaningful implementation step, and before completion, identify the
client-visible packet surface of the behavior being changed.

- Start from the relevant milestone and packet appendices in
  `../../../../aCis_gameserver/docs/go-rewrite/`, including opcode and field-layout documents.
- Search the behavioral source for inbound handlers and outbound sends: handler registration,
  client packets, server packets, direct sends, broadcasts, and packet imports are all scope clues.
- List related inbound and outbound packets for any client-visible data, world, movement, door,
  teleport, item, inventory, skill, combat, death, pet, quest, clan, store, manor, recipe, henna, or
  similar flow.
- Implement and wire packets required by the current milestone. If a packet depends on a deliberately
  later system, document the dependency, reason, and future integration point.
- Never leave an accepted opcode as an undocumented no-op or omit an expected send silently.

## Exact protocol behavior

Verify opcode, field order and width, byte order, state gating, send/broadcast target, packet order,
and framing. Use known-good byte fixtures, focused packet tests, or `cmd/packetdiff`; do not recreate
expected bytes from the encoder being tested.

Search both sides of a flow. A correct decoder with a missing response, or a correct packet type sent
in the wrong order, is incomplete behavior.

## No silent action rejection

Every rejection branch in a handler for a client action must answer. After decoding a request, an
early return for a missing object, invalid state, insufficient material, inactive session, ownership
failure, or domain rejection sends one of:

- the appropriate domain packet;
- a system message;
- `ActionFailed` when that is the protocol contract.

Never return nothing while the client is waiting for completion. Walk every early return, including
errors returned by domain services. Extend `silent_action_test.go` whenever a new action-shaped opcode
or rejection branch is added; passing registration checks alone is not sufficient.

## Orchestration boundary

Network code decodes fields, resolves session/world context, calls a domain API, and translates a
typed outcome into packets. It does not own target rules, skill or cast validation, costs, HP/MP
changes, item filtering, trade or pet rules, persistence decisions, or other game rules.

If a handler needs substantial rule logic, move that logic into a focused domain package first. Keep
the network-facing outcome explicit enough to map every success and rejection branch.

## Concurrency and lifecycle

- One goroutine writes each connection; other goroutines enqueue work through the owned path.
- Session and handler dependencies have documented ownership and cancellation.
- Do not hold a state lock while sending a packet, performing database I/O, or calling unknown code.
- Run focused race tests for changed shared state or lifecycle behavior.

## Completion check

Before completion, confirm the inbound/outbound packet list, opcode and field layout, state gates,
send order, rejection responses, byte fixtures, `silent_action_test.go` coverage, domain boundary,
concurrency ownership, and every explicit deferral.
