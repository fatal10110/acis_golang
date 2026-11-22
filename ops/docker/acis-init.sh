#!/bin/sh
# Prepares a compose stack before its servers start. docker-compose.yml runs
# it as the one-shot init service, on every `up`, from the server image
# (installed as /usr/local/bin/acis-init); see docs/ops/docker.md.
#
# It works in ACIS_HOME (default /opt/acis), with config/, data/, log/ and
# cache/ mounted there:
#   1. An empty config/ (no server.properties) gets the config bundle the
#      image carries (ACIS_CONFIG_BUNDLE, default /usr/local/share/acis/config).
#      An existing config, such as one init-config.sh wrote from a reference
#      tree, is kept.
#   2. The keys the compose network fixes are written on every run:
#        server.properties, loginserver.properties:
#          URL = jdbc:mariadb://db/acis, Login = root,
#          Password = $ACIS_DB_ROOT_PASSWORD (required)
#        server.properties:
#          LoginHost = loginserver, GameserverPort = $ACIS_GAME_PORT
#          (default 7777, the port compose publishes)
#   3. Per-stack keys are written only when their variable is set and not
#      empty; an unset one leaves the file's value:
#        ACIS_PUBLIC_HOST              server Hostname (advertised to clients)
#        ACIS_GAMESERVER_HOSTNAME      server GameserverHostname (bind address)
#        ACIS_MAX_ONLINE_USERS         server MaximumOnlineUsers
#        ACIS_AUTO_CREATE_ACCOUNTS     login AutoCreateAccounts
#        ACIS_LOGIN_TRY_BEFORE_BAN     login LoginTryBeforeBan
#        ACIS_ACCEPT_FILTER            EnableAcceptFilter, both servers
#        ACIS_ACCEPT_FILTER_ALLOWLIST  AcceptFilterAllowlist, both servers
#        ACIS_GEO_REGIONS              geoengine.properties region list:
#                                      regions such as "16_10 17_11"
#                                      (spaces or commas), or "none"
#      A key the file lacks is appended.
#   4. Game server id 1 is registered and config/hexid.txt written
#      (gsregister -register 1); a rerun over a matching registration
#      changes nothing.
#   5. log/loginserver, log/gameserver, cache/ and data/crests are created,
#      and, when it runs as root, they, the log root and config/ are given
#      to ACIS_UID:ACIS_GID (default 1000:1000), the user the servers run
#      as. Docker creates a missing bind-mount directory as root.
#
# Values are written verbatim, so they may not hold a newline or a
# backslash.
set -eu

home=${ACIS_HOME:-/opt/acis}
bundle=${ACIS_CONFIG_BUNDLE:-/usr/local/share/acis/config}
uid=${ACIS_UID:-1000}
gid=${ACIS_GID:-1000}
game_port=${ACIS_GAME_PORT:-7777}
cfg=$home/config

die() {
	echo "acis-init: $*" >&2
	exit 1
}

[ -n "${ACIS_DB_ROOT_PASSWORD:-}" ] || die "ACIS_DB_ROOT_PASSWORD is not set"
[ -d "$home/data/xml" ] || die "$home/data/xml not found; set ACIS_DATAPACK_DIR to the aCis_datapack checkout"

# rewrite FILE AWK-PROGRAM [awk -v assignments...] replaces FILE with the
# program's output, writing into the existing file so its owner and mode
# stay.
rewrite() {
	file=$1
	prog=$2
	shift 2
	awk "$@" "$prog" "$file" >"$file.tmp" || {
		rm -f "$file.tmp"
		die "cannot rewrite $file"
	}
	cat "$file.tmp" >"$file"
	rm -f "$file.tmp"
}

# set_key FILE KEY VALUE sets every uncommented KEY line of FILE to VALUE,
# and appends one when FILE has none.
set_key() {
	case $3 in
	*"
"* | *\\*) die "value for $2 holds a newline or a backslash" ;;
	esac
	rewrite "$1" '
		$0 ~ "^" k "([ \t=:]|$)" { print k " = " v; done = 1; next }
		{ print }
		END { if (!done) print k " = " v }
	' -v k="$2" -v v="$3"
}

# set_if VAR KEY FILE... sets KEY in each config FILE when VAR is not empty.
set_if() {
	eval "value=\${$1:-}"
	[ -n "$value" ] || return 0
	key=$2
	shift 2
	for f in "$@"; do
		set_key "$cfg/$f" "$key" "$value"
	done
}

# set_regions LIST replaces the geoengine.properties region lines with LIST.
set_regions() {
	list=$(echo "$1" | tr ',' ' ')
	[ "$list" = none ] && list=
	for r in $list; do
		echo "$r" | grep -Eqx '[0-9]+_[0-9]+' || die "ACIS_GEO_REGIONS: $r is not a region (X_Y)"
	done
	[ -f "$cfg/geoengine.properties" ] || die "$cfg/geoengine.properties not found"
	rewrite "$cfg/geoengine.properties" '
		$0 ~ /^[0-9]+_[0-9]+([ \t\r=:].*)?$/ { next }
		{ print }
		END { n = split(regions, r, " "); for (i = 1; i <= n; i++) print r[i] }
	' -v regions="$list"
}

mkdir -p "$cfg"
if [ ! -e "$cfg/server.properties" ]; then
	[ -f "$bundle/server.properties" ] || die "$bundle/server.properties not found"
	cp -R "$bundle/." "$cfg/"
	echo "acis-init: wrote the config bundle to $cfg"
fi
for f in server.properties loginserver.properties; do
	[ -f "$cfg/$f" ] || die "$cfg/$f not found"
done

for f in server.properties loginserver.properties; do
	set_key "$cfg/$f" URL "jdbc:mariadb://db/acis"
	set_key "$cfg/$f" Login root
	set_key "$cfg/$f" Password "$ACIS_DB_ROOT_PASSWORD"
done
set_key "$cfg/server.properties" LoginHost loginserver
set_key "$cfg/server.properties" GameserverPort "$game_port"

set_if ACIS_PUBLIC_HOST Hostname server.properties
set_if ACIS_GAMESERVER_HOSTNAME GameserverHostname server.properties
set_if ACIS_MAX_ONLINE_USERS MaximumOnlineUsers server.properties
set_if ACIS_AUTO_CREATE_ACCOUNTS AutoCreateAccounts loginserver.properties
set_if ACIS_LOGIN_TRY_BEFORE_BAN LoginTryBeforeBan loginserver.properties
set_if ACIS_ACCEPT_FILTER EnableAcceptFilter server.properties loginserver.properties
set_if ACIS_ACCEPT_FILTER_ALLOWLIST AcceptFilterAllowlist server.properties loginserver.properties
if [ -n "${ACIS_GEO_REGIONS:-}" ]; then
	set_regions "$ACIS_GEO_REGIONS"
fi

mkdir -p "$home/log/loginserver" "$home/log/gameserver" "$home/cache" "$home/data/crests"

cd "$home"
gsregister -config config/loginserver.properties -names data/serverNames.xml \
	-register 1 -out config/hexid.txt

if [ "$(id -u)" = 0 ]; then
	chown -R "$uid:$gid" "$cfg" "$home/log" "$home/cache" "$home/data/crests"
fi
echo "acis-init: config ready (game port $game_port)"
