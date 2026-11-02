# deploy-webhook

Separate module (own `go.mod`, excluded from the main `acis_golang` module's build/vet/test) — this
is server ops tooling, not game code.

Runs on the production Hetzner box as `acis-deploy-webhook.service`, bound to `127.0.0.1:8080`
behind Caddy for TLS. The public webhook URL is the `DEPLOY_URL` repository secret, so the deploy
host never appears in the repository or workflow logs.

`.github/workflows/deploy.yml` runs after the `Go` workflow passes on `main`. It builds
`gameserver`/`loginserver` on the GitHub runner and publishes them, with a `SHA256SUMS` file, as a
prerelease tagged `deploy-<first 12 chars of the commit sha>` on that commit. It then POSTs to
`/deploy` with `Authorization: Bearer $DEPLOY_TOKEN` and `X-Deploy-Tag: <tag>`, and prunes deploy
releases beyond the newest 30 (`KEEP_RELEASES`).

The webhook downloads the release assets from
`https://github.com/$GITHUB_REPO/releases/download/<tag>/` (public, no GitHub credential on the
server; `GITHUB_REPO` defaults to `fatal10110/acis_golang`). It saves each binary as `<name>.new` in
`BIN_DIR` (default `/opt/acis`), checks it is an ELF file matching its `SHA256SUMS` entry, renames
both into place only when both verified, restarts `acis-loginserver` then `acis-gameserver`, and
answers `200` once that is done, so a failed deploy fails the workflow. A second deploy while one
runs gets `409`.

Roll back (or deploy any kept commit) from the server by naming its tag:

```bash
ssh hetzner 'curl -sS --fail-with-body -X POST 127.0.0.1:8080/deploy \
  -H "Authorization: Bearer $(systemctl show -p Environment --value acis-deploy-webhook | tr " " "\n" | sed -n "s/^DEPLOY_TOKEN=//p")" \
  -H "X-Deploy-Tag: deploy-<sha12>"'
```

Nothing is compiled on the server: the box has 4 GB of RAM and the gameserver alone uses about half,
so an on-box `go build` pushed it into the OOM killer.

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
