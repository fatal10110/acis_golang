# 0005. Domain code reports events; only the network layer builds packets

- **Status:** Accepted
- **Date:** planned before the 2026-09-05 infrastructure review, which cites it
- **Sources:** [actor-model-degoifying-plan.md](../actor-model-degoifying-plan.md) (Phase B),
  [network-ownership-map.md](../network-ownership-map.md),
  [`internal/gameserver/network/AGENTS.md`](../../internal/gameserver/network/AGENTS.md),
  `internal/gameserver/model/actor/event`

## Context

When something happens to an actor (it moves, takes damage, dies, changes equipment), nearby clients
must get the right packets in the right order. The Java server does this through methods that
subclasses override. The first version of the port copied that shape with function fields: the
player type alone had 63 injected callbacks, set through 67 `Set...` setters, and 53
`if hook != nil { hook() }` checks.

This made domain code hard to follow, because what happened on an event depended on which callbacks
someone had wired up elsewhere. It also made it easy for game rules and packet building to mix.

## Decision

Domain code describes what happened, and the network layer decides what clients see:

- `internal/gameserver/model/actor/event` defines a closed set of event types (`Died`, `Moved`,
  ...). Domain code emits an event to the actor's `event.Sink`.
- In `internal/gameserver/network`, one type switch per actor kind maps each event to packets, in
  the order clients expect.
- Domain packages (`model/...`, `skill/...`, `handler/...`) never import the packet encoders.

Inbound packets follow the same split: a handler decodes the request, calls a domain API, and turns
its typed result into packets. Game rules belong in the domain packages, not in handlers. Every
rejected client action must get an answer, so the client is never left waiting.

## Consequences

- All the packets for one kind of event are in one place, which makes send order easy to review
  and test.
- Domain code can be tested by recording its events (`event.Recorder`), without a network.
- Adding a client-visible effect means adding an event type and its mapping, not a new callback.
- The network package is large, and some rule code still lives there. The
  [network ownership map](../network-ownership-map.md) tracks what belongs where.

## Alternatives considered

- **Keep the injected callbacks.** They mirror the Java design, but they spread the wiring over many
  files and hide what an event does.
- **One large "client" interface per actor kind.** The plan's owner rejected it in favor of typed
  events: an interface with dozens of send methods would keep packet thinking inside the domain.
