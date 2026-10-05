# Migrating from the aCis Java server

The Go servers read the reference server's files as they are. You do not convert, import or rename
anything:

- **Database**: the same MariaDB `acis` schema, created by the same `aCis_datapack/sql` files. The
  Go servers run no migrations and never create or alter tables, so an existing database works
  unchanged. Account passwords are bcrypt hashes in both servers.
- **Config**: the same `config/*.properties` files and keys. A key the Go server reads but does not
  find is logged as `config property missing; using default value`, with the default it uses. The
  few keys it reads but does not apply yet are logged by name and issue at boot (for example
  `siege.properties keys are read but not applied yet`).
- **Datapack**: the same `data/xml`, `data/html`, `data/crests` and `serverNames.xml`.
- **Geodata**: the same `data/geodata` files, `GeoDataPath` and `GeoDataType`. Exactly the regions
  listed in `geoengine.properties` are loaded.
- **Registration**: the same `hexid.txt` and `gameservers` row, so you do not register the game
  server again.

## Layout: same directories, same relative paths

The reference build (`ant dist` in `aCis_gameserver` and `aCis_datapack`) produces a `login/` and a
`gameserver/` directory, and each server runs from its own directory. The Go binaries default
every path to the same relative location, so you place each binary in the directory its Java
counterpart ran from and start it with no flags:

| Reference file, relative to the server directory | Go flag | Go default |
| --- | --- | --- |
| `login/config/loginserver.properties` | `loginserver -config` | `config/loginserver.properties` |
| `login/config/logging.properties` | `loginserver -logging` | `config/logging.properties` |
| `login/config/banned_ips.properties` | `loginserver -banned-ips` | `config/banned_ips.properties` |
| `login/serverNames.xml` | `loginserver -server-names`, `gsregister -names` | `serverNames.xml` |
| `login/log/` | `loginserver -log-root` | `.` (patterns in `logging.properties` start with `log/`) |
| `gameserver/config/server.properties` | `gameserver -config` | `config/server.properties` |
| `gameserver/config/{players,npcs,clans,events,siege,geoengine,logging}.properties` | `gameserver -players-config`, `-npcs-config`, `-clans-config`, `-events-config`, `-siege-config`, `-geo-config`, `-logging` | `config/<name>.properties` |
| `gameserver/config/hexid.txt` | `gameserver -hexid` | `config/hexid.txt` |
| `gameserver/data/` (`xml/`, `html/`, `geodata/`, `crests/`) | `gameserver -data-root` (the directory that **contains** `data/`) | `.` |
| `gameserver/log/` | `gameserver -log-root` | `.` |

`GeoDataPath = ./data/geodata/` is resolved against `-data-root`, as the reference resolves it
against its working directory.

## Steps

