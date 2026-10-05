#!/bin/sh
# Prepares the config directory docker-compose.yml mounts into the
# loginserver and gameserver containers (see docs/ops/docker.md).
#
# Usage, from anywhere in the checkout:
#   ops/docker/init-config.sh [reference-config-dir]
#
# Copies the reference server's config/ (default ../aCis_gameserver/config)
# to ACIS_CONFIG_DIR (default ./config), then rewrites only the keys that
# differ inside the compose network:
#   server.properties, loginserver.properties:
#     URL       -> jdbc:mariadb://db/acis      (the db service)
#     Login     -> root                         (the only user compose creates)
#     Password  -> $ACIS_DB_ROOT_PASSWORD       (default 123321, as compose)
#   server.properties:
#     LoginHost -> loginserver                  (the loginserver service)
#     Hostname  -> $ACIS_PUBLIC_HOST            (default 127.0.0.1)
# Every other key keeps the reference value. It also creates the log
# directories (under ACIS_LOG_DIR, default ./log) and the datapack's
# data/crests directory, which the servers write to: a directory Docker
# creates for a missing bind mount belongs to root, not to ACIS_UID.
#
# It refuses to overwrite an existing server.properties, so it never undoes
# an operator's edits. Values are written verbatim: keep backslashes out of
# them.
set -eu

cd "$(dirname "$0")/../.."

src=${1:-../aCis_gameserver/config}
dst=${ACIS_CONFIG_DIR:-./config}
datapack=${ACIS_DATAPACK_DIR:-../aCis_datapack}
logdir=${ACIS_LOG_DIR:-./log}
password=${ACIS_DB_ROOT_PASSWORD:-123321}
public_host=${ACIS_PUBLIC_HOST:-127.0.0.1}

die() {
	echo "init-config: $*" >&2
	exit 1
}

[ -f "$src/server.properties" ] || die "$src/server.properties not found; pass the reference config directory as the first argument"
[ -f "$src/loginserver.properties" ] || die "$src/loginserver.properties not found"
[ ! -e "$dst/server.properties" ] || die "$dst/server.properties already exists; edit it in place or remove $dst first"
[ -d "$datapack/data/xml" ] || die "$datapack/data/xml not found; set ACIS_DATAPACK_DIR to the aCis_datapack checkout"

# set_key FILE KEY VALUE replaces the first uncommented KEY line of FILE and
# fails when FILE has none, so a renamed key is reported, not skipped.
set_key() {
	awk -v k="$2" -v v="$3" '
		!done && $0 ~ "^" k "[ \t]*[=:]" { print k " = " v; done = 1; next }
		{ print }
		END { if (!done) exit 3 }
	' "$1" >"$1.tmp" || {
		rm -f "$1.tmp"
		die "$1 has no $2 key"
	}
	mv "$1.tmp" "$1"
}

mkdir -p "$dst"
cp -R "$src/." "$dst/"

for f in "$dst/server.properties" "$dst/loginserver.properties"; do
	set_key "$f" URL "jdbc:mariadb://db/acis"
	set_key "$f" Login root
	set_key "$f" Password "$password"
done
set_key "$dst/server.properties" LoginHost loginserver
set_key "$dst/server.properties" Hostname "$public_host"

mkdir -p "$logdir/loginserver" "$logdir/gameserver" "$datapack/data/crests"

echo "init-config: wrote $dst (database db, login link loginserver, advertised host $public_host)"
if [ ! -f "$dst/hexid.txt" ]; then
	echo "init-config: next, register the game server to create $dst/hexid.txt (docs/ops/docker.md)"
fi
