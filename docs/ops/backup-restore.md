# Backup and restore

The commands below are for the Docker Compose setup ([docker.md](docker.md)). They run from the
checkout root with `.env` loaded (`set -a; . ./.env; set +a`). For a server run without Docker,
use the [native variants](#without-docker).

## What to back up

| Item | Where | Why |
| --- | --- | --- |
| The `acis` database | `db` service (`mariadb_data` volume) | All account, character, item, clan, castle, Olympiad and world state. The login and game servers share this database, as in the reference setup. |
| `config/` | `ACIS_CONFIG_DIR` | Your edited `*.properties` and `hexid.txt`. `hexid.txt` must match the `gameservers` row in the same backup. |
| `data/crests/` | `ACIS_DATAPACK_DIR/data/crests` | Clan and alliance crest images. The database stores only the crest ids. |
| `data/xml/announcements.xml` | `ACIS_DATAPACK_DIR/data/xml` | The game server rewrites this file when a GM edits announcements in game. |

The rest of the datapack (`xml/`, `html/`, `geodata/`) is never written at run time. Record which
`aCis_datapack` commit you run, and you can fetch it again. Logs are optional.

## Consistency

All 65 aCis tables are InnoDB, so `mariadb-dump --single-transaction` takes a consistent snapshot
without locking the running servers. The game server keeps live state in memory and writes it on
its own schedule: at logout, on periodic saves, and in full on a graceful stop. That gives two kinds
of backup:

- **Hot backup** (servers running): a consistent database that is missing whatever the game server
  had not written yet. Restoring it is like recovering from a crash at the moment of the dump. Use
  it for routine scheduled backups.
- **Cold backup** (game server stopped first): includes everything, because the graceful stop saves
  all players and world state before the process exits. Take one before upgrades, migrations or
  any restore you plan ahead.

## Taking a backup

Save this as `backup.sh` in the checkout root and make it executable (`chmod +x backup.sh`). Run it
as `./backup.sh` for a hot backup, or `COLD=1 ./backup.sh` for a cold one. It is a script, not a
block to paste, because `set -euo pipefail` would close an interactive shell on the first error.

```bash
#!/usr/bin/env bash
# Run from the checkout root with .env loaded. COLD=1 stops the game server around the dump.
set -euo pipefail

dump()     { docker compose exec -T db sh -c 'exec mariadb-dump -uroot -p"$MARIADB_ROOT_PASSWORD" --single-transaction --routines --triggers --events acis'; }
stop_gs()  { docker compose --profile servers stop gameserver; }
start_gs() { docker compose --profile servers start gameserver; }

stamp=$(date +%Y%m%d-%H%M%S)
mkdir -p backups
# Absolute paths, so tar's -C options do not stack. Defaults as in docker-compose.yml.
cfg=$(cd "${ACIS_CONFIG_DIR:-./config}" && pwd)
datapack=$(cd "${ACIS_DATAPACK_DIR:-../aCis_datapack}" && pwd)
db=backups/acis-db-$stamp.sql.gz
files=backups/acis-files-$stamp.tar.gz

# Output keeps a .tmp suffix until it passes the checks; an early exit removes it and restarts a
# stopped game server.
stopped=0
cleanup() {
  status=$?
  rm -f "$db.tmp" "$files.tmp"
  if [ "$stopped" = 1 ]; then start_gs; fi
  if [ "$status" != 0 ]; then echo "backup FAILED (exit $status)" >&2; fi
}
trap cleanup EXIT

if [ "${COLD:-0}" = 1 ]; then stopped=1; stop_gs; fi   # cold: the final save is in the dump

dump | gzip > "$db.tmp"    # pipefail: a failed mariadb-dump fails here, not just gzip
tar czf "$files.tmp" -C "$(dirname "$cfg")" "$(basename "$cfg")" \
  -C "$datapack" data/crests data/xml/announcements.xml

if [ "$stopped" = 1 ]; then start_gs; stopped=0; fi

gunzip -c "$db.tmp" | tail -n 1 | grep -q '^-- Dump completed' \
  || { echo "$db.tmp: dump is incomplete" >&2; exit 1; }
tar tzf "$files.tmp" "$(basename "$cfg")/hexid.txt" "$(basename "$cfg")/server.properties" \
  data/crests data/xml/announcements.xml >/dev/null

mv "$db.tmp" "$db"
mv "$files.tmp" "$files"
echo "backup ok: $db $files"
```

The password is read from inside the `db` container, so it never appears on the host command
line. `backups/` is ignored by git. Copy it off the host: a backup kept on the same disk does not
survive losing that disk.

What each check proves:

- `pipefail` makes the dump line fail when `mariadb-dump` fails (a wrong password, a stopped `db`
  container), even though `gzip` still writes a valid, nearly empty file.
- `-- Dump completed` is the last line `mariadb-dump` writes, only after every table. Finding it
  proves the dump ran to the end; a dump cut short by a full disk or a killed process lacks it.
  Reading the file through `gunzip` also proves the gzip container is intact.
- `tar tzf` proves the archive holds `hexid.txt`, `server.properties`, the crests and the
  announcements. A wrong `ACIS_CONFIG_DIR` or `ACIS_DATAPACK_DIR` shows up as `Not found in
  archive`. The archive stores the config directory under its own name (`config` by default) and
  the datapack files under `data/`.

None of them loads the SQL. Only [a scratch load](#checking-a-backup-without-touching-the-live-database)
proves that the dump restores; do that before every restore and from time to time on a copy taken
off the host.

`backup ok` is printed, and the files lose their `.tmp` suffix, only when every step and check
passed. Anything else prints `backup FAILED` and leaves no new file behind.

For a nightly hot backup, call the script from cron and prune old files only after it succeeded,
so nights with a failing dump never delete the last good backups:

```bash
15 4 * * * cd /srv/acis_golang && set -a && . ./.env && set +a && ./backup.sh && find backups -name 'acis-*' -mtime +14 -delete
```

## Checking a backup without touching the live database

Save this as `check-dump.sh` next to `backup.sh` and run it as `./check-dump.sh
backups/acis-db-<stamp>.sql.gz`. It loads the dump into a scratch database next to `acis`, prints a
few row counts, then drops it:

```bash
#!/usr/bin/env bash
# Run from the checkout root with .env loaded.
set -euo pipefail

sql() { docker compose exec -T db sh -c 'exec mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" "$@"' sh "$@"; }

f=$1
gunzip -c "$f" | tail -n 1 | grep -q '^-- Dump completed' \
  || { echo "$f: dump is incomplete" >&2; exit 1; }
sql -e 'DROP DATABASE IF EXISTS acis_restore_check; CREATE DATABASE acis_restore_check'
gunzip -c "$f" | sql acis_restore_check
sql -t acis_restore_check -e 'SELECT (SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()) AS tables, (SELECT COUNT(*) FROM accounts) AS accounts, (SELECT COUNT(*) FROM characters) AS characters, (SELECT COUNT(*) FROM items) AS items, (SELECT COUNT(*) FROM gameservers) AS gameservers'
sql -e 'DROP DATABASE acis_restore_check'
echo "dump ok: $f"
```

`dump ok` proves that the dump is complete, that MariaDB accepts every statement in it, and that
the `accounts`, `characters`, `items` and `gameservers` tables exist after loading it. Compare the
counts with what you expect; `tables` is 65 for the current schema. If the script stops early, the
scratch database stays until the next run drops it.

## Restoring

Stop **both** servers before the database is replaced. A running game server would write its
in-memory state over the restored rows, and the login server reads the registered game servers
only at startup.

The block runs in a subshell with `set -euo pipefail`, so the first failing step stops it without
closing your shell. Nothing touches the live database until the dump has passed `check-dump.sh`,
the archive has been read, and the current state has been saved with a cold backup, so a restore
can itself be undone. If the live database is already lost (a new `mariadb_data` volume, for
example), drop the `COLD=1 ./backup.sh` line.

```bash
db=backups/acis-db-<stamp>.sql.gz
files=backups/acis-files-<stamp>.tar.gz
(
  set -euo pipefail
  sql()       { docker compose exec -T db sh -c 'exec mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" "$@"' sh "$@"; }
  stop_all()  { docker compose --profile servers stop gameserver loginserver; }
  start_all() { docker compose --profile servers start loginserver gameserver; }

  mkdir -p "${ACIS_CONFIG_DIR:-./config}"    # a fresh host or checkout has no config dir yet
  cfg=$(cd "${ACIS_CONFIG_DIR:-./config}" && pwd)
  datapack=$(cd "${ACIS_DATAPACK_DIR:-../aCis_datapack}" && pwd)

  # Verify first: the live database is untouched until here.
  ./check-dump.sh "$db"
  tar tzf "$files" "$(basename "$cfg")/hexid.txt" data/crests >/dev/null
  COLD=1 ./backup.sh                          # the current state, to undo this restore

  stop_all
  sql -e 'DROP DATABASE IF EXISTS acis; CREATE DATABASE acis'
  gunzip -c "$db" | sql acis
  tar xzf "$files" -C "$(dirname "$cfg")" "$(basename "$cfg")"
  tar xzf "$files" -C "$datapack" data
  start_all
  echo restore ok
)
```

After `restore ok`, follow `docker compose logs -f gameserver` until it prints `linked to
loginserver`. Run the block as it is: a `&&` or `||` after its closing parenthesis makes bash
ignore `set -e` inside it, and a failed check would no longer stop the restore.

If a step after `stop_all` fails, the servers stay stopped and no `restore ok` is printed. Run the
block again, or restore the backup the block took just before, whose name it printed after
`backup ok`.

For a restore drill, point `check-dump.sh` at a copy of the newest backup taken off the host. It
proves the backup restores without stopping anything or touching `acis`.

The dump holds the full schema (`DROP TABLE IF EXISTS` and `CREATE TABLE` for every table), so it
also restores into a brand-new `mariadb_data` volume.

## Without Docker

For servers run as plain processes against a host MariaDB (see [run-servers.md](../run-servers.md),
or a reference-style `gameserver/` directory as in [migrating-from-java.md](migrating-from-java.md)),
use the same scripts and restore block with the `docker compose` helpers replaced:

```bash
dump()      { mariadb-dump -u root -p --single-transaction --routines --triggers --events acis; }
sql()       { mariadb -u root -p "$@"; }
stop_gs()   { systemctl stop acis-gameserver; }
start_gs()  { systemctl start acis-gameserver; }
stop_all()  { systemctl stop acis-gameserver acis-loginserver; }
start_all() { systemctl start acis-loginserver && systemctl start acis-gameserver; }
```

Run them from the game server's working directory, the one holding `config/` and `data/`, with
`ACIS_CONFIG_DIR=config ACIS_DATAPACK_DIR=.`. `-p` asks for the password at every call; for cron,
put it in the `[client]` section of the user's `~/.my.cnf` (mode `600`) and drop `-p`. Put the
login server's `config/` in the same archive if it lives in a separate directory.

## Failure modes

| Symptom | Cause and fix |
| --- | --- |
| The most recent progress is missing after restoring a hot backup. | Expected: the game server had not written it yet. Take a cold backup when you need everything. |
| Restored items or characters change back, or the restore seems to have no effect. | A server was still running during the restore and wrote over it. Stop both, then restore again. |
| The game server logs `wrong hexid` after a restore. | `config/hexid.txt` comes from a different backup than the database. Restore both from the same `<stamp>`, or register again ([docker.md](docker.md#first-start)) and restart both services. |
| Clans have no crest after a restore. | `data/crests/` was not restored. Extract it from the files archive. |
| `gunzip: unexpected end of file`, or the dump ends without the `-- Dump completed` line. | The dump was cut short, for example because the disk filled up or the `db` container stopped. Treat that file as bad and take a new backup. `backup.sh` and `check-dump.sh` both reject it, because the `-- Dump completed` line is missing; `backup.sh` then prints `backup FAILED` and keeps no file. |
| `mariadb-dump: Got error: 1045: Access denied`, followed by `backup FAILED`. | `ACIS_DB_ROOT_PASSWORD` was changed after the volume was created. MariaDB keeps the root password it was created with. The variable only changes what these commands, and the servers' `Password` key, send. Set the variable back, or change the password in MariaDB with `ALTER USER` and update both `.properties` files to match. |
