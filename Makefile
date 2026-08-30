GO ?= go

.PHONY: test test-unit test-one test-race test-db-up test-db-down

# Full test run: core + behavior suites. Behavior suites read/write a single
# shared MariaDB instance (see docker-compose.test.yml, internal/dbtest) and
# require it to be up: `make test-db-up`.
test:
	$(GO) test ./...

# Fast core-only pass: everything except the tests/ behavior suites.
test-unit:
	$(GO) test $$($(GO) list ./... | grep -v '/tests')

# Single behavior suite: make test-one PKG=tests/items
test-one:
	@test -n "$(PKG)" || { echo "usage: make test-one PKG=<package under tests/, e.g. tests/items>"; exit 1; }
	$(GO) test ./$(PKG)/ -run . -count=1

# Full run with the race detector enabled.
test-race:
	$(GO) test -race ./...

DOCKER_COMPOSE ?= $(shell command -v docker-compose >/dev/null 2>&1 && echo docker-compose || echo "docker compose")

# Start the single shared MariaDB instance used by every integration test.
test-db-up:
	$(DOCKER_COMPOSE) -f docker-compose.test.yml up -d --wait

# Stop and remove it.
test-db-down:
	$(DOCKER_COMPOSE) -f docker-compose.test.yml down
