# Running with Docker Compose

`docker-compose.yml` runs MariaDB, the login server and the game server. The `Dockerfile` builds
one image that holds the four binaries (`loginserver`, `gameserver`, `gsregister`,
`accountmgr`). Config and datapack stay as files on the host, mounted into the containers. To run
the binaries directly instead, see [run-servers.md](../run-servers.md).

| File | Role |
| --- | --- |
| `Dockerfile` | Builds static binaries into a small Alpine image. Its working directory is `/opt/acis`, with `config/`, `data/` and `log/` mounted, so every flag default resolves as it does in a reference server directory. |
| `docker-compose.yml` | Runs `db` on its own (`docker compose up -d`, as before) and adds `loginserver` and `gameserver` under the `servers` profile. |
| `ops/docker/initdb/10-acis-schema.sh` | Loads the aCis schema the first time an empty database volume starts. |
| `ops/docker/init-config.sh` | Copies the reference `config/` and points it at the compose services. |

## Requirements

- Docker Engine with Compose v2 (`docker compose version`).
- An `aCis_datapack` checkout. Compose reads its `sql/` and `tools/` to create the database and
  mounts its `data/` (`xml/`, `html/`, `geodata/`, `serverNames.xml`) into both servers.
- The reference server's `config/` directory (`aCis_gameserver/config` in the aCis sources).
- By default both paths are `../aCis_datapack` and `../aCis_gameserver/config`, next to this
  checkout. Point compose and the init script elsewhere with `ACIS_DATAPACK_DIR` and the
  script's first argument.

## First start

Run these from the checkout root.

```bash
# 1. Settings that compose and init-config.sh both read. Keep them in .env (ignored by git);
#    `set -a` exports them to this shell too.
cat > .env <<'EOF'
ACIS_DATAPACK_DIR=../aCis_datapack
ACIS_DB_ROOT_PASSWORD=change-me
ACIS_PUBLIC_HOST=127.0.0.1
TZ=Europe/Berlin
EOF
set -a; . ./.env; set +a

# 2. Server config in ./config, pointed at the db and loginserver services.
ops/docker/init-config.sh ../aCis_gameserver/config

# 3. Server image, and the database. The database's first start loads the schema (under a minute).
docker compose --profile servers build
docker compose up -d --wait db

# 4. Register game server id 1. This writes config/hexid(server 1).txt.
printf '1\nexit\n' | docker compose --profile servers run --rm -T -w /opt/acis/config \
  loginserver gsregister -config loginserver.properties -names ../data/serverNames.xml
mv 'config/hexid(server 1).txt' config/hexid.txt

# 5. Both servers.
docker compose --profile servers up -d --build
docker compose logs -f gameserver
```

The game server is up once it logs `linked to loginserver` and then `listening for game clients`.
Point a client at `ACIS_PUBLIC_HOST`, port 2106. With `AutoCreateAccounts = True` (the reference
default), the first login creates the account.

`init-config.sh` changes only these keys and copies every other key and file unchanged:

| File | Key | Value | Why |
| --- | --- | --- | --- |
| both | `URL` | `jdbc:mariadb://db/acis` | The database is the `db` service, not `localhost`. |
| both | `Password` | `$ACIS_DB_ROOT_PASSWORD` (default `123321`) | Must match the `db` container's root password. |
| `server.properties` | `LoginHost` | `loginserver` | The game server links to the `loginserver` service. |
| `server.properties` | `Hostname` | `$ACIS_PUBLIC_HOST` (default `127.0.0.1`) | The address the login server gives clients for the game server. The reference `*` would give them the game container's internal address. |

The script also creates `log/loginserver`, `log/gameserver` and the datapack's `data/crests`. It
refuses to run when `config/server.properties` already exists, so it never undoes your edits. To
change a value later, edit `config/*.properties` and restart the service.

## Settings

Compose reads these from the environment or from `.env`:

