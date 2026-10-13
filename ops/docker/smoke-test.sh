#!/bin/sh
# Smoke test for the Docker setup; CI runs it on every change (the docker job
# in .github/workflows/go.yml). It checks that:
#   1. docker-compose.yml parses, with and without the servers profile;
#   2. the Dockerfile builds, and every binary in the image starts as the
#      image's runtime user;
#   3. the db service's first start loads the schema through
#      initdb/10-acis-schema.sh: every table file in a stub datapack is
#      loaded, and a datapack without tools/full_install.sql makes that
#      container start exit non-zero;
#   4. after such a failed first start under compose up, the restart policy
#      brings db back serving an empty acis database, and it never reports
#      healthy: the init script's marker fails the healthcheck.
#
# The stub datapack keeps this independent of an aCis_datapack checkout, so
# it checks the compose mounts and the init script, not the real 65-table
# schema.
#
# Run from anywhere: ops/docker/smoke-test.sh. It tags the image
# acis-golang:smoke, uses the compose project acis-smoke (its own volume,
# removed on exit) and publishes the database on ACIS_DB_PORT (default
# 3306). The db service has a fixed container name, so a running
# docker-compose.yml database on the same host makes this fail.
set -eu

root=$(cd "$(dirname "$0")/../.." && pwd)
image=acis-golang:smoke
project=acis-smoke
work=$(mktemp -d)

export ACIS_DB_ROOT_PASSWORD=smoke
export ACIS_DB_PORT="${ACIS_DB_PORT:-3306}"

compose() {
	docker compose --project-directory "$root" -f "$root/docker-compose.yml" -p "$project" "$@"
}

# timeout runs a program, not a shell function.
compose_timeout() {
	secs=$1
	shift
	timeout "$secs" docker compose --project-directory "$root" -f "$root/docker-compose.yml" -p "$project" "$@"
}

# 3b's one-off container, named so cleanup can stop it even when the run
# outlives its timeout (timeout stops the compose client, not the container).
broken=acis-smoke-broken-db

clean_project() {
	docker rm -f "$broken" >/dev/null 2>&1 || true
	compose down -v --remove-orphans >/dev/null 2>&1 || true
}

cleanup() {
	clean_project
	rm -rf "$work"
}
trap cleanup EXIT
# A volume left by an interrupted run would skip the first-start load.
clean_project

fail() {
	echo "smoke: FAIL: $*" >&2
	exit 1
}

# 1. Compose file.
ACIS_DATAPACK_DIR="$work" compose config -q
ACIS_DATAPACK_DIR="$work" compose --profile servers config -q
echo "smoke: docker-compose.yml parses"

# 2. Image. flag's -h prints usage and exits 0, so each binary runs without
# config, database or datapack.
docker build -t "$image" "$root"
for bin in gameserver loginserver gsregister accountmgr; do
	docker run --rm "$image" "$bin" -h >/dev/null 2>&1 || fail "$bin -h did not exit 0 in $image"
done
echo "smoke: image builds and its binaries run"

# 3a. Schema seed from a stub datapack: full_install.sql drops the tables,
# then each sql/*.sql creates one, as in the real datapack.
mkdir -p "$work/ok/tools" "$work/ok/sql" "$work/broken/sql"
tables="smoke_accounts smoke_characters smoke_items"
for t in $tables; do
	echo "DROP TABLE IF EXISTS \`$t\`;" >>"$work/ok/tools/full_install.sql"
	printf 'CREATE TABLE `%s` (`id` INT PRIMARY KEY);\nINSERT INTO `%s` VALUES (1);\n' "$t" "$t" >"$work/ok/sql/$t.sql"
done
cp "$work/ok/sql/"*.sql "$work/broken/sql/"
want=$(echo "$tables" | wc -w)

ACIS_DATAPACK_DIR="$work/ok" compose up -d --wait --wait-timeout 180 db ||
	{ compose logs db >&2; fail "db did not become healthy with a complete datapack"; }
got=$(compose exec -T db mariadb -uroot -p"$ACIS_DB_ROOT_PASSWORD" -N -B -e \
	"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'acis'")
[ "$got" = "$want" ] || { compose logs db >&2; fail "acis has $got tables, want $want"; }
rows=$(compose exec -T db mariadb -uroot -p"$ACIS_DB_ROOT_PASSWORD" -N -B acis -e \
	"SELECT COUNT(*) FROM smoke_items")
[ "$rows" = 1 ] || fail "smoke_items has $rows rows, want 1"
echo "smoke: first start loaded $got tables"
clean_project

# 3b. A datapack without tools/full_install.sql must stop the first start.
# compose run applies no restart policy, so the entrypoint's exit status is
# the container's.
rc=0
out=$(ACIS_DATAPACK_DIR="$work/broken" compose_timeout 120 run --rm -T --name "$broken" db 2>&1) || rc=$?
case $rc in
0 | 124) echo "$out" >&2 && fail "db first start did not stop without tools/full_install.sql (status $rc)" ;;
esac
echo "$out" | grep -q 'acis schema: .*full_install.sql not found' ||
	{ echo "$out" >&2; fail "db first start did not name the missing full_install.sql"; }
echo "smoke: first start without full_install.sql stops the db"
clean_project

# 4. The same failed first start through compose up, with the real restart
# policy. restart: always starts db again, the entrypoint skips the schema
# load over the existing data directory, and the server answers on an empty
# acis database. It must not report healthy, so the servers that wait for a
# healthy db never start. Wait until that restarted server answers and its
# own healthcheck has rejected it on the marker (Docker clears the health
# log on restart).
rc=0
ACIS_DATAPACK_DIR="$work/broken" compose up -d --wait --wait-timeout 120 db >/dev/null 2>&1 || rc=$?
[ "$rc" -ne 0 ] || fail "compose up --wait reported db healthy after a failed first-start schema load"
id=$(compose ps -a -q db)
[ -n "$id" ] || fail "compose up left no db container"
tries=0
while :; do
	state=$(docker inspect -f '{{.RestartCount}} {{.State.Status}} {{.State.Health.Status}}' "$id") ||
		fail "db container $id is gone"
	set -- $state
	[ "$3" != healthy ] || { compose logs db >&2; fail "db reported healthy after a failed first-start schema load"; }
	if [ "$1" -ge 1 ] && [ "$2" = running ] &&
		compose exec -T db mariadb -uroot -p"$ACIS_DB_ROOT_PASSWORD" -N -B -e 'SELECT 1' >/dev/null 2>&1 &&
		docker inspect -f '{{range .State.Health.Log}}{{.Output}}{{end}}' "$id" | grep -q 'acis schema load did not finish'; then
		break
	fi
	tries=$((tries + 1))
	[ "$tries" -lt 90 ] || { compose logs db >&2; fail "restarted db did not answer and fail its healthcheck on the schema marker (restarts=$1 state=$2 health=$3)"; }
	sleep 2
done
echo "smoke: restarted db over a failed first start stays unhealthy"

echo "smoke: ok"
