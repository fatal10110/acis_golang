# 0010. Bind privileged listeners to loopback by default

- **Status:** Accepted
- **Date:** link default changed in #4655
- **Sources:** [run-servers.md](../run-servers.md) (Start Loginserver, Debug Endpoints, IP Ban API),
  [infra-review-2026-09-05.md](../infra-review-2026-09-05.md) (finding 7)

## Context

Besides the client ports, the servers open listeners that give a lot of power to whoever reaches
them:

- **The GS-LS link port** (`LoginPort`, 9014). A connected game server can register, check session
  keys and change account access levels. The reference server binds it on every address when
  `LoginHostname` is missing.
- **The debug listener** (`-debug-addr`). It serves `pprof` (heap dumps, goroutine stacks, the
  command line), `expvar`, the IP ban API, and the character item API, with no authentication.

[0001](0001-stay-compatible-with-acis.md) says we follow the reference. Here, following it would
leave a privileged port open on every interface for anyone who forgets one config line.

## Decision

- When `LoginHostname` is missing, the login server binds the game server link to `127.0.0.1`
  only. To accept game servers from other hosts, set `LoginHostname = *` (as the reference
  `loginserver.properties` and the Docker config bundle do) or a specific address, and firewall
  the port.
- The debug listener is off unless `-debug-addr` is given. The docs require a loopback address.
- The IP ban API answers only loopback clients (`403` otherwise), even when the debug listener is
  bound more widely.

## Consequences

- A forgotten setting fails closed: a game server on another host cannot link until the operator
  opens the port on purpose.
- This is a documented difference from the reference default. Installations that relied on the
  missing key meaning "every address" must set it explicitly.
- Operators reach the debug endpoints from the host itself, or through an SSH tunnel.

## Alternatives considered

- **Keep the reference default (`*`).** Compatible, but unsafe by default for a port that can change
  access levels.
- **Add authentication to the debug listener.** More work and more to get wrong. Loopback-only
  covers the use case, which is an operator on the host.
