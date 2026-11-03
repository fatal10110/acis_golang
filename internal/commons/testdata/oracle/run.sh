#!/bin/sh
# Regenerates ../stringhash.golden. ACIS_DATAPACK points at the datapack root
# (the directory holding data/); by default it is the aCis_datapack checkout
# next to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
data=${ACIS_DATAPACK:-$(git -C "$here" rev-parse --show-toplevel)/../aCis_datapack}
docker run --rm -v "$data/data/xml/multisell:/multisell:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine \
	java -Dfile.encoding=UTF-8 StringHashProbe.java > "$here/../stringhash.golden"
