# admin-panel

Separate module (own `go.mod`) — server ops tooling, not game code, same pattern as
[`../deploy-webhook`](../deploy-webhook).

Minimal web UI to view/edit the production server's `.properties` config files and restart the two
game services. Runs as `acis-admin-panel.service`, bound to `127.0.0.1:8081`, behind Caddy at
`admin.iterharbor.com`. Authentication is Caddy's `basic_auth`, not app code — this binds
loopback-only and has no auth of its own.

- Editable files: `server.properties`, `loginserver.properties`, `players.properties`,
  `geoengine.properties`, `banned_ips.properties` (`CONFIG_DIR`, default `/opt/acis/config`).
- Save is line-preserving: only `key = value` lines are edited, everything else (comments, blank
  lines) is written back byte-for-byte — except line endings are normalized to LF even on lines that
  had CRLF, which is harmless (Java `Properties` parsing tolerates both) but will show as a
  whitespace-only diff if you ever compare the file to its CRLF original.
- Every save writes a timestamped `.bak` copy next to the file before overwriting.
- Restart buttons run `sudo systemctl restart acis-loginserver`/`acis-gameserver` — the `panel` user
  has passwordless sudo for exactly those two commands (`/etc/sudoers.d/panel` on the server), nothing else.
- No restart-on-save: saving a config change does not restart the service, restart is a separate button.

Build and deploy a change to this file itself:

```bash
cd ops/admin-panel
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/admin-panel .
ssh hetzner systemctl stop acis-admin-panel
scp /tmp/admin-panel hetzner:/opt/acis/
ssh hetzner 'chown panel:panel /opt/acis/admin-panel && systemctl start acis-admin-panel'
```
