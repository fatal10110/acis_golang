# Running Loginserver And Gameserver

Run these commands from the repository root. To run everything in containers instead, see
[ops/docker.md](ops/docker.md). To move an existing aCis Java installation, see
[ops/migrating-from-java.md](ops/migrating-from-java.md). For backups, see
[ops/backup-restore.md](ops/backup-restore.md).

## Prerequisites

- MariaDB is running and the `acis` schema has been loaded from:

```bash
mysql -uroot acis < ../aCis_datapack/tools/full_install.sql
for f in ../aCis_datapack/sql/*.sql; do mysql -uroot acis < "$f"; done
```

- `../aCis_gameserver/config/loginserver.properties` and `../aCis_gameserver/config/server.properties` point at that database with the `URL`, `Login`, and `Password` keys.
- Optional: append `?socketTimeout=<ms>` (and `connectTimeout=<ms>`) to `URL`, for example `jdbc:mariadb://localhost/acis?socketTimeout=10000`, to bound each database read and write at the driver. The login server also bounds every account and registration call at 10 seconds on its own; a call that runs out fails that one request with the same reply as any other database failure.
- `server.properties` has `LoginHost = 127.0.0.1` and `LoginPort = 9014`, matching `loginserver.properties`.
- Optional: `MaxConnections` in `loginserver.properties` / `server.properties` sets that process's database pool size. The key is absent from the shipped files; omitting it keeps the current pool size of 8. Values below 1 are rejected at boot.
- XML datapack files stay on disk under `../aCis_datapack`; do not import them into the database.
- Geodata files stay on disk under `../aCis_datapack/data/geodata`. The shipped `GeoDataPath = ./data/geodata/` is resolved against `-data-root`, so L2OFF files should be named like `16_10_conv.dat` and L2J files like `16_10.l2j`.

## Build

```bash
go build ./cmd/loginserver ./cmd/gameserver ./cmd/gsregister ./cmd/botswarm
```

