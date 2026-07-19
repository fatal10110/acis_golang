package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestCharacterStoreCreatePersistsInitialPosition(t *testing.T) {
	rec := &characterStoreRecorder{}
	db := sql.OpenDB(characterStoreConnector{rec: rec})
	t.Cleanup(func() { _ = db.Close() })
	store := NewCharacterStore(db)

	c := &player.Character{
		ID:          0x10000001,
		AccountName: "acct1",
		Name:        "Newbie",
		ClassID:     44,
		BaseClassID: 44,
		Race:        player.RaceOrc,
		Sex:         player.SexMale,
		CharLevel:   1,
		Location:    location.Location{X: -56733, Y: -113459, Z: -690},
		LastHeading: 32768,
	}
	c.SetResourceValues(player.Resources{
		MaxHP: 80, CurrentHP: 80,
		MaxCP: 32, CurrentCP: 32,
		MaxMP: 30, CurrentMP: 30,
	})

	if err := store.Create(context.Background(), c); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !strings.Contains(rec.query, "heading, x, y, z") {
		t.Fatalf("Create() query = %q, missing initial position columns", rec.query)
	}

	values := map[any]bool{}
	for _, arg := range rec.args {
		values[arg.Value] = true
	}
	for _, want := range []any{int64(c.LastHeading), int64(c.Location.X), int64(c.Location.Y), int64(c.Location.Z)} {
		if !values[want] {
			t.Fatalf("Create() args = %#v, missing %v", rec.args, want)
		}
	}
}

func TestCharacterStoreSetPositionPersistsCoordinates(t *testing.T) {
	rec := &characterStoreRecorder{}
	db := sql.OpenDB(characterStoreConnector{rec: rec})
	t.Cleanup(func() { _ = db.Close() })
	store := NewCharacterStore(db)

	loc := location.Location{X: 46160, Y: 41237, Z: -3534}
	if err := store.SetPosition(context.Background(), 0x10000001, loc, 32768); err != nil {
		t.Fatalf("SetPosition() error = %v", err)
	}

	if !strings.Contains(rec.query, "SET heading = ?, x = ?, y = ?, z = ?") {
		t.Fatalf("SetPosition() query = %q, missing position columns", rec.query)
	}

	want := []any{int64(32768), int64(loc.X), int64(loc.Y), int64(loc.Z), int64(0x10000001)}
	if len(rec.args) != len(want) {
		t.Fatalf("SetPosition() args = %#v, want %d args", rec.args, len(want))
	}
	for i, arg := range rec.args {
		if arg.Value != want[i] {
			t.Fatalf("SetPosition() arg %d = %v, want %v; args=%#v", i, arg.Value, want[i], rec.args)
		}
	}
}

type characterStoreRecorder struct {
	query string
	args  []driver.NamedValue
}

type characterStoreConnector struct {
	rec *characterStoreRecorder
}

func (c characterStoreConnector) Connect(context.Context) (driver.Conn, error) {
	return &characterStoreConn{rec: c.rec}, nil
}

func (c characterStoreConnector) Driver() driver.Driver {
	return characterStoreDriver{}
}

type characterStoreDriver struct{}

func (characterStoreDriver) Open(string) (driver.Conn, error) {
	return nil, driver.ErrSkip
}

type characterStoreConn struct {
	rec *characterStoreRecorder
}

func (c *characterStoreConn) Prepare(query string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (c *characterStoreConn) Close() error { return nil }

func (c *characterStoreConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrSkip
}

func (c *characterStoreConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.rec.query = query
	c.rec.args = append([]driver.NamedValue(nil), args...)
	return driver.RowsAffected(1), nil
}
