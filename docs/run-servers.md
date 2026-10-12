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
- `server.properties` has `LoginHost = 127.0.0.1` and `LoginPort = 9014`, matching `loginserver.properties`.
- Optional: `MaxConnections` in `loginserver.properties` / `server.properties` sets that process's database pool size. The key is absent from the shipped files; omitting it keeps the current pool size of 8. Values below 1 are rejected at boot.
- XML datapack files stay on disk under `../aCis_datapack`; do not import them into the database.
- Geodata files stay on disk under `../aCis_datapack/data/geodata`. The shipped `GeoDataPath = ./data/geodata/` is resolved against `-data-root`, so L2OFF files should be named like `16_10_conv.dat` and L2J files like `16_10.l2j`.

## Build

```bash
go build ./cmd/loginserver ./cmd/gameserver ./cmd/gsregister
```

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

The loginserver binds the client listener from `LoginserverHostname/LoginserverPort` and the gameserver-link listener from `LoginHostname/LoginPort`.

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

[Profiling And GC Tuning](profiling.md) covers reading these profiles, spotting goroutine leaks, the load-test profiling harness, and why the servers keep the default GC settings.

## Smoke Checks

- Loginserver should log both listeners: one for game clients from `LoginserverHostname/LoginserverPort`, and one for game servers from `LoginHostname/LoginPort`.
- Gameserver should link to the loginserver and then log its game-client listener from `GameserverHostname/GameserverPort`.
- With `AutoCreateAccounts = True` or the key omitted, a fresh client login creates the account and reaches the server list.
- After selecting the linked gameserver, the client can create/select a character and enter the empty world.

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
