GO ?= go

.PHONY: hooks test test-unit test-internal test-one test-race fuzz test-db-up test-db-down

# Enable the formatting and lint pre-commit hook for this clone.
hooks:
	git config core.hooksPath .githooks

# Full test run: core + behavior suites. Behavior suites read/write a single
# shared MariaDB instance (see docker-compose.test.yml, internal/dbtest) and
# require it to be up: `make test-db-up`.
test:
	$(GO) test ./...

# Fast DB-free pure-core pass for edit feedback.
test-unit:
	$(GO) test ./cmd/testtiming ./internal/config ./internal/commons/crypt ./internal/commons/wire ./internal/link ./internal/loginserver/crypt ./internal/gameserver/network/cipher ./internal/gameserver/network/clientpackets ./internal/gameserver/network/serverpackets ./internal/gameserver/skill/formulas ./internal/gameserver/skill/stat ./internal/gameserver/skill/statbonus

# Broad internal pass, including DB and socket tests outside tests/.
test-internal:
	$(GO) test $$($(GO) list ./... | grep -v '/tests')

# Single behavior suite: make test-one PKG=tests/items
test-one:
	@test -n "$(PKG)" || { echo "usage: make test-one PKG=<package under tests/, e.g. tests/items>"; exit 1; }
	$(GO) test ./$(PKG)/ -run '$(or $(RUN),.)' -count=1

# Full run with the race detector enabled.
test-race:
	$(GO) test -race ./...

# Inbound-packet fuzz targets as package:Target pairs; `go test -fuzz` takes
# one package and one target per run. `go test` alone runs only their seeds.
FUZZ_TARGETS = \
	./internal/commons/wire:FuzzFrameReader \
	./internal/commons/wire:FuzzReader \
	./internal/commons/crypt:FuzzLinkCryptDecrypt \
	./internal/loginserver/crypt:FuzzLoginCryptDecrypt \
	./internal/loginserver/network/clientpackets:FuzzLoginClientPackets \
	./internal/gameserver/network/clientpackets:FuzzGameClientPackets \
	./internal/link:FuzzLinkPackets
FUZZTIME ?= 60s

# Fuzz every inbound-packet target for FUZZTIME each, or one with
# FUZZ=<Target>: make fuzz FUZZ=FuzzLinkPackets FUZZTIME=5m
# Needs no database. A failing input is saved under the package's
# testdata/fuzz/ directory and replays in every later `go test`.
fuzz:
	@ran=0; for t in $(FUZZ_TARGETS); do \
		pkg=$${t%%:*}; name=$${t##*:}; \
		if [ -n "$(FUZZ)" ] && [ "$(FUZZ)" != "$$name" ]; then continue; fi; \
		ran=1; echo "== $$name ($$pkg, $(FUZZTIME))"; \
		$(GO) test "$$pkg" -run '^$$' -fuzz "^$$name\$$" -fuzztime $(FUZZTIME) || exit 1; \
	done; \
	[ $$ran = 1 ] || { echo "unknown FUZZ=$(FUZZ); targets: $(foreach t,$(FUZZ_TARGETS),$(lastword $(subst :, ,$(t))))"; exit 1; }

DOCKER_COMPOSE ?= $(shell command -v docker-compose >/dev/null 2>&1 && echo docker-compose || echo "docker compose")

# The project name is pinned because container_name is fixed: compose would
# otherwise name the project after the checkout directory, so `down` from a
# different worktree would miss the container and `up` would hit a name
# conflict.
TEST_DB_COMPOSE = $(DOCKER_COMPOSE) -p acis-test -f docker-compose.test.yml

# Start the single shared MariaDB instance used by every integration test.
test-db-up:
	$(TEST_DB_COMPOSE) up -d --wait

# Stop and remove it.
test-db-down:
	$(TEST_DB_COMPOSE) down
