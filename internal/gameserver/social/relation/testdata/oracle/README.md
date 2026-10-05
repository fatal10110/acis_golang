# Relation list order oracle

`../order.golden` is the expected output of `TestListOrderMatchesReferenceProbe`
(`internal/gameserver/social/relation/order_oracle_test.go`). It is the output
of `RelationOrderProbe.java` in this directory over `../order.scenarios`.

## What the probe is

`RelationOrderProbe.java` copies, from aCis 409 (outer repository revision
`55ff8a4e`)
`aCis_gameserver/java/net/sf/l2j/gameserver/data/manager/RelationManager.java`,
the `_relations` store (`ConcurrentHashMap<PlayerPair, Integer>`), the
constructor's `putIfAbsent` load, `isInBlockList`, `getBlockList`,
`addToBlockList`, `removeFromBlockList`, `areFriends`, `getFriendList`,
`addToFriendList`, `removeFromFriendList` and `updateRelation`, and
`model/records/PlayerPair.java`. The copies are unchanged except:

- A `Player` is its object id.
- Packets, system messages and `World` lookups are dropped; none of them
  touches the store.
- The load reads the scenario's `load` rows instead of
  `SELECT * FROM character_relations`. The scenarios list them in primary-key
  order, the order a full scan of the InnoDB table returns them and the order
  the Go loader asks for.

The list order comes from the JDK's `ConcurrentHashMap` and `HashSet`
(`HashMap`), so the probe runs on the JDK the reference requires (21).

## Scenario lines

Blank lines and lines starting with `#` are skipped.

- `scenario <name>`: a new, empty relation store.
- `load <char_id> <friend_id> <relation> [<stepA> <stepB> <count>]`: stored
  rows, read at boot. They come before the scenario's other lines.
- `friend <a> <b> [...]`: `RequestAnswerFriendInvite` accepted:
  `addToFriendList(a, b)` then `addToFriendList(b, a)`.
- `unfriend <a> <b> [...]`: `removeFromFriendList(a, b)`.
- `block <a> <b> [...]`: `addToBlockList(a, b)`.
- `unblock <a> <b> [...]`: `removeFromBlockList(a, b)`.
- `list <id>`: prints `<name> friends <id>: ...` (`getFriendList(id)`) and
  `<name> blocks <id>: ...` (`getBlockList(id)`), in iteration order.

With `<stepA> <stepB> <count>` the line runs `count` times, the i-th time on
`a + i*stepA` and `b + i*stepB`.

The `server` scenario was generated once (Python `random.Random(3156)`: 400
ids from 268435456 up, 600 stored rows, 1500 random updates) and is
committed as plain lines; nothing regenerates it.

## Regenerating

From `internal/gameserver/social/relation/testdata`:

```bash
docker run --rm -v "$PWD:/p" -w /p eclipse-temurin:21-jdk-alpine \
  java oracle/RelationOrderProbe.java order.scenarios > order.golden
```
