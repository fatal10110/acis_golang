package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
)

// register makes server id registered with the hexid file at path agreeing
// with the database, without prompting, so a deploy script can run it on
// every start:
//
//   - neither registered nor a file: register a new key and write the file;
//   - registered, no file: write the file from the stored key;
//   - a file for id, not registered (the database was recreated): register
//     the file's key;
//   - both, with the same id and key: change nothing.
//
// A file naming another server id, or another key than the registered one,
// is an error and both stay untouched: replacing either would lock out the
// game server that uses the other.
func register(ctx context.Context, out io.Writer, names *manager.ServerNames, store *sql.GameServerStore, id int, path string, now time.Time) error {
	if len(names.IDs()) == 0 {
		return errors.New("no server names available; check the -names file")
	}
	if _, ok := names.Name(id); !ok {
		return fmt.Errorf("no name for server id %d in the server name list", id)
	}

	fileID, fileKey, err := readHexIDFile(path)
	haveFile := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	row, err := store.GameServer(ctx, id)
	registered := err == nil
	if err != nil && !errors.Is(err, sql.ErrGameServerNotFound) {
		return err
	}

	if haveFile && (fileID != id || registered && model.HexKeyText(fileKey) != model.HexKeyText(row.HexID)) {
		return fmt.Errorf("%s holds ServerID=%d HexID=%s, which differs from the registration of server %d; remove the file to rewrite it from the database, or unregister the server",
			path, fileID, model.HexKeyText(fileKey), id)
	}

	switch {
	case registered && haveFile:
		fmt.Fprintf(out, "Server %d is already registered with the hexid in '%s'.\n", id, path)
		return nil
	case registered:
		if err := model.WriteHexIDFile(path, id, row.HexID, now); err != nil {
			return fmt.Errorf("save hex ID to '%s': %w", path, err)
		}
		fmt.Fprintf(out, "Server %d is already registered; wrote its hexid to '%s'.\n", id, path)
		return nil
	case haveFile:
		if err := store.CreateGameServer(ctx, model.NewGameServer(id, fileKey, "")); err != nil {
			return err
		}
		fmt.Fprintf(out, "Server %d registered with the hexid in '%s'.\n", id, path)
		return nil
	}

	key := make([]byte, authKeyLen)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("generate auth key: %w", err)
	}
	if err := store.CreateGameServer(ctx, model.NewGameServer(id, key, "")); err != nil {
		return err
	}
	// A failed write leaves the registration in place; the next run writes
	// the file from the stored key.
	if err := model.WriteHexIDFile(path, id, key, now); err != nil {
		return fmt.Errorf("save hex ID to '%s': %w", path, err)
	}
	fmt.Fprintf(out, "Server %d registered under '%s'.\n", id, path)
	return nil
}

// readHexIDFile returns the server id and key a hexid file holds, read as
// the game server reads it. A missing file is reported as fs.ErrNotExist.
func readHexIDFile(path string) (int, []byte, error) {
	if _, err := os.Stat(path); err != nil {
		return 0, nil, err
	}
	props, err := config.LoadFile(path)
	if err != nil {
		return 0, nil, err
	}
	idText, ok := props.Lookup("ServerID")
	if !ok {
		return 0, nil, fmt.Errorf("%s: missing ServerID", path)
	}
	id, err := strconv.ParseInt(idText, 10, 32)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: parse ServerID: %w", path, err)
	}
	hexText, ok := props.Lookup("HexID")
	if !ok {
		return 0, nil, fmt.Errorf("%s: missing HexID", path)
	}
	key, err := model.ParseHexKey(hexText)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: %w", path, err)
	}
	return int(id), key, nil
}