| Variable | Default | Meaning |
| --- | --- | --- |
| `ACIS_DATAPACK_DIR` | `../aCis_datapack` | Datapack checkout. `sql/` and `tools/` create the schema, and `data/` is mounted read-write, because the game server writes clan crests to `data/crests/` and admin announcement edits to `data/xml/announcements.xml`. |
| `ACIS_CONFIG_DIR` | `./config` | Config directory, mounted read-write at `/opt/acis/config`. |
| `ACIS_LOG_DIR` | `./log` | Log root. Each server writes the files named in `logging.properties` under its own `loginserver/` or `gameserver/` subdirectory. |
| `ACIS_DB_ROOT_PASSWORD` | `123321` | MariaDB root password, applied only when the volume is first created. |
| `ACIS_DB_BIND`, `ACIS_DB_PORT` | `127.0.0.1`, `3306` | Host address and port the database is published on. Leave the address on loopback. |
| `ACIS_MARIADB_TAG` | `11` | MariaDB image tag. CI tests against MariaDB 11. |
| `ACIS_UID`, `ACIS_GID` | `1000` | Host user the servers run as. It needs write access to the config, log and `data/crests` directories. |
| `TZ` | `UTC` | Server time zone. Seven Signs, Olympiad, sieges, the manor, the lottery and the fishing championship run on local wall-clock time. Set the zone the reference server ran in. |

The published ports are 2106 (login clients) and 7777 (game clients). Port 9014, the link between
the game server and the login server, stays on the compose network.

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
above the server's own worst-case shutdown (`gameServerStopTimeout`, currently 3m35s).
`TestComposeStopGraceCoversGameServerStop` fails if the two drift apart. The `servers` profile
uses `restart: unless-stopped`, so a crashed server comes back up. The reference loop scripts
also restarted on the admin restart exit code. The Go server has no admin shutdown or restart
command yet ([#3369](https://github.com/fatal10110/acis_golang/issues/3369)).

Backups: [backup-restore.md](backup-restore.md). Moving an existing Java server:
[migrating-from-java.md](migrating-from-java.md).

## Failure modes

| Symptom | Cause and fix |
| --- | --- |
| `db` exits on first start and its log shows `acis schema: ... not found`. | `ACIS_DATAPACK_DIR` does not point at an `aCis_datapack` checkout. The data directory was already created, so the next start skips the schema load. Run `docker compose down -v` (it **deletes** the database volume), fix the path, and start again. |
| The game server exits with `open config/hexid.txt: no such file or directory`. | Step 4 was skipped, or the file was not renamed to `hexid.txt`. |
| The game server log shows `wrong hexid`, or it keeps retrying the login link. | `config/hexid.txt` does not match the `gameservers` table, for example after the database was recreated. Register again (step 4), then restart **both** services: the login server reads the registered servers only at startup. |
| Clients see the server list, but entering the game times out. | `Hostname` in `config/server.properties` is not reachable from the client. Set it to the public IP or DNS name and restart `gameserver`. |
| `permission denied` on `log/...` or `data/crests/...`. | Docker created a missing bind-mount directory as root, or `ACIS_UID` does not own the directory. Run `ops/docker/init-config.sh` first, or `chown` the directory to `ACIS_UID:ACIS_GID`. |
| The game server boot fails and names a geodata file. | A region listed in `config/geoengine.properties` is missing or malformed under `data/geodata/`. Unlisted regions are not loaded. |
| `gameserver` exits with code 137 on `stop`. | Docker killed it before it finished saving. Keep `stop_grace_period` above `gameServerStopTimeout`. Check with `docker inspect --format '{{.State.ExitCode}}' <container>`. |
| Events start hours early or late. | `TZ` differs from the zone the schedules were set up in. |
| `db` refuses to start after this change and logs an upgrade or downgrade error. | The existing `mariadb_data` volume was created by a newer `mariadb:latest`. Set `ACIS_MARIADB_TAG=latest`, or the tag that created it. |
| `bind: address already in use` on 3306. | A host MariaDB already uses the port. Set `ACIS_DB_PORT`, or stop the host service. |
