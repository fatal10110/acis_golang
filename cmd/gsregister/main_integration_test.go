//go:build integration

package main

import (
	"bytes"
	"context"
	dbsql "database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go/modules/mariadb"

	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
)

// gameserversSchema mirrors aCis_datapack/sql/gameservers.sql verbatim.
const gameserversSchema = "CREATE TABLE IF NOT EXISTS `gameservers` (\n" +
	"  `server_id` int(11) NOT NULL default '0',\n" +
	"  `hexid` varchar(50) NOT NULL default '',\n" +
	"  `host` varchar(50) NOT NULL default '',\n" +
	"  PRIMARY KEY (`server_id`)\n" +
	")"

const serverNamesXML = `<?xml version="1.0" encoding="UTF-8"?>
<serverNames>
	<server id="1" name="Bartz" />
	<server id="2" name="Sieghardt" />
</serverNames>`

func newIntegrationStore(t *testing.T) *sql.GameServerStore {
	t.Helper()
	ctx := context.Background()

	container, err := mariadb.Run(ctx, "mariadb:11")
	if err != nil {
		t.Fatalf("start mariadb container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminate mariadb container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	db, err := dbsql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.ExecContext(ctx, gameserversSchema); err != nil {
		t.Fatalf("create gameservers table: %v", err)
	}
	return sql.NewGameServerStore(db)
}

func loadTestNames(t *testing.T) *manager.ServerNames {
	t.Helper()
	path := filepath.Join(t.TempDir(), "serverNames.xml")
	if err := os.WriteFile(path, []byte(serverNamesXML), 0o644); err != nil {
		t.Fatalf("write server names: %v", err)
	}
	names, err := manager.LoadServerNames(path)
	if err != nil {
		t.Fatalf("load server names: %v", err)
	}
	return names
}

// runScript executes one command-loop session over the scripted input,
// reloading registered servers from the database — each call behaves like a
// fresh start of the tool.
func runScript(t *testing.T, store *sql.GameServerStore, names *manager.ServerNames, dir, input string) string {
	t.Helper()
	var out bytes.Buffer
	if err := run(context.Background(), strings.NewReader(input), &out, names, store, dir); err != nil {
		t.Fatalf("run() unexpected error: %v\noutput:\n%s", err, out.String())
	}
	return out.String()
}

func TestRegisterCleanLifecycle(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	names := loadTestNames(t)
	dir := t.TempDir()

	// Register server 1: row persisted, hexid file written, both agree.
	out := runScript(t, store, names, dir, "1 exit")
	if !strings.Contains(out, "Server registered under 'hexid(server 1).txt'.") {
		t.Fatalf("register output missing confirmation:\n%s", out)
	}

	row, err := store.GameServer(ctx, 1)
	if err != nil {
		t.Fatalf("GameServer(1) after register: %v", err)
	}
	if row.Host != "" {
		t.Errorf("registered host = %q, want empty", row.Host)
	}

	data, err := os.ReadFile(filepath.Join(dir, "hexid(server 1).txt"))
	if err != nil {
		t.Fatalf("read hexid file: %v", err)
	}
	fileHex := ""
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "HexID="); ok {
			fileHex = v
		}
	}
	if fileHex == "" {
		t.Fatalf("hexid file has no HexID line:\n%s", data)
	}
	if got := model.HexKeyText(row.HexID); got != fileHex {
		t.Errorf("database key %q does not match hexid file key %q", got, fileHex)
	}

	// A fresh session sees the registration: duplicate rejected, list stars it.
	out = runScript(t, store, names, dir, "1 list exit")
	if !strings.Contains(out, "This server id is already used.") {
		t.Errorf("duplicate register not rejected:\n%s", out)
	}
	if !strings.Contains(out, "1: Bartz *") {
		t.Errorf("list does not mark id 1 as used:\n%s", out)
	}
	if !strings.Contains(out, "2: Sieghardt \n") {
		t.Errorf("list does not show free id 2:\n%s", out)
	}

	// Unknown and unlisted ids are rejected without touching the database.
	out = runScript(t, store, names, dir, "9 abc exit")
	if !strings.Contains(out, "No name for server id: 9.") {
		t.Errorf("unlisted id not rejected:\n%s", out)
	}
	if !strings.Contains(out, "Type a number or list|clean|cleanall commands.") {
		t.Errorf("non-number not rejected:\n%s", out)
	}

	// Clean removes the row; cleaning it again reports it unused.
	out = runScript(t, store, names, dir, "clean 1 clean 1 exit")
	if !strings.Contains(out, "You successfully dropped gameserver #1.") {
		t.Errorf("clean missing confirmation:\n%s", out)
	}
	if !strings.Contains(out, "This server id isn't used.") {
		t.Errorf("second clean not rejected:\n%s", out)
	}
	if _, err := store.GameServer(ctx, 1); !errors.Is(err, sql.ErrGameServerNotFound) {
		t.Fatalf("GameServer(1) after clean: got err %v, want ErrGameServerNotFound", err)
	}

	// Cleanall requires confirmation, then empties the table.
	out = runScript(t, store, names, dir, "1 2 cleanall n cleanall y exit")
	if !strings.Contains(out, "'cleanall' processus has been aborted.") {
		t.Errorf("declined cleanall missing abort message:\n%s", out)
	}
	if !strings.Contains(out, "You successfully dropped all registered gameservers.") {
		t.Errorf("cleanall missing confirmation:\n%s", out)
	}
	rows, err := store.GameServers(ctx)
	if err != nil {
		t.Fatalf("GameServers() after cleanall: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("table not empty after cleanall: %v", rows)
	}
}
