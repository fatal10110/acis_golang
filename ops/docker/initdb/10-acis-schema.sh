#!/bin/sh
# Loads the aCis schema into a fresh MariaDB volume. The MariaDB image runs
# this once, on the first start of an empty data directory, with the server
# reachable only on its local socket. docker-compose.yml mounts the
# datapack's tools/ and sql/ directories at /acis/tools and /acis/sql.
#
# This is the full install of aCis_datapack/tools/database_installer.sh:
# full_install.sql drops every table, then each sql/*.sql creates its table
# and loads its rows. The files have no cross-table references, so their
# order does not matter.
#
# Any failure aborts the image's first-start initialization, so that
# container start exits. The data directory already exists by then, so the
# next start (restart: always makes one on its own) skips this script and
# serves an empty or partial acis database. To keep that database from
# passing for a seeded one, the script first writes the marker file
# /var/lib/mysql/.acis-schema-incomplete and removes it only after the last
# table file loads; the db healthcheck in docker-compose.yml fails while the
# marker exists, so services that wait for a healthy db never start. Recover
# with `docker compose down -v` (it deletes the volume), fix the datapack
# mount, and start again.
#
# The body runs in a subshell: the image sources a .sh file that has lost
# its executable bit, and the subshell keeps this script's shell options and
# positional parameters out of the entrypoint either way.
(
set -u

# Keep in step with the db healthcheck in docker-compose.yml.
marker=/var/lib/mysql/.acis-schema-incomplete
: >"$marker" || exit 1

tools=/acis/tools
sqldir=/acis/sql
db=${MARIADB_DATABASE:?MARIADB_DATABASE must name the aCis database}

if [ ! -f "$tools/full_install.sql" ]; then
	echo "acis schema: $tools/full_install.sql not found; set ACIS_DATAPACK_DIR to the aCis_datapack checkout" >&2
	exit 1
fi
set -- "$sqldir"/*.sql
if [ ! -f "$1" ]; then
	echo "acis schema: no .sql files in $sqldir; set ACIS_DATAPACK_DIR to the aCis_datapack checkout" >&2
	exit 1
fi

run_sql() {
	mariadb --protocol=socket -uroot -p"$MARIADB_ROOT_PASSWORD" "$db" <"$1"
}

echo "acis schema: loading $tools/full_install.sql"
run_sql "$tools/full_install.sql" || exit 1
for f in "$@"; do
	echo "acis schema: loading $f"
	run_sql "$f" || exit 1
done
rm -f "$marker" || exit 1
echo "acis schema: loaded $# table files into $db"
)
# A subshell tested by || would run with set -e disabled, so check its
# status separately.
acis_schema_rc=$?
[ "$acis_schema_rc" -eq 0 ] || exit "$acis_schema_rc"
