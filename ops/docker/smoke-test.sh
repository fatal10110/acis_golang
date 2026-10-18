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
#      healthy: the init script's marker fails the healthcheck;
#   5. a data directory left by a first start killed before the init script
#      ran (no marker, no tables) never reports healthy either;
#   6. with ACIS_SMOKE_DATAPACK set to an aCis_datapack checkout, the first
#      start loads its real schema: acis then holds one table per sql/*.sql
#      file.
#
# Steps 1-5 use a generated stub datapack, so they check the compose mounts,
# the init script and the healthcheck without an aCis_datapack checkout.
# Step 6 is skipped when ACIS_SMOKE_DATAPACK is unset.
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

if [ -n "${ACIS_SMOKE_DATAPACK:-}" ]; then
	[ -f "$ACIS_SMOKE_DATAPACK/tools/full_install.sql" ] ||
		fail "ACIS_SMOKE_DATAPACK=$ACIS_SMOKE_DATAPACK has no tools/full_install.sql"
	ACIS_SMOKE_DATAPACK=$(cd "$ACIS_SMOKE_DATAPACK" && pwd)
fi

# acis_tables prints the number of tables in acis.
acis_tables() {
	compose exec -T db mariadb -uroot -p"$ACIS_DB_ROOT_PASSWORD" -N -B -e \
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'acis'"
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
got=$(acis_tables)
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

# wait_rejected waits until db, started by compose up, is running after at
# least $1 restarts, one of its healthchecks that began during the current
# start has rejected it with a message containing $2, and the command $3
# succeeds. It fails as soon as the container reports healthy.
#
# This polls rather than trusting compose up --wait: some compose releases
# (2.38 among them) exit 0 when --wait-timeout expires while the service is
# still starting, so its status cannot tell a timeout from a healthy db.
#
# Docker keeps health log entries across a restart, and an earlier start's
# entries can carry the same message, so only entries that began after the
# current start count.
db_state() {
	compose logs db >&2 || true
	[ -z "${id:-}" ] || docker inspect -f '{{json .State}}' "$id" >&2 || true
}
# One inspect call, so the start time and the health log come from the same
# container start: a first line with the restart count, status, health and
# start time, then one line per health log entry with its start in Unix
# nanoseconds and its JSON-quoted output.
inspect_fmt='{{.RestartCount}} {{.State.Status}} {{.State.Health.Status}} {{.State.StartedAt}}{{"\n"}}{{range .State.Health.Log}}{{.Start.UnixNano}} {{json .Output}}{{"\n"}}{{end}}'
wait_rejected() {
	min_restarts=$1
	want_msg=$2
	ready_check=$3
	id=$(compose ps -a -q db)
	[ -n "$id" ] || fail "compose up left no db container"
	tries=0
	while :; do
		snap=$(docker inspect -f "$inspect_fmt" "$id") || fail "db container $id is gone"
		set -- $(echo "$snap" | head -n 1)
		[ "$3" != healthy ] || { db_state; fail "db reported healthy ($want_msg expected)"; }
		marked=no
		if [ "$1" -ge "$min_restarts" ] && [ "$2" = running ]; then
			since=$(date -u -d "$4" +%s%N) || fail "cannot parse db start time $4"
			echo "$snap" | tail -n +2 | {
				while read -r ns out; do
					if [ "$ns" -gt "$since" ]; then
						case $out in *"$want_msg"*) exit 0 ;; esac
					fi
				done
				exit 1
			} && marked=yes
		fi
		if [ "$marked" = yes ] && $ready_check; then
			break
		fi
		tries=$((tries + 1))
		[ "$tries" -lt 90 ] || { db_state; fail "db did not answer and fail its healthcheck with \"$want_msg\" (restarts=$1 state=$2 health=$3)"; }
		sleep 2
	done
}

# 4. The same failed first start through compose up, with the real restart
# policy. restart: always starts db again, the entrypoint skips the schema
# load over the existing data directory, and the server answers on an empty
# acis database. It must not report healthy, so the servers that wait for a
# healthy db never start. Wait until that restarted server answers with no
# acis tables and one of its own healthchecks has rejected it on the marker.
restarted_empty() {
	got=$(acis_tables 2>/dev/null) || return 1
	[ "$got" = 0 ] || { db_state; fail "restarted db has $got acis tables, want 0"; }
}
ACIS_DATAPACK_DIR="$work/broken" compose up -d db >&2 ||
	{ db_state; fail "compose up did not start db over the broken datapack"; }
wait_rejected 1 'acis schema load did not finish' restarted_empty
echo "smoke: restarted db over a failed first start stays unhealthy"
clean_project

# 5. A first start killed after the image initialized the data directory
# (mariadb-install-db) but before the init script wrote its marker. Build
# that data directory with the image's own install step, then start db
# through compose up: the entrypoint sees an initialized data directory and
# skips the schema load, so acis has no tables (it is not even created) and
# root has no password yet. Wait until the server answers and one of its
# healthchecks has rejected it for having no tables.
rc=0
out=$(ACIS_DATAPACK_DIR="$work/ok" compose_timeout 120 run --rm -T --no-deps --name "$broken" \
	--entrypoint mariadb-install-db db \
	--user=mysql --datadir=/var/lib/mysql --auth-root-authentication-method=normal --skip-test-db 2>&1) || rc=$?
[ "$rc" = 0 ] || { echo "$out" >&2; fail "mariadb-install-db exited $rc"; }
killed_ready() {
	compose exec -T db healthcheck.sh --connect >/dev/null 2>&1 || return 1
	if compose logs db 2>&1 | grep -q 'acis schema: loading'; then
		db_state
		fail "db ran the schema load over an initialized data directory"
	fi
}
ACIS_DATAPACK_DIR="$work/ok" compose up -d db >&2 ||
	{ db_state; fail "compose up did not start db over the initialized data directory"; }
wait_rejected 0 'acis database has no tables' killed_ready
echo "smoke: db over a first start killed before the schema load stays unhealthy"
clean_project

# 6. The real datapack schema. Every table file must load under this
# MariaDB image, and acis must then hold one table per sql/*.sql file.
if [ -n "${ACIS_SMOKE_DATAPACK:-}" ]; then
	want=$(find "$ACIS_SMOKE_DATAPACK/sql" -maxdepth 1 -type f -name '*.sql' | wc -l)
	[ "$want" -gt 0 ] || fail "$ACIS_SMOKE_DATAPACK/sql has no .sql files"
	ACIS_DATAPACK_DIR="$ACIS_SMOKE_DATAPACK" compose up -d --wait --wait-timeout 300 db ||
		{ compose logs db >&2; fail "db did not become healthy with $ACIS_SMOKE_DATAPACK"; }
	got=$(acis_tables) || { compose logs db >&2; fail "cannot count acis tables"; }
	[ "$got" = "$want" ] || { compose logs db >&2; fail "acis has $got tables, want $want (one per sql/*.sql)"; }
	echo "smoke: real datapack schema loaded $got tables"
	clean_project
else
	echo "smoke: ACIS_SMOKE_DATAPACK unset; real datapack schema not checked"
fi

echo "smoke: ok"
