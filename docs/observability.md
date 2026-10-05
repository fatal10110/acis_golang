# Logs, Metrics, And Failure Modes

Both `cmd/loginserver` and `cmd/gameserver` read the shipped `logging.properties` and write
structured logs. Each can also serve a metrics endpoint. Commands below run from the repository
root and use the startup flags from [run-servers.md](run-servers.md).

## Log Files

Every log path in `logging.properties` is resolved against `-log-root` (default `.`). `%g` is the
rotation generation and `%u` is always `0`. With the shipped file and `-log-root log/gameserver`:

| Sink | `logging.properties` key prefix | File | What it records |
|------|---------------------------------|------|-----------------|
| console | `java.util.logging.FileHandler` | `log/gameserver/log/console/console_0.txt` | every process event at or above `FileHandler.level` |
| error | `net.sf.l2j.commons.logging.handler.ErrorLogHandler` | `log/gameserver/log/error/error_0.txt` | process events at or above `ErrorLogHandler.level` that carry an `error` field. This is the Go form of Java's `ErrorFilter`, which keeps records with a throwable. |
| chat | `...ChatLogHandler` | `log/gameserver/log/chat/chat_0.txt` | chat lines, only when `server.properties` `LogChat = True` |
| gmaudit | `...GMAuditLogHandler` | `log/gameserver/log/gmaudit/gmaudit_0.txt` | admin commands, only when `server.properties` `GMAudit = True` |
| item | `...ItemLogHandler` | `log/gameserver/log/item/item_0.txt` | opened but not written yet: `LogItems` is not ported (#3466) |

Process events at or above `java.util.logging.ConsoleHandler.level` also go to stderr. The
loginserver opens the same five files under its own `-log-root`. Only its console and error files
receive events.

**Give each process its own `-log-root`.** The shipped file has no `.append` keys, so each boot
truncates generation 0. Two processes sharing a root write over each other's files.

Rotation follows the JUL file handler. Once generation 0 reaches `<prefix>.limit` bytes, the files
shift up one generation, the oldest (`<prefix>.count - 1`) is dropped, and a new generation 0
starts. `limit = 0` (or no key) never rotates. `<prefix>.append = true` keeps generation 0 across
restarts instead of truncating it.

## Log Format

Every sink writes one JSON object per line. This is a Go-specific simplification: Java's
`ConsoleLogFormatter`/`FileLogFormatter` write `[date] LEVEL message` text. The files, levels,
rotation keys and routing are unchanged. Fixed fields:

| Field | Value |
|-------|-------|
| `time` | RFC 3339 timestamp, second precision, local zone |
| `level` | `trace`, `debug`, `info`, `warn` or `error` |
| `msg` | the event message |
| `error` | the error text, when the event has one (these events also reach the error file) |

All other fields are typed event context, such as `addr`, `account`, `key` or `port`. For example:

```bash
jq -c 'select(.level == "warn" or .level == "error")' log/gameserver/log/console/console_0.txt
jq -r '"\(.time) \(.msg) \(.error)"' log/gameserver/log/error/error_0.txt
```

Java level names map to Go levels as follows. A name outside this table fails boot.

| `logging.properties` | Go level |
|----------------------|----------|
| `ALL`, `FINEST`, `FINER` | `trace` |
| `FINE`, `CONFIG` | `debug` |
| `INFO` | `info` |
| `WARNING` | `warn` |
| `SEVERE` | `error` |
| `OFF` | disabled |

`.level` is the floor for every sink. Each handler's `.level` can raise its own sink above that
floor, but never lower it below `.level`.

The Go logger reads only the level, pattern, limit, count and append keys above. It accepts the
`handlers`, `*.useParentHandlers`, `*.formatter`, `*.filter`, `net.sf.l2j.gameserver.level` and
`net.sf.l2j.loginserver.level` keys from the shipped file without error, but the Go sink routing
is fixed and they change nothing. Any other key is ignored. At boot the server logs it once as a
`warn` event with message `logging.properties keys not supported; ignored` and a `keys` array.
The shipped file produces no such event.

## Metrics Endpoint

`-debug-addr host:port` serves `expvar` at `/debug/vars` and `net/http/pprof` under
`/debug/pprof/`. The variables and the loopback-only rule are listed under
[Debug Endpoints](run-servers.md#debug-endpoints). Besides `connections`, `players-online` and
`tickers`, Go's `expvar` always publishes `cmdline` and `memstats` (`runtime.MemStats`).

```bash
curl -s http://127.0.0.1:6060/debug/vars | jq '{connections, "players-online", tickers}'
curl -s http://127.0.0.1:6060/debug/vars | jq '.memstats | {HeapAlloc, NumGC, PauseTotalNs}'
```

Give each process its own port: for example, `6061` for the loginserver and `6060` for the
gameserver.

## Failure Modes

A boot failure is printed to stderr by the `fx` lifecycle, and the process exits with status 1.
At that point the log files may not exist yet.

| Condition | Result | Covered by |
|-----------|--------|------------|
| `-logging` file missing or unreadable | boot fails: `open config <path>: ...` | `config.LoadFile` |
| unknown level name in any `*.level` key | boot fails: `unsupported log level "<value>"` | `TestBadLevelFails` |
| non-numeric `*.limit` / `*.count` | boot fails: `parse <key> as int64/int: ...` | `config.Properties` parsers |
| log directory cannot be created or a log file cannot be opened | boot fails with the OS error | `logging.Setup` |
| unsupported `logging.properties` key | boot continues; one `warn` event names the keys | `TestSetupWarnsUnsupportedKeysOnce` |
| missing optional `logging.properties` key (e.g. `.append`) | boot continues with the default; a `config property missing` warning goes to stderr only, because the file sinks are not open yet | `config.Properties` |
| a log file write or rotation fails at runtime (disk full, rename denied) | the event is lost for that sink only. The other sinks still get it, `zerolog: could not write event: ...` goes to stderr, and the server keeps running. | zerolog multi-writer |
| `-debug-addr` cannot be bound (port in use, bad address) | boot fails: `listen for debug http on <addr>: ...` | `TestListenBusyAddrFails` |
| `-debug-addr` omitted | no listener, and nothing is exposed | `TestListenEmptyAddrIsNoop` |