1. **Take a cold backup** of the Java server. Stop both Java servers, then follow the
   [native backup](backup-restore.md#without-docker) steps. The Go servers write the same tables
   and files the Java server does. This backup is your way back.
2. **Build the binaries** on any machine with Go (see `go.mod` for the version):

   ```bash
   CGO_ENABLED=0 go build -trimpath -o dist/ ./cmd/gameserver ./cmd/loginserver ./cmd/gsregister ./cmd/accountmgr
   ```

   Static binaries need no runtime on the server. Cross-compile with `GOOS=linux GOARCH=amd64`.
3. **Install them next to the Java files.** Copy `loginserver`, `gsregister` and `accountmgr` into
   `login/`, and `gameserver` into `gameserver/`. The jars and `libs/` are no longer used.
4. **Start them** from those directories, the login server first:

   ```bash
   (cd login && ./loginserver)
   (cd gameserver && ./gameserver)
   ```

5. **Check the boot.** The login server logs `listening for gameservers` and `listening for login
   clients`. The game server logs its data loads, then `listening for game clients`, and then,
   once the login link is up, `linked to loginserver` with the server id from `hexid.txt`. Log in
   with an existing account and enter the world with an existing character.

### What replaces the reference scripts

| Reference | Go |
| --- | --- |
| `startLoginServer.sh`, `LoginServer_loop.sh` | Run `./loginserver` under a supervisor (systemd, Docker). |
| `startGameServer.sh`, `GameServer_loop.sh` (restarts on exit code 2) | Run `./gameserver` under a supervisor with automatic restart. Admin-requested shutdown and restart (`//server_shutdown`, `//server_restart`) are not ported yet ([#3369](https://github.com/fatal10110/acis_golang/issues/3369)), so the Go server exits only on SIGTERM or SIGINT, or on a fatal error. |
| `RegisterGameServer.sh` | `./gsregister` in `login/`: the same prompts and the same `hexid(server N).txt` output. |
| `startSQLAccountManager.sh` | `./accountmgr` in `login/`. |
| `java -Xmx2G` | No heap flag is needed. Set `GOMEMLIMIT` (for example `GOMEMLIMIT=1500MiB`) to keep the garbage collector under a memory budget. |
| `log/stdout.log` redirect | stdout carries the JSON log lines, one object per line, plus a dependency-injection trace at boot and stop. The `log/<category>/` files from `logging.properties` are still written. |

A minimal systemd unit for the game server:

```ini
[Unit]
Description=aCis game server
After=network-online.target mariadb.service acis-loginserver.service
Wants=acis-loginserver.service

[Service]
User=acis
WorkingDirectory=/opt/acis/gameserver
ExecStart=/opt/acis/gameserver/gameserver
Restart=always
RestartSec=5
# Above the server's own worst-case graceful shutdown; see ops/systemd/.
TimeoutStopSec=240

[Install]
WantedBy=multi-user.target
```

The login server unit is the same with `WorkingDirectory=/opt/acis/login` and
`ExecStart=/opt/acis/login/loginserver`, without the `acis-loginserver` dependency.

## Moving into Docker instead

[docker.md](docker.md) uses one config directory for both servers and an `aCis_datapack` checkout
(it needs `data/`, `sql/` and `tools/` side by side). To move a Java installation:

1. Take the cold backup (step 1 above).
2. Merge the two config directories: copy `gameserver/config/*`, then `login/config/loginserver.properties`
   and `login/config/banned_ips.properties`, into one directory. `hexid.txt` comes along with
   them. Run `ops/docker/init-config.sh <that directory>`, then skip the registration step. The
   script sets `Login = root` and the `db` root password, because the `db` service creates no
   other user and the dump does not carry MariaDB users. A dedicated Java DB user is not needed.
3. Copy `gameserver/data/crests/` and `gameserver/data/xml/announcements.xml` into the datapack
   checkout you point `ACIS_DATAPACK_DIR` at. Use the same datapack version the Java server ran.
4. `docker compose up -d --wait db`, then load the dump as in
   [Restoring](backup-restore.md#restoring), then `docker compose --profile servers up -d --build`.

## Verifying data parity

`cmd/datadiff` writes field-level dumps of the datapack tables as the Go server loads them. Use it
to check that the datapack your server runs from loads the same as a known checkout:

```bash
go run ./cmd/datadiff -list                                     # categories it covers
go run ./cmd/datadiff -datapack ../aCis_datapack -category=skill > skill.dump
go run ./cmd/datadiff -datapack /opt/acis/gameserver -category=skill -expected-dump=skill.dump
```

`-datapack` names the directory that contains `data/`.

Malformed or missing data stops the boot with an error that names the file. Examples: a missing
`hexid.txt` (`open config/hexid.txt: no such file or directory`), a listed geodata region that is
missing or unreadable (the error names each file, `GeoDataPath` and `GeoDataType`), or an
unparseable config value (the error names the key).

## Rolling back

Stop the Go servers and start the Java servers again from the same directories. Both read the same
database and files. If anything looks wrong after running the Go servers, restore the cold backup
from step 1 before you start Java.

## Known differences

The port is still in progress. Systems the Go server does not run yet are tracked as open issues
under the [milestones](https://github.com/fatal10110/acis_golang/milestones). Check them before you
move a live server.