`botswarm` is the load-test client described in [Bot Swarm](#bot-swarm); the servers do not need it.

## Register A Gameserver ID

```bash
go run ./cmd/gsregister \
  -config ../aCis_gameserver/config/loginserver.properties \
  -names ../aCis_datapack/data/serverNames.xml
```

Choose server ID `1` unless the database already reserves another ID. Then install the generated hexid file:

```bash
mv 'hexid(server 1).txt' ../aCis_gameserver/config/hexid.txt
```

Or register without prompts, writing the file in place; a rerun over the same registration changes
nothing:

```bash
go run ./cmd/gsregister \
  -config ../aCis_gameserver/config/loginserver.properties \
  -names ../aCis_datapack/data/serverNames.xml \
  -register 1 -out ../aCis_gameserver/config/hexid.txt
```

## Start Loginserver

```bash
go run ./cmd/loginserver \
  -config ../aCis_gameserver/config/loginserver.properties \
  -logging ../aCis_gameserver/config/logging.properties \
  -server-names ../aCis_datapack/data/serverNames.xml \
  -banned-ips ../aCis_gameserver/config/banned_ips.properties \
  -log-root log/login
```

Add `-debug-addr 127.0.0.1:6061` to serve the debug endpoints described under [Debug Endpoints](#debug-endpoints).

The loginserver binds the client listener from `LoginserverHostname/LoginserverPort` and the gameserver-link listener from `LoginHostname/LoginPort`. For either host, `*` binds every address and an empty value binds `127.0.0.1` only. A `LoginserverHostname` that does not resolve logs an error and binds every address; a `LoginHostname` that does not resolve fails boot.

## Start Gameserver

In a second terminal:

```bash
go run ./cmd/gameserver \
  -config ../aCis_gameserver/config/server.properties \
  -players-config ../aCis_gameserver/config/players.properties \
  -npcs-config ../aCis_gameserver/config/npcs.properties \
  -clans-config ../aCis_gameserver/config/clans.properties \
  -events-config ../aCis_gameserver/config/events.properties \
  -siege-config ../aCis_gameserver/config/siege.properties \
  -logging ../aCis_gameserver/config/logging.properties \
  -hexid ../aCis_gameserver/config/hexid.txt \
  -geo-config ../aCis_gameserver/config/geoengine.properties \
  -data-root ../aCis_datapack \
  -log-root log/gameserver
```

Add `-debug-addr 127.0.0.1:6060` to serve the debug endpoints described under [Debug Endpoints](#debug-endpoints).

Each config-file flag above defaults to a file under `config/` in the current directory, for example `-players-config config/players.properties`. When that file is missing, boot fails with `open config config/players.properties: ...`. A repository checkout has no `config/` directory, so pass every flag as shown.

Give each process its own `-log-root` and its own `-debug-addr` port. Both binaries open the same relative log paths from `logging.properties`. At boot each one truncates the console and error files, and it appends to the chat, gmaudit and item files and rotates them on its own size counter. A shared root therefore makes the two processes wipe and interleave each other's files, and a second listener on a port already in use fails boot. Log files, their JSON format, and every logging and metrics failure mode are described in [Logs, Metrics, And Failure Modes](observability.md).

The gameserver loads the minimal XML tables, loads geodata from `geoengine.properties`, links to the loginserver, and binds the game-client listener from `GameserverHostname/GameserverPort`. It loads exactly the regions listed as `X_Y` keys in `geoengine.properties`; unlisted regions use the null-region fallback even when their file exists, and a listed region whose file is missing, unreadable, or malformed fails boot with an error naming each failed file, `GeoDataPath`, and `GeoDataType`.

## Debug Endpoints

Both binaries accept an optional `-debug-addr host:port`. When set, that address serves `net/http/pprof` and `expvar`:

```bash
go run ./cmd/gameserver -debug-addr 127.0.0.1:6060 ...
curl -s http://127.0.0.1:6060/debug/vars
curl -s 'http://127.0.0.1:6060/debug/pprof/goroutine?debug=1'
```

`/debug/vars` publishes `connections` (live accepted connections, both processes), `players-online` (the gameserver's live player count; the loginserver publishes the key but always reports 0), and `tickers`, a map of `{"last":ns,"max":ns}` per scheduler ticker name.

**Bind this to loopback only.** The handlers are registered on `http.DefaultServeMux` with no authentication, and `net/http/pprof` exposes heap dumps, goroutine stacks, and `/debug/pprof/cmdline`. Never bind `-debug-addr` to `0.0.0.0` or a public interface. Omit the flag entirely to leave the listener off.

### IP Ban API

The debug listener also serves an IP ban API, with the same paths and JSON on both servers, so an
operator can see and lift bans without a restart. It is on only when `-debug-addr` is set, and it
answers only loopback clients (`403` otherwise), even if the listener is bound wider.

| Server | `server` | Store | What it holds | Add | Remove |
| --- | --- | --- | --- | --- | --- |
| login | `login` | `login-ban` | The login IP ban list: `-banned-ips` entries (permanent), failed-login bans (`LoginTryBeforeBan` failures ban for `LoginBlockAfterBan` seconds), and API bans | yes | yes |
| login | `login` | `login-accept-filter` | Burst bans on the login-client port | no | yes |
| game | `game` | `game-ban` | The gameserver IP ban list, filled only through this API | yes | yes |
| game | `game` | `game-accept-filter` | Burst bans on the game-client port | no | yes |

A `login-ban` IP is refused on the login-client port and, as before, on the gameserver-link port
(9014). With both servers on one host, banning `127.0.0.1` in `login-ban` also cuts the
gameserver's link to the loginserver. A `game-ban` IP is closed at accept on the game-client port,
before any byte and before the accept filter counts it.

`GET /debug/bans` lists every ban in force:

```json
{
  "server": "login",
  "stores": [{"name": "login-ban", "add": true}, {"name": "login-accept-filter", "add": false}],
  "bans": [
    {"ip": "203.0.113.7", "store": "login-ban", "permanent": true},
    {"ip": "198.51.100.9", "store": "login-ban", "permanent": false, "expires": "2026-11-06T12:10:00Z"},
    {"ip": "192.0.2.4", "store": "login-accept-filter", "permanent": false, "expires": "2026-11-06T12:05:00Z"}
  ]
}
```

Rows come in `stores` order, then by IP. `expires` (UTC, RFC 3339) is absent for a permanent ban.
An accept-filter ban's `expires` is its last attempt plus `AcceptFilterBanTime`: each further attempt
pushes it back, and the ban lifts at the first 5 s sweep after it. `bans` is `[]` when there are none,
and an accept-filter store is always empty when `EnableAcceptFilter = False`.

Writes are `POST` with `Content-Type: application/json` and a JSON body of at most 4 KiB with no
unknown fields:

| Path | Body | Answer |
| --- | --- | --- |
| `/debug/bans/add` | `{"ip":"203.0.113.7","store":"game-ban","permanent":true}` or `{"ip":"203.0.113.7","store":"game-ban","seconds":600}` | `{"added":true}`; `{"added":false}` when the IP already has a ban in force, which keeps its expiry |
| `/debug/bans/remove` | `{"ip":"203.0.113.7","store":"login-accept-filter"}` | `{"removed":true}`; `{"removed":false}` when the store held no ban in force for the IP |

`ip` is a literal IPv4 or IPv6 address (no host names, no zones); an IPv4-mapped IPv6 address is
its IPv4 address. `add` takes exactly one of `"permanent": true` or `"seconds"` from 1 to
315360000 (ten years). To change a ban's length, remove it and add it again. Removing a ban does not
reset a login IP's failed-login count, which the ban had already cleared; removing an accept-filter
ban clears that IP's tries too.

Errors answer `{"error":"message"}` with `400` (bad JSON, IP, store or duration, or `add` on a store
that takes no bans), `403` (non-loopback client), `405` (wrong method), `413` (body too large),
or `415` (not `application/json`). A binary from before this API answers `404` with a plain-text
body on these paths.

```bash
curl -s http://127.0.0.1:6061/debug/bans
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"ip":"203.0.113.7","store":"login-ban","seconds":600}' http://127.0.0.1:6061/debug/bans/add
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"ip":"203.0.113.7","store":"game-accept-filter"}' http://127.0.0.1:6060/debug/bans/remove
```

Every add and remove is logged at warn level with the IP, store, duration and outcome.

**API bans are not persisted.** They live in memory and are gone after a restart on both servers.
The loginserver rebuilds `login-ban` from `-banned-ips` at boot and never writes that file, so a
permanent login ban that must survive a restart also goes into `banned_ips.properties` (one IP per
line). The gameserver has no ban file and starts with an empty `game-ban`. Removing a
`banned_ips.properties` entry through the API lifts it until the next restart only.

### Character Item API

The gameserver's debug listener also serves a character item API, so an operator can fix a
character's inventory and paperdoll while the server runs, whether the character is in the world or
not. Like the ban API it is on only with `-debug-addr`, answers only loopback clients, reads with
`GET`, and writes with `POST`, `Content-Type: application/json` and a body of at most 4 KiB with no
unknown fields. The loginserver does not serve it.

`GET /debug/items?character_id=268435457` lists the character's `INVENTORY` and `PAPERDOLL` items,
by object id:

```json
{
  "character_id": 268435457,
  "name": "YourChar",
  "online": false,
  "items": [
    {"object_id": 268435460, "item_id": 57, "count": 1500, "enchant": 0, "location": "INVENTORY", "slot": -1, "augmented": false, "mana": -1},
    {"object_id": 268435461, "item_id": 2369, "count": 1, "enchant": 3, "location": "PAPERDOLL", "slot": 7, "augmented": false, "mana": -1}
  ]
}
```

`slot` is the paperdoll slot of a worn item (`-1` otherwise), `mana` a shadow item's remaining mana in
seconds (`-1` otherwise). Every write answers with the same list as it stands after the change.

| Path | Body |
| --- | --- |
| `/debug/items/add` | `{"character_id":268435457,"item_id":57,"count":1000,"enchant":0}` |
| `/debug/items/edit` | `{"character_id":…,"object_id":…,"expect":{…},"count":5}`, `"enchant":3`, or both |
| `/debug/items/delete` | `{"character_id":…,"object_id":…,"expect":{…}}` |
| `/debug/items/equip` | `{"character_id":…,"object_id":…,"expect":{…}}` |
| `/debug/items/unequip` | `{"character_id":…,"object_id":…,"expect":{…}}` |

`expect` is the item as the caller last listed it:
`{"item_id":57,"count":1500,"enchant":0,"location":"INVENTORY"}`. When the item no longer matches
it, nothing changes and the answer is `409`: list again and decide again.

- **add** takes object ids from the server's own id allocator. A stackable item joins the held stack
  (the result may not pass 2147483647) and takes no enchant; any other item arrives as `count`
  instances, at most 100 per request, each at `enchant` (0 to 65535). A shadow item starts with its
  full duration. Herbs and cursed weapons are refused. A character in the world must have the
  inventory slots for it, as for `//create_item`, and sees the usual obtained-item message.
- **edit** sets a stack's count (1 or more; a non-stackable stays at 1) and/or the enchant level.
- **delete** removes the item whole, taking it off first when worn. The collar of a pet that is out
  is refused.
- **equip** and **unequip** follow the player's own paperdoll rules: a bow takes its arrows into the
  left hand and leaves them on the way off, a two-handed weapon clears the off hand, formal wear
  refuses some pieces, and a hero item goes on an active hero only.
- A cursed weapon is never edited, deleted, equipped or unequipped (`422`), and its holder puts on
  no other weapon or shield, nor formal wear.

For a character **in the world** the change goes through its live inventory on its own queue, as a
game master's commands do: the client gets its InventoryUpdate, an equipment change also its
UserInfo (and CharInfo to the players around), and the item persistence task writes the rows on its
next tick.

For a character **out of the world** the edit is a login without the session. It takes the claim a
login takes on that character, so a login meanwhile is refused (the client stays on the character
list and can try again) and an edit during a login answers `503`. It waits for every save the last
session queued, then, as one job on the character's persistence lane, reads the rows, resolves them
against item writes still held from that session (a stack destroyed in the world but not yet deleted
from the table stays gone), applies the change and writes the result the way a logout does. The rows
hold the change when the answer arrives. If that write fails, the answer is `500` and the change stays
queued for the item persistence task, as a failed logout write does; list before retrying.

Errors answer `{"error":"message"}` with `400` (bad request or a change that can never succeed),
`403`, `404` (no such character or item), `405`, `409` (stale `expect`), `413`, `415`, `422` (the
paperdoll or the character refused the change), `503` with `Retry-After: 1` (the character is
logging in, leaving the world, or did not answer in time; nothing changed) or `500`.

```bash
curl -s 'http://127.0.0.1:6060/debug/items?character_id=268435457'
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"character_id":268435457,"item_id":57,"count":1000}' http://127.0.0.1:6060/debug/items/add
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"character_id":268435457,"object_id":268435461,"expect":{"item_id":2369,"count":1,"enchant":3,"location":"PAPERDOLL"},"enchant":5}' \
  http://127.0.0.1:6060/debug/items/edit
```

Every change is logged at info level (`item api: item changed`) with the character, the operation,
the object and item ids, and the count, enchant and location before and after.

[Profiling And GC Tuning](profiling.md) covers reading these profiles, spotting goroutine leaks, the load-test profiling harness, and why the servers keep the default GC settings.

## Smoke Checks

- Loginserver should log both listeners: one for game clients from `LoginserverHostname/LoginserverPort`, and one for game servers from `LoginHostname/LoginPort`.
- Gameserver should link to the loginserver and then log its game-client listener from `GameserverHostname/GameserverPort`.
- With `AutoCreateAccounts = True` or the key omitted, a fresh client login creates the account and reaches the server list.
- After selecting the linked gameserver, the client can create/select a character and enter the empty world.
- `./botswarm -bots 1 -actions 10 -duration 0` runs the same path without a client (see [Bot Swarm](#bot-swarm)) and exits 0 when the bot entered the world and logged out.

## Bot Swarm

`cmd/botswarm` runs simulated players against a running loginserver and gameserver over the real
client protocol: the login handshake (RSA credentials, Blowfish), the server list, the game-server
cipher, character creation or selection, `EnterWorld`, then a behavior cycle per bot. It is a
separate process, so it measures the servers the way real clients load them. The in-process
counterpart is the `tests/perf` harness in [Profiling And GC Tuning](profiling.md#profiling-under-load).

```bash
go build ./cmd/botswarm
# 50 bots, 5 new bots per second, 2 minutes, report also written as JSON
./botswarm -login 127.0.0.1:2106 -bots 50 -rate 5 -duration 2m -json swarm.json
```

Requirements and limits:

- One account per bot: bot `i` logs in as `<prefix><i>` (default prefix `bot`) with `-password`. A
  second login on an account already in play kicks the first, so two swarms need different prefixes.
- The loginserver must run with `AutoCreateAccounts = True` (the default), or the accounts must exist
  already (`cmd/accountmgr`). New accounts are hashed with bcrypt on the loginserver, so a high
  `-rate` on fresh accounts measures bcrypt as much as the server; rerun on the same prefix for
  login-path numbers without it. Failed logins count toward `LoginTryBeforeBan` for `127.0.0.1`.
- Account names hold at most 14 characters, so the prefix plus the last bot index must fit, using
  lowercase ASCII letters and digits. Each bot names its character after its account.
- A bot whose account has no character creates a human fighter and selects it; an account that
  already has a character selects its first slot.
- `-game-addr` replaces the game address the ServerList advertises, for a gameserver whose
  `GameserverHostname` is not reachable from the bot host.
- Both client ports (loginserver 2106, gameserver 7777) refuse connection bursts from one IP, so
  a swarm from one host is cut off after a few bots unless the filter is opened up. See
  [Connection accept filter](#connection-accept-filter).

### Connection Accept Filter

Each client port runs an accept filter that judges every new connection by its source IP before a
byte is exchanged. A connection that comes within the burst window of the same IP's previous one
counts as a try; the connection after the burst limit is reached bans the IP. A slower connection
is admitted but does not clear the tries. A banned IP's connections are closed unanswered, and each
attempt restarts its silence timer: only an IP that stays silent for the ban time is forgotten,
lifting the ban and clearing its tries. Every ban is logged at warn level with the IP and the
filter's settings. The [IP Ban API](#ip-ban-api) lists these bans and lifts one without a
restart.

`loginserver.properties` and `server.properties` take the same keys; each server reads its own file.
None is in the shipped files, and an absent key keeps its default:

| Key | Default | Meaning |
| --- | --- | --- |
| `EnableAcceptFilter` | `True` | `False` turns the filter off for that port. |
| `AcceptFilterBurstTime` | `1000` | Burst window, in milliseconds. |
| `AcceptFilterBurstLimit` | `3` | Burst connections an IP may open after its first one; the next one bans it. |
| `AcceptFilterBanTime` | `300` | Seconds of silence after which an IP is forgotten, lifting its ban. |
| `AcceptFilterAllowlist` | empty | IPs and CIDR prefixes (`127.0.0.1, 10.0.0.0/8`), separated by commas, semicolons or spaces, that the filter always admits. |

So by default an IP may open four connections less than a second apart each; the fifth bans it for
as long as it keeps trying plus 300 s. Burst time, limit and ban time must not be negative; a
malformed value or allowlist entry fails the boot. The loginserver's `EnableFloodProtection`,
`FastConnection*`, `NormalConnectionTime` and `MaxConnectionPerIP` keys guard only the game-server
link port (9014), not login clients.

For a load test, allowlist the swarm host in both files:

```properties
# loginserver.properties and server.properties
AcceptFilterAllowlist = 127.0.0.1
```

Raising `AcceptFilterBurstLimit` instead (e.g. to the bot count) keeps the filter on for other
hosts. `EnableAcceptFilter = False` drops it for every host; do not use it on a public server.

Behaviors (`-behaviors`, default `move,validate,chat,attack,idle`) run one per `-interval` tick, each
bot starting at its own offset into the cycle: `move` clicks a point within `-move-radius` of the
bot, `validate` sends `ValidatePosition`, `chat` says a line on all-chat, `attack` targets and
attacks the nearest attackable NPC the bot has seen (counted as `attack_no_target` when none is in
sight), `idle` sends nothing. A dead bot asks to restart in town instead. A bot stops after
`-actions` ticks or at `-duration`, whichever comes first, then sends `Logout`; a refused logout (in
combat) or no answer within `-timeout` closes the connection.

The report lists the bot, started and entered-world counts (a `-duration` that ends during the
`-rate` ramp leaves the remaining bots unstarted; they are counted in neither errors nor `aborted`), connect (TCP plus Init), login (Init to `PlayOk`)
and enter-world (game connect to `UserInfo`, character creation included) latency as p50, p99
and max, packets sent and received in total and per second, action counts, disconnects (the server
closed an in-world bot), errors by phase (`login`, `game`) with up to ten distinct samples, and bots
the run's end interrupted before entering (`aborted`). `-json <file>` (or `-json -` for stdout)
writes the same fields as JSON. The exit code is 0 when every bot entered the world with no error
or disconnect, 1 otherwise, 2 on a usage error.

`tests/botswarm` runs a few bots end to end against an in-process server through this tool's code
(`make test-one PKG=tests/botswarm` with the test database up).

## Game Master Access And Audit Log

### Access levels

A character's access level is the `characters.accesslevel` column. New characters start at `0`, the user level. The gameserver reads the column from the row each time the character is selected on the character-select screen.

The levels are defined in `../aCis_datapack/data/xml/accessLevels.xml`. The shipped table:

| Level | Name | `isGM` | `childLevel` |
|-------|------|--------|--------------|
| -1 | Banned | false | |
| 0 | User | false | 0 |
| 1 | Chat Moderator | false | 0 |
| 2 | Test GM | false | 1 |
| 3 | General GM | false | 2 |
| 4 | Support GM | false | 3 |
| 5 | Event GM | false | 4 |
| 6 | Head GM | false | 5 |
| 7 | Admin | true | 6 |
| 8 | Master | true | 7 |

- **Command rights.** `../aCis_datapack/data/xml/adminCommands.xml` gives each `//command` the level it needs (its `accessLevel` attribute). A character can run a command when its own level is that level, or when the `childLevel` chain reaches it: a level's child, that child's child, and so on. Every shipped command needs level 7, so only levels 7 (Admin) and 8 (Master, whose child is 7) can run admin commands.
- **GM status.** `isGM="true"` makes a character a game master. A GM is added to the `/gmlist` roster, can see invisible characters, and gets the `players.properties` login modes below.
- **Highest level.** The highest level in the table is the master level, and a master login is logged.
- **Effects of a positive level.** Any level above 0 replaces the character's title with the level's name. The name and title colors come from `nameColor` and `titleColor`.
- **Negative and undefined levels.** Any negative level reads as `-1`, and such a character cannot be selected. A level that is not in the table plays as level 0, and each time the character is selected the console log gets the warning `access level: undefined level granted, reset to the user level`.

To make a character the master, log that character out of the game first, then run the following against the `acis` schema loaded in [Prerequisites](#prerequisites):

```bash
mysql -uroot acis -e "UPDATE characters SET accesslevel = 8 WHERE char_name = 'YourChar'"
```

Select the character again and enter the world. Then type `//admin` in chat to open the admin panel. A character without the rights gets `You don't have the access right to use this command.` The console log then gets a `warn` event, `admin: tried to use an admin command without access to it`.

After one GM exists, that GM can grant levels in game:

- `//set access <level>` sets the level of the selected player, or the GM's own level when nothing is selected.
- `//set access <name> <level>` sets the level of a named character, online or offline.

The change is saved to `characters.accesslevel`.

`players.properties` also has a `DefaultAccessLevel` key, meant to set the level of new characters. The gameserver does not read it yet (#3721), so new characters always start at 0.

### GM audit log

Set this in `../aCis_gameserver/config/server.properties` and restart the gameserver:

```properties
GMAudit = True
```

The setting is read only at boot. It is `False` in the shipped file, and then nothing is recorded. With it on, the audit log records every admin command that passes its access check, just before the command runs. That includes commands the server lists in `adminCommands.xml` but has not implemented yet. A command that is refused for missing rights, or that is unknown, is not recorded.

The file is the `GMAuditLogHandler` pattern in `logging.properties`, `log/gmaudit/gmaudit_%g.txt`, resolved against `-log-root`. With the start command above it is `log/gameserver/log/gmaudit/gmaudit_0.txt`. The file is created at boot even with `GMAudit = False`. It is appended to across restarts, and rotation is described in [Logs, Metrics, And Failure Modes](observability.md#log-files).

Each record is one JSON line, and its `msg` field has this format:

```text
<name> [<objectId>] used '<command>' command on: <target>
```

- `<command>` is the text as the server received it. A command typed in chat as `//admin 2` is recorded as `admin 2`. A link clicked in the admin panel is recorded with its `admin_` prefix, for example `admin_admin`.
- `<target>` is the selected character's name, the object id of any other selected object, or `none` when nothing is selected.

```bash
jq -r '"\(.time) \(.msg)"' log/gameserver/log/gmaudit/gmaudit_0.txt
# 2026-10-12T12:00:00+03:00 YourChar [268435457] used 'admin' command on: none
```

### GM login modes

These `players.properties` keys apply when a character whose level has `isGM="true"` enters the world. They have no effect on other characters. The keys are read only at boot.

| Key | Shipped value | Effect at login |
|-----|---------------|-----------------|
| `GMStartupInvulnerable` | `False` | the GM is invulnerable |
| `GMStartupInvisible` | `False` | the GM is hidden from other players |
| `GMStartupBlockAll` | `False` | the GM starts in block-all mode, as after `/blockall` |
| `GMStartupAutoList` | `True` | every player sees the GM on `/gmlist`. With `False`, the GM is listed hidden: only other GMs see it, marked `(invis)`. In game, `//gmlist` switches between the two. |
| `GMHeroAura` | `False` | the GM shows the hero aura. This is not a login mode: it applies whenever the GM is drawn. |
