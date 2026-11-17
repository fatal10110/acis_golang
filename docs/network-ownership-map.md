# Network Ownership Map

`internal/gameserver/network` owns packet and session adaptation: decoding, state gates, resolving
the acting client and actor, mapping domain outcomes to frames, send order, and connection
lifecycle. Game rules, validation, state mutation and persistence decisions belong to domain
packages; server-initiated changes reach the package as `event.Event` values. The package rules are
in `internal/gameserver/network/AGENTS.md`.

The package is large (about 290 non-test files), so this map is by file family rather than by file.
A file that does not fit a family below is a gameplay adapter.

| Family | Files | Owns | Notes |
|---|---|---|---|
| Transport | `conn.go`, `session.go`, `broadcast.go`, `frame_reject.go`, `cipher/` | outbound queue and its writer goroutine, the rolling cipher and send order, per-recipient broadcast copies, refused-frame diagnostics | `frame_reject.go` reports to one process-wide reporter |
| Inbound loop and gates | `client_loop.go`, `floodprotector.go`, `clientstats.go`, `state.go`, `client.go`, `client_dispatch.go`, `clientregistry.go` | the read loop, packet protection, the per-state opcode gate, abuse counters, the handler registry and its `whenLive` adapters | every accepted opcode has a handler (`TestEveryAcceptedOpcodeHasAHandler`) |
| Login server link | `loginlink.go`, `loginstatus.go`, `loginanswers.go` | the GS-LS link, its handshake and reconnect loop, status and account answers | candidate for its own package |
| Composition | `dispatch.go` | `GameClientLink` and its collaborators, `NewGameClientLink` | also builds several services and fallbacks; see the S3 cleanup issue |
| Character lifecycle | `character_*.go`, `live_*.go`, `lifecycle.go`, `restart*.go`, `disconnect_delay.go`, `exitguard.go` | character selection, the EnterWorld burst and its order, the in-world player handle, logout and disconnect | the EnterWorld order is client-visible and pinned by tests |
| GM commands | `admin_*.go`, `gm*.go` | `//` command parsing and GM-only views | candidate for `network/admin` behind a GM-session interface |
| Gameplay adapters | one file or prefix family per domain (`clan_*`, `castle_*`, `siege_*`, `party_*`, `summon_*`, `pet_*`, `olympiad_*`, `trade.go`, `enchant.go`, ...) | decode, resolve the actor, call the domain package, map the outcome to frames | rule code found here belongs in the domain package |
| Packets | `clientpackets/`, `serverpackets/` | opcode constants and decoders; frame encoders | some encoder snapshots still read a live character at encode time |

When a change moves rule code out of `network`, or splits a family into its own package, update the
matching row.
