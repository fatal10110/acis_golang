# Creature movement smoke path (#81)

Use two game clients that can observe each other. This is a manual smoke
contract for the future packet/world adapter; it consumes the `move.Event`
returned by `CreatureMove.MoveToLocation` and does not prescribe adapter
implementation details.

## Passable target

1. Place the mover and an observing client in mutual visibility range.
2. Call `CreatureMove.MoveToLocation` for the mover with a passable target and
   retain the returned `move.Event`.
3. Verify the adapter broadcasts exactly one movement event. Its origin must
   equal `Event.Origin`, and its normalized geodata Z must equal
   `Event.Destination.Z`.
4. Verify the observing client sees one movement start, and the mover reaches
   `Event.Destination`.

## Blocked target

1. Keep both clients in range and call `CreatureMove.MoveToLocation` with a
   blocked target.
2. Verify no event is broadcast and neither client sees movement.

## API ownership check

`move.NewCreatureMove` creates a `move.CreatureMove`, whose movement state is
owned and updated by one caller. That caller invokes `MoveToLocation` and, for
an accepted request, receives the `move.Event` used by the adapter checks
above.
