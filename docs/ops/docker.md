# Running with Docker Compose

`docker-compose.yml` runs MariaDB, the login server and the game server. The `Dockerfile` builds
one image that holds the four binaries (`loginserver`, `gameserver`, `gsregister`,
`accountmgr`), plus a default config set and the `acis-init` script that installs it. Config and
datapack stay as files on the host, mounted into the containers. To run
the binaries directly instead, see [run-servers.md](../run-servers.md).

| File | Role |
| --- | --- |
| `Dockerfile` | Builds static binaries into a small Alpine image. Its working directory is `/opt/acis`, with `config/`, `data/` and `log/` mounted, so every flag default resolves as it does in a reference server directory. |
| `docker-compose.yml` | Runs `db` on its own (`docker compose up -d`, as before) and adds `loginserver` and `gameserver` under the `servers` profile. |
| `ops/docker/initdb/10-acis-schema.sh` | Loads the aCis schema the first time an empty database volume starts. |
| `ops/docker/config/` | The config bundle: every file both servers read, already pointed at the compose services. The image carries it in `/usr/local/share/acis/config`. |
| `ops/docker/acis-init.sh` | The `init` service (`acis-init` in the image). On every `up`, before the servers start, it fills an empty config directory from the bundle, writes the compose and per-stack keys, and registers game server 1. |
| `ops/docker/init-config.sh` | Optional. Copies a reference `config/` and points it at the compose services. |

## Requirements

- Docker Engine with Compose v2 (`docker compose version`).
- An `aCis_datapack` checkout. Compose reads its `sql/` and `tools/` to create the database and
  mounts its `data/` (`xml/`, `html/`, `geodata/`, `serverNames.xml`) into both servers.
- Optionally, the reference server's `config/` directory (`aCis_gameserver/config` in the aCis
  sources). Without one, the `init` service installs the config bundle.
- By default the datapack is `../aCis_datapack`, next to this checkout. The repository's own `aCis_datapack/` is a test copy without geodata; do not point
  servers at it. Point compose elsewhere with `ACIS_DATAPACK_DIR`.
- The repository copy also holds server additions an upstream checkout lacks (listed in
  `docs/agents/verification.md`, such as `data/xml/spawnlist/custom_classmaster.xml`). Copy them
  into the checkout compose mounts; the deploy package already includes them.

## First start

Run these from the checkout root. A stack needs only `docker-compose.yml`, `ops/docker/initdb/`,
the image and a datapack; the rest of the checkout is only needed to build the image.

```bash
# 1. Settings compose reads. Keep them in .env (ignored by git).
cat > .env <<'END'
ACIS_DATAPACK_DIR=../aCis_datapack
ACIS_DB_ROOT_PASSWORD=change-me
ACIS_PUBLIC_HOST=127.0.0.1
TZ=Europe/Berlin
END

# 2. Server image.
docker compose --profile servers build

# 3. Everything. db loads the schema on its first start (under a minute); then init writes
#    ./config and registers game server 1 (config/hexid.txt); then both servers start.
docker compose --profile servers up -d
docker compose logs -f gameserver
```

The game server is up once it has logged both `listening for game clients` and `linked to loginserver`.
The login link is made in the background, so `linked to loginserver` usually comes last.
Point a client at `ACIS_PUBLIC_HOST`, port 2106. With `AutoCreateAccounts = True` (the reference
default), the first login creates the account.

To start from a reference server's config instead of the bundle, create `./config` before step 3:

```bash
set -a; . ./.env; set +a
ops/docker/init-config.sh ../aCis_gameserver/config
```

### The init service

`init` runs `acis-init` once per `docker compose up`, after `db` is healthy and before both
servers, which wait for it to finish successfully. It runs as root only to create and `chown`
directories, and does this:

1. When the config directory has no `server.properties`, it copies the bundle there. An existing
   config, such as the one `init-config.sh` wrote, is kept.
2. It writes the keys the compose network fixes, on every run:

   | File | Key | Value |
   | --- | --- | --- |
   | both | `URL` | `jdbc:mariadb://db/acis` |
   | both | `Login` | `root` |
   | both | `Password` | `$ACIS_DB_ROOT_PASSWORD` (default `123321`) |
   | `server.properties` | `LoginHost` | `loginserver` |
   | `server.properties` | `GameserverPort` | `$ACIS_GAME_PORT` (default `7777`) |

