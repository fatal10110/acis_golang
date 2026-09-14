# Server-initiated updates

The repository packet-impact rule covers inbound opcodes: an accepted user action may not be a quiet
no-op. This guide covers the other direction — state the server changes without a client request:
kill and quest rewards, ticking tasks, effect actions, AI, decay, respawn, and zone effects.

Load it whenever a change touches one of those paths.

## The failure it prevents

Unit tests assert domain state, not delivery. A reward that updates experience and SP passes every
test while the client keeps displaying the values it was last told about, until some unrelated action
— equipping an item, reopening a window — happens to resend the packet. That is a bug, not a cosmetic
delay.

## Rules

- Find where the reference sends its packet for that change and send the equivalent from the Go code
  path that makes the change. The send often sits inside a setter rather than the caller:
  `PlayerStatus.addExp` sends `UserInfo`, `PlayerStatus.setSp` sends a `StatusUpdate`, and
  `CreatureStatus.setHp` calls `broadcastStatusUpdate`. Read the setter, not only the reward or task
  method that calls it.
- Domain packages must not import `serverpackets`; that is a real import cycle, not a style rule.
  Emit an `event.Event` from the code path that makes the change; the actor's `event.Sink` (set
  once at attach, nil in domain tests) carries it; the network layer maps it to packets in that
  actor kind's single `Emit` type switch; cover delivery with a domain test asserting on
  `event.Recorder`.
- Never add a func-typed hook field or a `Set<Thing>Hook` setter to an actor; add an event type
  instead.
- A test asserting only the new state value cannot fail when the packet is missing, which is exactly
  the bug.
- A ported task manager, hook interface, or composition-root adapter is not done until a production
  caller feeds it. A constructor referenced only from tests, a queue nothing drains, and an adapter
  method with an empty body are unshipped features, not complete ports; treat them as gaps needing a
  follow-up issue rather than evidence that the port exists.

## Checks

See [`verification.md`](verification.md) for the two detector commands and their known false
negative for `fx`-provided types.
