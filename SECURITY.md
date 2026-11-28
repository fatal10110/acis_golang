# Security policy

## Reporting a vulnerability

Please **do not** report security problems in public issues, pull requests or discussions.

Report them privately through GitHub instead:

1. Open the repository's **Security** tab.
2. Choose **Report a vulnerability**.
3. Describe the problem, how to reproduce it, and what an attacker could do with it.

If that button is not available, open a public issue that only asks the maintainers for a private
way to send a report. Do not include any details of the problem in it.

We will confirm that we received the report, look into it, and tell you what we plan to do. Please
give us reasonable time to release a fix before you share details publicly.

## What to report

Examples of problems we want to hear about:

- a packet that crashes a server, hangs a connection, or uses unbounded memory or CPU;
- a way to bypass login, session-key checks or access levels;
- a way to duplicate items or currency, or to change another player's data;
- a way to reach the GS-LS link or the debug endpoints from outside when they are configured as
  documented;
- SQL injection, path traversal or similar problems in the servers or their tools.

## Supported versions

Fixes go to the `main` branch and ship in the next release. Older releases do not get separate fixes.

## Running a server safely

- Keep the GS-LS link port (`LoginPort`, 9014 by default) closed to the internet. Open it only to
  your game servers ([decision 0010](docs/decisions/0010-secure-defaults.md)).
- Bind `-debug-addr` to `127.0.0.1` only. It serves heap dumps, goroutine stacks and admin APIs
  without authentication ([docs/run-servers.md](docs/run-servers.md#debug-endpoints)).
- Change the default database password (`ACIS_DB_ROOT_PASSWORD` in the Docker setup).
- Keep `hexid.txt` secret. It holds the key a game server uses to link to the login server.
