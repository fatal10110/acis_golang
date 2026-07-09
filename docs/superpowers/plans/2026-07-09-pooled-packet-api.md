# Pooled Packet API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the affected game-server packet encoders use owned pooled frames exclusively and benchmark the actual session send path.

**Architecture:** `Frame*` remains the sole builder API for each affected packet, so callers must hand a `wire.Frame` to `Session.SendFrame`. Existing tests inspect `frame.Bytes()` and release the frame. The network benchmark compares the old payload send path with `FrameUserInfo` handed to `Session.SendFrame` over the discard connection.

**Tech Stack:** Go standard library `testing`, existing `wire.Frame`, `Session`, and `packetWriterPool`.

## Global Constraints

- Preserve byte-identical little-endian framing and cipher behavior.
- Do not add dependencies or send abstractions without a live caller.
- Release every owned frame exactly once.
- Keep `gofmt`, `go vet ./...`, `go build ./...`, and `go test -race ./...` clean apart from documented external fixtures.

---

### Task 1: Remove obsolete unpooled packet APIs

**Files:**
- Modify: `internal/gameserver/network/serverpackets/{userinfo,itemlist,charselectinfo,charselected,skilllist,charcreatefail,charcreateok,chardeletefail,chardeleteok,newcharactersuccess,ssqinfo}.go`
- Test: `internal/gameserver/network/serverpackets/*_test.go`, `internal/gameserver/network/selectenter_test.go`

**Interfaces:**
- Produces: only the existing `Frame*` builders, each returning `wire.Frame` (or `(wire.Frame, error)`).

- [ ] **Step 1: Adapt the existing frame tests**

Replace each test use of an obsolete `Encode*` function with the corresponding `Frame*` call and defer `frame.Release()`.

- [ ] **Step 2: Run the existing frame tests before the API-only refactor**

Run: `go test ./internal/gameserver/network/serverpackets ./internal/gameserver/network`

Expected: PASS. This is an API-only deletion: the owned-frame behavior already exists and is covered by the adapted tests.

- [ ] **Step 3: Remove the unpooled encoder implementations**

Delete each `Encode*` function while retaining shared `write*` helpers and the pooled `Frame*` implementation.

- [ ] **Step 4: Run targeted tests**

Run: `go test ./internal/gameserver/network/serverpackets ./internal/gameserver/network`

Expected: PASS, with the frame byte assertions unchanged in meaning.

### Task 2: Benchmark the real UserInfo send handoff

**Files:**
- Modify: `internal/gameserver/network/session_bench_test.go`
- Modify: `internal/gameserver/network/serverpackets/frame_test.go`

**Interfaces:**
- Consumes: `serverpackets.FrameUserInfo(UserInfoSnapshot) wire.Frame` and `(*Session).SendFrame(wire.Frame) bool`.
- Produces: `BenchmarkSessionSendUserInfoPayload` and `BenchmarkSessionSendUserInfoFrame`.

- [ ] **Step 1: Write the failing benchmark migration**

Move the UserInfo allocation comparison to the network package and make the pooled benchmark call `session.SendFrame(serverpackets.FrameUserInfo(snapshot))`.

- [ ] **Step 2: Run the benchmark compilation check**

Run: `go test ./internal/gameserver/network -run '^$' -bench 'BenchmarkSessionSendUserInfo' -benchtime=1x`

Expected: PASS after Task 1 exposes only `FrameUserInfo`.

- [ ] **Step 3: Remove the builder-only benchmark**

Delete `BenchmarkUserInfoUnpooled` and `BenchmarkUserInfoPooled` from the server-packet tests.

- [ ] **Step 4: Run focused verification**

Run: `go test ./internal/gameserver/network/serverpackets ./internal/gameserver/network`

Expected: PASS.

### Task 3: Format and verify the PR branch

**Files:**
- Modify: all files above only.

- [ ] **Step 1: Format changed Go files**

Run: `gofmt -w <changed-go-files>`.

- [ ] **Step 2: Run static and race checks**

Run: `go vet ./... && go build ./... && go test -race ./...`.

Expected: all commands pass; if the existing datapack fixture is missing, record its independent failure.

- [ ] **Step 3: Commit the verified change**

Run: `git add <changed-go-files> docs/superpowers/plans/2026-07-09-pooled-packet-api.md && git commit -m "Wire pooled packet frames into send benchmarks (#401)"`.
