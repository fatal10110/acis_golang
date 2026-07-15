# Network Ownership Map

`internal/gameserver/network` should own packet/session adaptation: decode, state gates, frame
mapping, and send order. Gameplay state and reusable mutation rules should live in focused domain
packages.

| File | Current role | Keep in network? | Next owner |
|---|---|---:|---|
| `internal/gameserver/network/client_loop.go` | Opcode gate, packet decode, handler dispatch | yes | n/a |
| `internal/gameserver/network/dispatch.go` | Connection collaborator wiring and protocol handshake dependencies | yes | n/a |
| `internal/gameserver/network/character_flow.go` | Login/session character flow plus inventory restore adapter | yes for now | roster/session application service if this grows |
| `internal/gameserver/network/inventory.go` | Packet adapter for equip/drop/destroy/crystallize; mutation delegated to `internal/gameserver/inventory` | temporarily | equip workflow can move to `internal/gameserver/inventory` when equipment messages are isolated |
| `internal/gameserver/network/trade.go` | Direct-trade frame mapping and participant lookup; state delegated to `internal/gameserver/trade` | yes | n/a |
| `internal/gameserver/network/pet.go` | Active-pet lookup, distance check, frame mapping; pet item rules delegated to `internal/gameserver/petitem` | yes | n/a |
| `internal/gameserver/network/enchant.go` | Enchant result-to-frame mapping and persistence application; state/mutation delegated to `internal/gameserver/enchant` | yes | n/a |
| `internal/gameserver/network/movement.go` | Movement packet responses, world position update, visibility broadcast | temporarily | `world` or actor movement controller |
| `internal/gameserver/network/targeting.go` | Target selection frames, pet-status shortcut, attack start adapter | temporarily | player target state plus combat adapter |
| `internal/gameserver/network/magic_skill.go` | Packet adapter over `model/actor/cast.Controller` | mostly | `skillflow` only if more cast orchestration lands |
| `internal/gameserver/network/skill_acquisition.go` | Learn-skill validation, persistence, and packet responses | move later | `skillflow` or `model/actor/player` learn API |
| `internal/gameserver/network/visibility.go` | Broadcast mapping for world visibility changes | yes | n/a |
| `internal/gameserver/network/shop.go` | Store packet adapter and buy/sell/list frame mapping | temporarily | shop application package when merchant state expands |
