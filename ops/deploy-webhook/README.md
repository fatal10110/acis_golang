# deploy-webhook

Separate module (own `go.mod`, excluded from the main `acis_golang` module's build/vet/test) — this
is server ops tooling, not game code.

Runs on the production Hetzner box as `acis-deploy-webhook.service`, bound to `127.0.0.1:8080`
behind Caddy (TLS at `deploy.iterharbor.com`). On a valid `Authorization: Bearer $DEPLOY_TOKEN`
POST to `/deploy`, it pulls `main`, rebuilds `gameserver`/`loginserver`, and restarts both services
— only swapping binaries if the build succeeds, so a broken build never touches the running server.

Triggered by `.github/workflows/deploy.yml` after the `Go` workflow passes on `main`. See
[`docs/ops/hetzner-server.md`](../../../docs/ops/hetzner-server.md) for the full server setup.

Build and deploy a change to this file itself:

```bash
cd ops/deploy-webhook
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/deploy-webhook .
ssh hetzner systemctl stop acis-deploy-webhook
scp /tmp/deploy-webhook hetzner:/opt/acis/
ssh hetzner 'chown deploy:deploy /opt/acis/deploy-webhook && systemctl start acis-deploy-webhook'
```

## Restarts and connected players

`systemctl restart acis-gameserver` stops the server with SIGTERM. Every client with a character in
the world gets `ServerClose` and drops to the login screen; players log in again once the new
process has booted (~20-30s). The Interlude client does not reconnect by itself.

The server's own worst-case shutdown (`gameServerStopTimeout` in `cmd/gameserver/main.go`) is longer
than systemd's default 90s stop timeout. Install the drop-in in
[`ops/systemd`](../systemd/acis-gameserver.service.d/stop-timeout.conf) once, so systemd never
SIGKILLs a shutdown that is still saving:

```bash
scp ops/systemd/acis-gameserver.service.d/stop-timeout.conf hetzner:/tmp/
ssh hetzner 'install -D -m 644 /tmp/stop-timeout.conf /etc/systemd/system/acis-gameserver.service.d/stop-timeout.conf && systemctl daemon-reload'
```
