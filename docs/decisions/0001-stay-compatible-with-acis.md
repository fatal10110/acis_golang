# 0001. Stay compatible with the aCis Java server

- **Status:** Accepted
- **Date:** project start; restated in every porting plan since
- **Sources:** [ops/migrating-from-java.md](../ops/migrating-from-java.md),
  [agents/porting-playbook.md](../agents/porting-playbook.md),
  [memory-redesign-plan.md](../memory-redesign-plan.md) (scope line),
  [actor-model-degoifying-plan.md](../actor-model-degoifying-plan.md) (invariants)

## Context

The project ports aCis, a Java server for the Lineage II Interlude client, to Go. A port could
take two directions: rebuild the game on new data formats and protocols, or reproduce the existing
server exactly in a new language.

The users of such a server already have three things they cannot change easily: the game client
(which speaks a fixed protocol), a database full of characters and items, and a datapack and config
set they have tuned. aCis also gives us something valuable: a working reference. When we are not
sure how something should behave, the Java server answers the question.

## Decision

The Go servers behave like the aCis Java server, as seen from outside:

- the same client protocol (revision 746): packets, field layout, packet order, and which packets
  are sent when;
- the same database schema, read and written in the same way;
- the same config files and keys;
- the same datapack (XML, HTML) and geodata files, read with the same rules;
- the same two-process layout: a login server and game servers joined by the GS-LS link protocol;
- the same game formulas and rules, including their quirks.

Inside, the code is free to be idiomatic Go. Internal refactors are allowed as long as nothing
visible changes.

A deliberate difference from the reference needs a reason, an issue, a test, and a written record:
a decision record for a design choice (for example [0010](0010-secure-defaults.md)), or a *parity
exception* in [agents/porting-playbook.md](../agents/porting-playbook.md#3-check-the-client-visible-surface)
for a single behavior.

## Consequences

- An existing aCis installation can switch to the Go servers and back without converting data.
- The reference is the test oracle. Tools compare the Go results with it: `cmd/datadiff` for
  loaded data, `cmd/geoprobe` for geodata answers, `internal/testsupport/scriptfp` for scripts, and
  scenario traces for quests and NPC behavior.
- Some Java behavior must be reproduced on purpose, even where Go would naturally do something
  else. For example, `internal/commons/hashorder` reproduces the order a Java concurrent hash table
  lists ids in, because the client shows several lists in that order.
- Any packet-order or formula difference is a bug, not a matter of taste. Reviews check behavior
  against the reference, which takes time.
- The project cannot improve the protocol or schema, even where they are awkward.

## Alternatives considered

- **A new design with new data formats.** This would allow a cleaner system, but it would break
  every existing client and installation, and it would leave us with no oracle to test against.
- **A literal line-by-line translation of the Java code.** This would be compatible, but it would
  carry Java's structure into Go (base classes, injected callbacks, a lock on every object).
  Several later decisions ([0003](0003-per-actor-queues.md), [0005](0005-domain-events-to-packets.md))
  remove exactly that structure.
