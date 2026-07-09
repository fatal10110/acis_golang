# Pooled Packet API Design

## Goal

Make the listed game-server packet encoders produce owned, pooled frames as their only outbound API, and measure their actual `Session.SendFrame` handoff.

## Context

The current `Frame*` builders use `packetWriterPool`, but their matching `Encode*` functions still expose fresh payload slices. There are no live callers for these packets in the current server; `AuthLoginFail` is the only live `SendFrame` user and already follows the owned-frame pattern.

## Design

- Remove the obsolete unpooled `Encode*` functions for UserInfo, ItemList, CharSelectInfo, CharSelected, SkillList, character create/delete results, NewCharacterSuccess, and SSQInfo.
- Keep their `Frame*` builders as the sole API. Error-returning builders continue to return a zero frame and release their writer before returning an error.
- Update unit and sequence tests to read payload bytes from the owned frame and call `Release` after inspection.
- Move the UserInfo allocation comparison to the network benchmark package. The pooled side constructs `FrameUserInfo` and hands it to `Session.SendFrame`; the unpooled baseline uses the existing payload path. Both use the discard connection already used by the AuthLoginFail send benchmarks.

## Guarantees

- Packet bytes remain unchanged: every frame still reserves and backfills the same two-byte little-endian length header.
- An owned frame is released exactly once by the connection writer after it is sent.
- No new abstraction or dependency is introduced.

## Verification

- Tests first demonstrate that the obsolete encoder APIs are unavailable at each migrated test call site.
- Run the server-packet and network test packages, then their race variants.
- Run formatting, vet, build, and the full race suite; report any environment-only fixture failure separately.