3. It writes these keys only when their variable is set and not empty, so a stack can change them
   without editing files. An unset variable leaves the file's value. A key the file lacks is
   appended:

   | Variable | File | Key |
   | --- | --- | --- |
   | `ACIS_PUBLIC_HOST` | `server.properties` | `Hostname`, the address clients are given for the game server |
   | `ACIS_GAMESERVER_HOSTNAME` | `server.properties` | `GameserverHostname`, the game server's bind address |
   | `ACIS_MAX_ONLINE_USERS` | `server.properties` | `MaximumOnlineUsers` |
   | `ACIS_AUTO_CREATE_ACCOUNTS` | `loginserver.properties` | `AutoCreateAccounts` |
   | `ACIS_LOGIN_TRY_BEFORE_BAN` | `loginserver.properties` | `LoginTryBeforeBan` |
   | `ACIS_ACCEPT_FILTER` | both | `EnableAcceptFilter` |
   | `ACIS_ACCEPT_FILTER_ALLOWLIST` | both | `AcceptFilterAllowlist` |
   | `ACIS_GEO_REGIONS` | `geoengine.properties` | The region list: regions such as `16_10 17_11`, separated by spaces or commas, replace every enabled region line. `none` enables none, for a datapack without geodata. |

   Values are written verbatim and may not hold a newline or a backslash.
4. It registers game server id 1 and writes `config/hexid.txt` with
   `gsregister -register 1 -out config/hexid.txt`. A rerun over a matching registration changes
   nothing. A registration without the file rewrites the file from the database; a file without
   the registration (the database was recreated) registers the file's key. A file that names
   another server or another key than the registration fails the service, and the servers do not
   start.
5. It creates `log/loginserver`, `log/gameserver`, `cache` and the datapack's `data/crests`, and
   gives them, the log root and the config directory to `ACIS_UID:ACIS_GID`.

`init-config.sh` changes only these keys and copies every other key and file unchanged:

| File | Key | Value | Why |
| --- | --- | --- | --- |
| both | `URL` | `jdbc:mariadb://db/acis` | The database is the `db` service, not `localhost`. |
| both | `Login` | `root` | The `db` service creates only `root`. A dedicated user from a migrated Java config does not exist there. |
| both | `Password` | `$ACIS_DB_ROOT_PASSWORD` (default `123321`) | Must match the `db` container's root password. |
| `server.properties` | `LoginHost` | `loginserver` | The game server links to the `loginserver` service. |
| `server.properties` | `Hostname` | `$ACIS_PUBLIC_HOST` (default `127.0.0.1`) | The address the login server gives clients for the game server. The reference `*` would give them the game container's internal address. |

The script also creates `log/loginserver`, `log/gameserver`, `cache` and the datapack's `data/crests`. It
refuses to run when `config/server.properties` already exists, so it never undoes your edits. To
change a value later, edit `config/*.properties` (or set the variable the init service reads) and
restart the stack with `docker compose --profile servers up -d`.

### Registering by hand

The init service registers server 1 on its own. To register from a shell, without prompts:

```bash
docker compose --profile servers run --rm --no-deps loginserver \
  gsregister -config config/loginserver.properties -names data/serverNames.xml \
  -register 1 -out config/hexid.txt
```

The interactive tool still works and writes `hexid(server N).txt` for you to install:

```bash
printf '1\nexit\n' | docker compose --profile servers run --rm --no-deps -T -w /opt/acis/config \
  loginserver gsregister -config loginserver.properties -names ../data/serverNames.xml
mv 'config/hexid(server 1).txt' config/hexid.txt
```

## Settings

Compose reads these from the environment or from `.env`:

