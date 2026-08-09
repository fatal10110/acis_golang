package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

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
	if !strings.Contains(rec.query, "online, lastAccess") {
		t.Fatalf("Create() query = %q, missing offline recency columns", rec.query)
	}
	if got := rec.args[len(rec.args)-2].Value; got != int64(0) {
		t.Fatalf("Create() online = %#v, want 0", got)
	}
	lastAccess, ok := rec.args[len(rec.args)-1].Value.(int64)
	if !ok || lastAccess < time.Now().Add(-time.Minute).UnixMilli() || lastAccess > time.Now().Add(time.Minute).UnixMilli() {
		t.Fatalf("Create() lastAccess = %#v, want current epoch milliseconds", rec.args[len(rec.args)-1].Value)
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

func TestCharacterStoreSavePersistsFullStats(t *testing.T) {
	rec := &characterStoreRecorder{}
	db := sql.OpenDB(characterStoreConnector{rec: rec})
	t.Cleanup(func() { _ = db.Close() })
	store := NewCharacterStore(db)

	c := &player.Character{
		ID:        0x10000001,
		CharLevel: 5,
		Exp:       12345,
		SP:        67,
	}
	c.SetResourceValues(player.Resources{
		MaxHP: 200, CurrentHP: 150,
		MaxCP: 90, CurrentCP: 45,
		MaxMP: 70, CurrentMP: 60,
	})
	c.KarmaPoints = 240
	c.PvPKills = 3
	c.PKKills = 1
	c.SetDeathPenaltyLevel(4)

	if err := store.Save(context.Background(), c); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if !strings.Contains(rec.query, "SET level = ?, maxHp = ?, curHp = ?, maxCp = ?, curCp = ?, maxMp = ?, curMp = ?, exp = ?, sp = ?, karma = ?, pvpkills = ?, pkkills = ?, death_penalty_level = ?") {
		t.Fatalf("Save() query = %q, missing full-stat columns", rec.query)
	}
	want := []any{int64(5), 200.0, 150.0, 90.0, 45.0, 70.0, 60.0, int64(12345), int64(67), int64(240), int64(3), int64(1), int64(4), int64(0x10000001)}
	if len(rec.args) != len(want) {
		t.Fatalf("Save() args = %#v, want %d args", rec.args, len(want))
	}
	for i, arg := range rec.args {
		if arg.Value != want[i] {
			t.Fatalf("Save() arg %d = %v, want %v; args=%#v", i, arg.Value, want[i], rec.args)
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

func TestScanCharacterReadsHeroFlag(t *testing.T) {
	c, err := scanCharacter(characterScanRow{
		int32(0x10000001), "acct1", "Newbie",
		1, 80.0, 80.0, 32.0, 32.0, 30.0, 30.0,
		1, 2, 3, byte(player.SexMale),
		32768, -56733, -113459, -690,
		int64(0), 0, 0, 0, 0, 0,
		int(player.RaceHuman), 0, 0,
		int64(0), "", 0, 1, int64(0),
		0,
	})
	if err != nil {
		t.Fatalf("scanCharacter() error = %v", err)
	}
	if !c.IsHero() {
		t.Fatal("scanCharacter() returned non-hero for hero = 1")
	}
}

func TestScanCharacterReadsDeathPenaltyLevel(t *testing.T) {
	c, err := scanCharacter(characterScanRow{
		int32(0x10000001), "acct1", "Newbie",
		1, 80.0, 80.0, 32.0, 32.0, 30.0, 30.0,
		1, 2, 3, byte(player.SexMale),
		32768, -56733, -113459, -690,
		int64(0), 0, 0, 0, 0, 0,
		int(player.RaceHuman), 0, 0,
		int64(0), "", 0, 0, int64(0),
		7,
	})
	if err != nil {
		t.Fatalf("scanCharacter() error = %v", err)
	}
	if got := c.DeathPenaltyLevel(); got != 7 {
		t.Fatalf("scanCharacter() DeathPenaltyLevel() = %d, want 7", got)
	}
}

func TestCharacterStoreSetDeathPenaltyLevelPersists(t *testing.T) {
	rec := &characterStoreRecorder{}
	db := sql.OpenDB(characterStoreConnector{rec: rec})
	t.Cleanup(func() { _ = db.Close() })
	store := NewCharacterStore(db)

	if err := store.SetDeathPenaltyLevel(context.Background(), 0x10000001, 5); err != nil {
		t.Fatalf("SetDeathPenaltyLevel() error = %v", err)
	}

	if !strings.Contains(rec.query, "SET death_penalty_level = ?") {
		t.Fatalf("SetDeathPenaltyLevel() query = %q, missing death_penalty_level column", rec.query)
	}
	want := []any{int64(5), int64(0x10000001)}
	if len(rec.args) != len(want) {
		t.Fatalf("SetDeathPenaltyLevel() args = %#v, want %d args", rec.args, len(want))
	}
	for i, arg := range rec.args {
		if arg.Value != want[i] {
			t.Fatalf("SetDeathPenaltyLevel() arg %d = %v, want %v; args=%#v", i, arg.Value, want[i], rec.args)
		}
	}
}

func TestCharacterStoreSetOfflinePersistsRecency(t *testing.T) {
	rec := &characterStoreRecorder{}
	db := sql.OpenDB(characterStoreConnector{rec: rec})
	t.Cleanup(func() { _ = db.Close() })
	store := NewCharacterStore(db)

	lastAccess := time.Now().UnixMilli()
	if err := store.SetOffline(context.Background(), 0x10000001, lastAccess); err != nil {
		t.Fatalf("SetOffline() error = %v", err)
	}

	if !strings.Contains(rec.query, "SET online = 0, lastAccess = ?") {
		t.Fatalf("SetOffline() query = %q, missing offline recency columns", rec.query)
	}
	want := []any{lastAccess, int64(0x10000001)}
	if len(rec.args) != len(want) {
		t.Fatalf("SetOffline() args = %#v, want %d args", rec.args, len(want))
	}
	for i, arg := range rec.args {
		if arg.Value != want[i] {
			t.Fatalf("SetOffline() arg %d = %v, want %v; args=%#v", i, arg.Value, want[i], rec.args)
		}
	}
}

type characterScanRow []any

func (r characterScanRow) Scan(dest ...any) error {
	if len(dest) != len(r) {
		return fmt.Errorf("scan dest count = %d, want %d", len(dest), len(r))
	}
	for i := range r {
		dst := reflect.ValueOf(dest[i])
		if dst.Kind() != reflect.Pointer || dst.IsNil() {
			return fmt.Errorf("dest %d is not a non-nil pointer", i)
		}
		src := reflect.ValueOf(r[i])
		elem := dst.Elem()
		if src.Type().AssignableTo(elem.Type()) {
			elem.Set(src)
			continue
		}
		if src.Type().ConvertibleTo(elem.Type()) {
			elem.Set(src.Convert(elem.Type()))
			continue
		}
		return fmt.Errorf("cannot scan %T into %s at %d", r[i], elem.Type(), i)
	}
	return nil
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
