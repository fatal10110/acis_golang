GO ?= go

.PHONY: test test-unit test-one test-race

# Full test run: core + behavior suites. Behavior suites use a MariaDB
# testcontainer and require a running Docker daemon.
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