| Variable | Default | Meaning |
| --- | --- | --- |
| `ACIS_DATAPACK_DIR` | `../aCis_datapack` | Datapack checkout. `sql/` and `tools/` create the schema, and `data/` is mounted read-write, because the game server writes clan crests to `data/crests/` and admin announcement edits to `data/xml/announcements.xml`. |
| `ACIS_CONFIG_DIR` | `./config` | Config directory, mounted read-write at `/opt/acis/config`. The `init` service fills it from the bundle when it holds no `server.properties`. |
| `ACIS_LOG_DIR` | `./log` | Log root. Each server writes the files named in `logging.properties` under its own `loginserver/` or `gameserver/` subdirectory. |
| `ACIS_CACHE_DIR` | `./cache` | Game server cache directory, mounted at `/opt/acis/cache`. The game server keeps the packed geodata cache there (`-geodata-cache cache/geodata.cache`): the first boot builds it from `data/geodata/`, later boots map it instead of loading every region onto the heap. Deleting it only costs one rebuild. |
| `ACIS_DB_ROOT_PASSWORD` | `123321` | MariaDB root password, applied only when the volume is first created. |
| `ACIS_DB_BIND`, `ACIS_DB_PORT` | `127.0.0.1`, `3306` | Host address and port the database is published on. Leave the address on loopback. |
| `ACIS_MARIADB_TAG` | `11` | MariaDB image tag. CI tests against MariaDB 11. |
| `ACIS_UID`, `ACIS_GID` | `1000` | Host user the servers run as. The `init` service gives it the config, log, cache and `data/crests` directories. |
| `ACIS_IMAGE` | `acis-golang:local` | Server image, used by `init`, `loginserver` and `gameserver`. The default is built from this checkout. |
| `ACIS_LOGIN_PORT` | `2106` | Host port for login clients. |
| `ACIS_GAME_PORT` | `7777` | Game client port, published on the host and listened on in the container. The `init` service writes it to `GameserverPort`, the port the login server hands to clients, so the two cannot disagree. |
| `ACIS_PUBLIC_HOST`, `ACIS_GAMESERVER_HOSTNAME`, `ACIS_MAX_ONLINE_USERS`, `ACIS_AUTO_CREATE_ACCOUNTS`, `ACIS_LOGIN_TRY_BEFORE_BAN`, `ACIS_ACCEPT_FILTER`, `ACIS_ACCEPT_FILTER_ALLOWLIST`, `ACIS_GEO_REGIONS` | unset | Per-stack config keys, written by the `init` service when set ([The init service](#the-init-service)). `init-config.sh` also reads `ACIS_PUBLIC_HOST` (default `127.0.0.1`). |
| `TZ` | `UTC` | Server time zone. Seven Signs, Olympiad, sieges, the manor, the lottery and the fishing championship run on local wall-clock time. Set the zone the reference server ran in. |

The published ports are `ACIS_LOGIN_PORT` (login clients, default 2106) and `ACIS_GAME_PORT`
(game clients, default 7777). Port 9014, the link between the game server and the login server,
stays on the compose network. No service has a fixed container name: compose names containers
after the project (`-p`, `COMPOSE_PROJECT_NAME`, or the directory name), so two stacks with their
own project names, `ACIS_DB_PORT`, `ACIS_LOGIN_PORT` and `ACIS_GAME_PORT` run side by side.

## Day-to-day commands

```bash
docker compose --profile servers ps
docker compose logs -f --tail=100 gameserver          # JSON log lines
docker compose --profile servers restart gameserver
docker compose --profile servers stop                 # graceful: the servers save, then exit
git pull && docker compose --profile servers up -d --build   # update to a new version

# Accounts (interactive)
docker compose --profile servers run --rm loginserver accountmgr -config config/loginserver.properties
```

On `stop`, `restart` and `down`, Docker sends SIGTERM. The game server then closes every client
with `ServerClose`, saves players and world state, and exits. Its `stop_grace_period` (240s) is set
above the server's own worst-case shutdown (`gameServerStopTimeout`, currently 3m55s).
`TestComposeStopGraceCoversGameServerStop` fails if the two drift apart. The `servers` profile
uses `restart: unless-stopped`, so a crashed server comes back up, and so does one stopped
in game: `//server restart` (exit code 2) and `//server shutdown` (exit code 0) both end in a
restart under this policy. To keep the server down, stop it with `docker compose stop`.

CI runs `ops/docker/smoke-test.sh` on every change. It checks that `docker-compose.yml` parses,
that the image builds and each of its binaries starts, and that the `db` service's first start
loads every table file of a stub datapack. It also checks that a first start without
`tools/full_install.sql` exits non-zero, and that under `docker compose up` the `restart: always`
restart that follows serves an empty database but never reports healthy, and that a data directory
left by a first start killed before the schema load never reports healthy either. It
also boots a whole stack from a directory that holds only `docker-compose.yml`,
`ops/docker/initdb/` and a datapack, with `ACIS_GEO_REGIONS=none`, and waits for the game server to
link to the login server. It also loads the real schema and checks that `acis` holds one table per `sql/*.sql` file. That
schema comes from the repository's own `aCis_datapack/` copy, or from the checkout
`ACIS_SMOKE_DATAPACK` names; CI uses the copy and first checks it against its manifest
(`ops/datapack-manifest.sh check`). To run it locally, stop any stack that publishes the
same ports first (`ACIS_DB_PORT`, `ACIS_LOGIN_PORT`, `ACIS_GAME_PORT` move them):

```sh
ops/docker/smoke-test.sh
ACIS_SMOKE_DATAPACK=/path/to/aCis_datapack ops/docker/smoke-test.sh
```

Backups: [backup-restore.md](backup-restore.md). Moving an existing Java server:
[migrating-from-java.md](migrating-from-java.md).

## Failure modes

| Symptom | Cause and fix |
| --- | --- |
| `db` stays unhealthy, `loginserver` and `gameserver` never start, and `docker inspect --format '{{json .State.Health}}' $(docker compose ps -q db)` shows `acis schema load did not finish`. The first `db` log shows `acis schema: ... not found` or a failed load. | The first-start schema load failed, usually because `ACIS_DATAPACK_DIR` does not point at an `aCis_datapack` checkout. That start exits; `restart: always` starts `db` again, and since the data directory already exists, that start skips the schema load and serves an empty or partial `acis` database. The load leaves the marker `/var/lib/mysql/.acis-schema-incomplete` behind, and the healthcheck fails while it exists. Run `docker compose down -v` (it **deletes** the database volume), fix the path, and start again. |
| `db` stays unhealthy, `loginserver` and `gameserver` never start, and `docker inspect --format '{{json .State.Health}}' $(docker compose ps -q db)` shows `acis database has no tables`. The `db` log has no `acis schema:` lines. | The first `db` start was killed (out of memory, host reboot, `docker kill`) after MariaDB initialized the data directory but before the schema load ran. Every later start sees an initialized data directory and skips the load, so the `acis` database has no tables. Run `docker compose down -v` (it **deletes** the database volume) and start again. |
| `loginserver` and `gameserver` never start, and `docker compose --profile servers ps -a` shows `init` exited non-zero. | `docker compose logs init` names the cause: a missing datapack (`data/xml not found`), a bad per-stack value, or a `config/hexid.txt` that `differs from the registration of server 1`. For the last, the file and the `gameservers` table disagree; remove `config/hexid.txt` to rewrite it from the database, or unregister server 1 to register the file's key. |
| The game server exits with `open config/hexid.txt: no such file or directory`. | The server was started without the `init` service (for example with `--no-deps`), or a hand registration was not renamed to `hexid.txt`. Run `docker compose --profile servers up -d`. |
| The game server log shows `wrong hexid`, or it keeps retrying the login link. | `config/hexid.txt` does not match the `gameservers` table, for example after a hand registration while the login server ran. Run `docker compose --profile servers up -d --force-recreate`: `init` checks the registration, then **both** servers restart, and the login server reads the registered servers only at startup. |
| Clients see the server list, but entering the game times out. | `Hostname` in `config/server.properties` is not reachable from the client. Set it to the public IP or DNS name and restart `gameserver`. |
| `permission denied` on `log/...`, `cache/...` or `data/crests/...`, or the game server logs that it cannot build the packed geodata cache and loads the regions onto the heap. | `ACIS_UID` does not own the directory, for example after a server was started with `--no-deps`, which skips the `init` service that hands the directories over. Run `docker compose --profile servers up -d`, or `chown` the directory to `ACIS_UID:ACIS_GID`. |
| The game server boot fails and names a geodata file. | A region listed in `config/geoengine.properties` is missing or malformed under `data/geodata/`. Unlisted regions are not loaded. |
| `gameserver` exits with code 137 on `stop`. | Docker killed it before it finished saving. Keep `stop_grace_period` above `gameServerStopTimeout`. Check with `docker inspect --format '{{.State.ExitCode}}' <container>`. |
| Events start hours early or late. | `TZ` differs from the zone the schedules were set up in. |
| `db` refuses to start after this change and logs an upgrade or downgrade error. | The existing `mariadb_data` volume was created by a newer `mariadb:latest`. Set `ACIS_MARIADB_TAG=latest`, or the tag that created it. |
| `bind: address already in use` on 3306, 2106 or 7777. | A host service or another stack already uses the port. Set `ACIS_DB_PORT`, `ACIS_LOGIN_PORT` or `ACIS_GAME_PORT`, or stop the other service. |
