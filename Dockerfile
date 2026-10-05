# Image for the login server, game server, and their operator tools.
# docker-compose.yml builds it for the loginserver and gameserver services;
# see docs/ops/docker.md.
#
# The image holds binaries only. Config (config/*.properties, hexid.txt) and
# the datapack (data/xml, data/html, data/geodata, data/crests,
# serverNames.xml) are mounted at run time under /opt/acis, the same
# config/ + data/ layout the reference server runs from, so every flag
# default (config/server.properties, -data-root .) resolves unchanged.

ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Static binaries, as the production deploy builds them.
RUN CGO_ENABLED=0 go build -trimpath -o /out/ \
	./cmd/gameserver ./cmd/loginserver ./cmd/gsregister ./cmd/accountmgr

FROM alpine:3.22
# tzdata: the server schedules Seven Signs, Olympiad, sieges and other
# timers on local wall-clock time, read from TZ (see docs/ops/docker.md).
RUN apk add --no-cache tzdata ca-certificates \
	&& mkdir -p /opt/acis/config /opt/acis/data /opt/acis/log
COPY --from=build /out/ /usr/local/bin/
WORKDIR /opt/acis
# docker-compose.yml runs the servers as the host user that owns the mounted
# config, datapack and log directories; this is only the bare-run default.
USER 1000:1000
# 2106 login clients, 9014 game server link, 7777 game clients.
EXPOSE 2106 9014 7777
CMD ["gameserver"]
