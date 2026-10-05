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

```bash
stamp=$(date +%Y%m%d-%H%M%S)
mkdir -p backups
# Absolute paths, so tar's -C options do not stack. Defaults as in docker-compose.yml.
cfg=$(cd "${ACIS_CONFIG_DIR:-./config}" && pwd)
datapack=$(cd "${ACIS_DATAPACK_DIR:-../aCis_datapack}" && pwd)

# Cold backup only: stop the game server first, so its final save is included.
docker compose --profile servers stop gameserver

docker compose exec -T db sh -c 'exec mariadb-dump -uroot -p"$MARIADB_ROOT_PASSWORD" --single-transaction --routines --triggers --events acis' \
  | gzip > "backups/acis-db-$stamp.sql.gz"
tar czf "backups/acis-files-$stamp.tar.gz" -C "$(dirname "$cfg")" "$(basename "$cfg")" \
  -C "$datapack" data/crests data/xml/announcements.xml

# Cold backup only: start it again.
docker compose --profile servers start gameserver

gzip -t "backups/acis-db-$stamp.sql.gz" \
  && tar tzf "backups/acis-files-$stamp.tar.gz" "$(basename "$cfg")/hexid.txt" "$(basename "$cfg")/server.properties" \
       data/crests data/xml/announcements.xml >/dev/null \
  && echo backup ok
```

The password is read from inside the `db` container, so it never appears on the host command
line. `backups/` is ignored by git. Copy it off the host: a backup kept on the same disk does not
survive losing that disk.

The archive stores the config directory under its own name (`config` by default) and the datapack
files under `data/`. The last command prints `backup ok` only when the dump is intact and the
archive holds `hexid.txt`, `server.properties`, the crests and the announcements, so a wrong
`ACIS_CONFIG_DIR` or `ACIS_DATAPACK_DIR` shows up as `Not found in archive` instead.

For a nightly hot backup, put this block without its two cold-backup lines in a script and call it
from cron.
For example, `15 4 * * * cd /srv/acis_golang && set -a && . ./.env && set +a && ./backup.sh`.
Prune old files with `find backups -name 'acis-*' -mtime +14 -delete`.

## Checking a backup without touching the live database

Load the dump into a scratch database next to `acis`, look at a few row counts, then drop it:

```bash
f=backups/acis-db-<stamp>.sql.gz
docker compose exec -T db sh -c 'exec mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -e "CREATE DATABASE acis_restore_check"'
gunzip -c "$f" | docker compose exec -T db sh -c 'exec mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" acis_restore_check'
docker compose exec -T db sh -c 'exec mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -t -e "SELECT (SELECT COUNT(*) FROM acis_restore_check.accounts) AS accounts, (SELECT COUNT(*) FROM acis_restore_check.characters) AS characters, (SELECT COUNT(*) FROM acis_restore_check.items) AS items, (SELECT COUNT(*) FROM acis_restore_check.gameservers) AS gameservers"'
docker compose exec -T db sh -c 'exec mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -e "DROP DATABASE acis_restore_check"'
```

## Restoring

Stop **both** servers first. A running game server would write its in-memory state over the
restored rows, and the login server reads the registered game servers only at startup.

```bash
db=backups/acis-db-<stamp>.sql.gz
files=backups/acis-files-<stamp>.tar.gz
mkdir -p "${ACIS_CONFIG_DIR:-./config}"    # a fresh host or checkout has no config dir yet
cfg=$(cd "${ACIS_CONFIG_DIR:-./config}" && pwd)
datapack=$(cd "${ACIS_DATAPACK_DIR:-../aCis_datapack}" && pwd)

docker compose --profile servers stop gameserver loginserver

docker compose exec -T db sh -c 'exec mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -e "DROP DATABASE acis; CREATE DATABASE acis"'
gunzip -c "$db" | docker compose exec -T db sh -c 'exec mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" acis'
tar xzf "$files" -C "$(dirname "$cfg")" "$(basename "$cfg")"
tar xzf "$files" -C "$datapack" data

docker compose --profile servers start loginserver gameserver
docker compose logs -f gameserver    # wait for "linked to loginserver"
```

The dump holds the full schema (`DROP TABLE IF EXISTS` and `CREATE TABLE` for every table), so it
also restores into a brand-new `mariadb_data` volume.

## Without Docker

For servers run as plain processes against a host MariaDB (see [run-servers.md](../run-servers.md),
or a reference-style `gameserver/` directory as in [migrating-from-java.md](migrating-from-java.md)),
use the same commands without the `docker compose exec` wrapper. Run them from the game server's
working directory, the one holding `config/` and `data/`:

```bash
systemctl stop acis-gameserver                       # cold backup only
mariadb-dump -u root -p --single-transaction --routines --triggers --events acis | gzip > "backups/acis-db-$stamp.sql.gz"
tar czf "backups/acis-files-$stamp.tar.gz" config data/crests data/xml/announcements.xml
systemctl start acis-gameserver                      # cold backup only
```

To restore, stop both services, run `mariadb -u root -p -e 'DROP DATABASE acis; CREATE DATABASE acis'`,
then `gunzip -c <dump> | mariadb -u root -p acis`, then `tar xzf <files>`, then start the login
server and then the game server. Put the login server's `config/` in the same archive if it lives in
a separate directory.

## Failure modes

| Symptom | Cause and fix |
| --- | --- |
| The most recent progress is missing after restoring a hot backup. | Expected: the game server had not written it yet. Take a cold backup when you need everything. |
| Restored items or characters change back, or the restore seems to have no effect. | A server was still running during the restore and wrote over it. Stop both, then restore again. |
| The game server logs `wrong hexid` after a restore. | `config/hexid.txt` comes from a different backup than the database. Restore both from the same `<stamp>`, or register again ([docker.md](docker.md#first-start)) and restart both services. |
| Clans have no crest after a restore. | `data/crests/` was not restored. Extract it from the files archive. |
| `gunzip: unexpected end of file`, or the dump ends without the `-- Dump completed` line. | The dump was cut short, for example because the disk filled up or the `db` container stopped. Treat that file as bad and take a new backup. `gzip -t` catches it right after the dump. |
| `mariadb-dump: Got error: 1045: Access denied`. | `ACIS_DB_ROOT_PASSWORD` was changed after the volume was created. MariaDB keeps the root password it was created with. The variable only changes what these commands, and the servers' `Password` key, send. Set the variable back, or change the password in MariaDB with `ALTER USER` and update both `.properties` files to match. |
