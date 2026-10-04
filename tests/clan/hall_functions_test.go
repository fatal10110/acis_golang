package clan

import (
	"bytes"
	"context"
	"database/sql"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: ClanHallManager loads each owned hall's clanhall_functions
// rows; ClanHallFunction charges its lease to the owning clan's warehouse
// when its term (endTime) ends, at once at boot for a term already over,
// then every rate, and deletes itself when the warehouse holds too little.
// ClanHallZone.onEnter sends ClanHallDecoration (0xf7), each slot the
// clanHallDeco.xml depth of the function's getFuncLvl (HP level / 20, MP
// and experience level / 5, the level itself otherwise, plus 10 in a
// siegable hall). PlayerStatus.getRegenHp/getRegenMp multiply a clan
// member's regeneration by 1 + level/100 of its own hall's recovery
// function while it stands in any clan hall zone.

const (
	hallClanID    = 0x70000011
	hallAdenaObj  = 0x70000012
	moonstoneHall = 22 // auctionable
	resistance    = 21 // siegable
	onyxHall      = 23 // auctionable, owned by no one here
)

// hallFunctionRow is one clanhall_functions row.
type hallFunctionRow struct {
	hall, funcType, level, lease int
	rate, endTime                int64
}

// hallSetup is the clan hall state a scenario boots with.
type hallSetup struct {
	// owned is the hall the leader's clan owns, 0 for no clan at all.
	owned int
	// zoneHall is the hall whose grounds the spawn point lies in, 0 for
	// none.
	zoneHall  int
	functions []hallFunctionRow
	// warehouseAdena is the adena in the clan's warehouse.
	warehouseAdena int
}

// bootHall boots "Newbie" at its spawn point, leading a clan owning
// s.owned, against the shipped clan halls and decorations.
func bootHall(t *testing.T, s hallSetup) *gameservertest.Server {
	t.Helper()
	datapack.Require(t)
	halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
	if err != nil {
		t.Fatal(err)
	}
	decos, err := gamexml.LoadClanHallDeco(datapack.Path(t, "data", "xml", "clanHallDeco.xml"))
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	if s.zoneHall != 0 {
		form, err := zone.NewCuboid(-100, 100, -100, 100, -10_000, 10_000)
		if err != nil {
			t.Fatal(err)
		}
		set := commons.NewStatSet()
		set.Set("clanHallId", strconv.Itoa(s.zoneHall))
		hz, err := zone.NewClanHall(1, form, set)
		if err != nil {
			t.Fatal(err)
		}
		zones.Add(hz)
	}
	return gameservertest.Boot(t,
		regenClass(),
		gameservertest.WithCharacter("Newbie", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedHall(t, db, s) }),
	)
}

// regenClass is the shared human-fighter class with the datapack human
// fighter CON and MEN and flat HP and MP regeneration tables, which the
// shared class template leaves empty.
func regenClass() gameservertest.Option {
	tmpl := gameservertest.ClassTemplate()
	tmpl.CON, tmpl.MEN = 43, 25
	tmpl.HPRegenTable, tmpl.MPRegenTable = make([]float64, 80), make([]float64, 80)
	for i := range 80 {
		tmpl.HPRegenTable[i], tmpl.MPRegenTable[i] = 2, 0.9
	}
	return gameservertest.WithClassTemplate(tmpl)
}

func seedHall(t *testing.T, db *sql.DB, s hallSetup) {
	t.Helper()
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatalf("seed clan hall: %s: %v", q, err)
		}
	}
	if s.owned != 0 {
		exec("UPDATE characters SET clanid = ? WHERE char_name = 'Newbie'", hallClanID)
		exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
			SELECT ?, 'Hallkeepers', 4, obj_Id FROM characters WHERE char_name = 'Newbie'`, hallClanID)
		exec("INSERT INTO clanhall (id, ownerId, paid) VALUES (?, ?, 1)", s.owned, hallClanID)
	}
	if s.warehouseAdena > 0 {
		exec("INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) VALUES (?, ?, ?, ?, 'CLANWH', 0)",
			hallClanID, hallAdenaObj, item.AdenaID, s.warehouseAdena)
	}
	for _, f := range s.functions {
		exec("INSERT INTO clanhall_functions (hall_id, type, lvl, lease, rate, endTime) VALUES (?, ?, ?, ?, ?, ?)",
			f.hall, f.funcType, f.level, f.lease, f.rate, f.endTime)
	}
}

// decorationIn returns the ClanHallDecoration among frames.
func decorationIn(t *testing.T, frames [][]byte) []byte {
	t.Helper()
	frame, ok := firstOpcode(frames, serverpackets.OpcodeClanHallDecoration)
	if !ok {
		t.Fatalf("no ClanHallDecoration among opcodes % x", opcodes(frames))
	}
	return frame
}

// decoration is the ClanHallDecoration body for hall with the twelve
// slot bytes.
func decoration(hall int, slots ...byte) []byte {
	return append([]byte{serverpackets.OpcodeClanHallDecoration, byte(hall), 0, 0, 0}, slots...)
}

func future() int64 { return time.Now().UnixMilli() + 30*dayMs }

// TestClanHallDecorationOnEnter: a player entering a hall's grounds sees
// each rented function's decoration depth, ahead of the compass update the
// same zone check sends.
func TestClanHallDecorationOnEnter(t *testing.T) {
	t.Parallel()
	end := future()
	for _, tt := range []struct {
		name  string
		setup hallSetup
		want  []byte
	}{
		{
			// HP 80 -> fireplace 4 (depth 1), MP 25 -> rug 5 (2),
			// exp 25 -> chandelier 5 (2), mirror 2 (2), curtain 1 (1),
			// bunting 4 (2), platform 2 (2), machine 1 (1).
			name: "auctionable hall",
			setup: hallSetup{owned: moonstoneHall, zoneHall: moonstoneHall, functions: []hallFunctionRow{
				{moonstoneHall, 1, 80, 1000, dayMs, end},
				{moonstoneHall, 2, 25, 12000, dayMs, end},
				{moonstoneHall, 4, 25, 15000, dayMs, end},
				{moonstoneHall, 5, 2, 6000, 3 * dayMs, end},
				{moonstoneHall, 7, 1, 2000, 7 * dayMs, end},
				{moonstoneHall, 9, 4, 11000, dayMs, end},
				{moonstoneHall, 11, 2, 4000, 3 * dayMs, end},
				{moonstoneHall, 12, 1, 30000, dayMs, end},
			}},
			want: decoration(moonstoneHall, 1, 2, 0, 2, 2, 0, 1, 0, 2, 0, 2, 1),
		},
		{
			// A siegable hall shows 10 levels up: HP 40 -> fireplace 12
			// (depth 2, where fireplace 2 is 1), mirror 11 (1),
			// bunting 15 (1), machine 12 (1).
			name: "siegable hall",
			setup: hallSetup{owned: resistance, zoneHall: resistance, functions: []hallFunctionRow{
				{resistance, 1, 40, 3750, dayMs, end},
				{resistance, 5, 1, 1000, 7 * dayMs, end},
				{resistance, 9, 5, 49000, 7 * dayMs, end},
				{resistance, 12, 2, 160000, 7 * dayMs, end},
			}},
			want: decoration(resistance, 2, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 1),
		},
		{
			// Another clan's hall still shows its own, bare, state.
			name: "hall renting nothing",
			setup: hallSetup{owned: moonstoneHall, zoneHall: onyxHall, functions: []hallFunctionRow{
				{moonstoneHall, 1, 80, 1000, dayMs, end},
			}},
			want: decoration(onyxHall, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0),
		},
		{
			// A free hall's stored rows are not loaded.
			name: "free hall's rows",
			setup: hallSetup{zoneHall: onyxHall, functions: []hallFunctionRow{
				{onyxHall, 1, 80, 1000, dayMs, end},
			}},
			want: decoration(onyxHall, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := bootHall(t, tt.setup)
			frames := startInWorld(t, srv.Client)
			if got := decorationIn(t, frames); !bytes.Equal(got, tt.want) {
				t.Fatalf("ClanHallDecoration = % x, want % x", got, tt.want)
			}
			deco, compass := -1, -1
			for i, f := range frames {
				switch {
				case f[0] == serverpackets.OpcodeClanHallDecoration:
					deco = i
				case f[0] == serverpackets.OpcodeExtended && len(f) > 2 && f[1] == 0x32 && f[2] == 0 && compass < 0:
					compass = i
				}
			}
			if compass < deco {
				t.Fatalf("ClanHallDecoration at %d, compass update at %d; want the decoration first", deco, compass)
			}
		})
	}
}

// TestClanHallNoDecorationOutside: outside any hall's grounds nothing is
// shown.
func TestClanHallNoDecorationOutside(t *testing.T) {
	t.Parallel()
	srv := bootHall(t, hallSetup{owned: moonstoneHall, functions: []hallFunctionRow{
		{moonstoneHall, 1, 80, 1000, dayMs, future()},
	}})
	if _, ok := firstOpcode(startInWorld(t, srv.Client), serverpackets.OpcodeClanHallDecoration); ok {
		t.Fatal("ClanHallDecoration sent outside any clan hall")
	}
}

// TestClanHallRecoveryBonus: a member of a clan whose hall rents HP 80 and
// MP 25 regenerates 1.8x HP and 1.25x MP in any clan hall's grounds, its
// own or another's, and at the base rate elsewhere.
func TestClanHallRecoveryBonus(t *testing.T) {
	t.Parallel()
	end := future()
	rows := []hallFunctionRow{
		{moonstoneHall, 1, 80, 1000, dayMs, end},
		{moonstoneHall, 2, 25, 12000, dayMs, end},
	}
	rates := func(t *testing.T, s hallSetup) (hp, mp float64) {
		t.Helper()
		srv := bootHall(t, s)
		startInWorld(t, srv.Client)
		obj, ok := srv.State.Player(srv.SoleObjectID(t))
		if !ok {
			t.Fatal("player not in the world")
		}
		c, ok := network.OnlineCharacter(obj)
		if !ok {
			t.Fatalf("%T is not an online character", obj)
		}
		return c.HPRegenRate(), c.MPRegenRate()
	}
	baseHP, baseMP := rates(t, hallSetup{owned: moonstoneHall, functions: rows})
	if baseHP <= 0 || baseMP <= 0 {
		t.Fatalf("base regen = %v HP, %v MP; want both positive", baseHP, baseMP)
	}
	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-9 }
	for _, tt := range []struct {
		name         string
		setup        hallSetup
		hpMul, mpMul float64
	}{
		{"own hall", hallSetup{owned: moonstoneHall, zoneHall: moonstoneHall, functions: rows}, 1.8, 1.25},
		{"another clan's hall", hallSetup{owned: moonstoneHall, zoneHall: onyxHall, functions: rows}, 1.8, 1.25},
		{"no clan", hallSetup{zoneHall: moonstoneHall, functions: rows}, 1, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hp, mp := rates(t, tt.setup)
			if !near(hp, baseHP*tt.hpMul) || !near(mp, baseMP*tt.mpMul) {
				t.Fatalf("regen = %v HP, %v MP; want %v, %v (base %v, %v)", hp, mp, baseHP*tt.hpMul, baseMP*tt.mpMul, baseHP, baseMP)
			}
		})
	}
}

// warehouseAdena reads the clan warehouse's adena row, 0 when it is gone.
func warehouseAdena(t *testing.T, srv *gameservertest.Server) int64 {
	t.Helper()
	srv.FlushItems(t)
	var n int64
	err := srv.DB.QueryRowContext(context.Background(),
		"SELECT count FROM items WHERE owner_id = ? AND item_id = ? AND loc = 'CLANWH'", hallClanID, item.AdenaID).Scan(&n)
	if err == sql.ErrNoRows {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// functionRow reads hall's function funcType row: its end time, and
// whether it exists.
func functionRow(t *testing.T, srv *gameservertest.Server, hall, funcType int) (int64, bool) {
	t.Helper()
	srv.FlushItems(t)
	var end int64
	err := srv.DB.QueryRowContext(context.Background(),
		"SELECT endTime FROM clanhall_functions WHERE hall_id = ? AND type = ?", hall, funcType).Scan(&end)
	if err == sql.ErrNoRows {
		return 0, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return end, true
}

// TestClanHallFunctionFees: a term that ended while the server was down is
// paid from the clan warehouse at boot and the next term starts then; each
// later term is paid when it ends. A fee the warehouse cannot pay removes
// the function, row and all, and the hall no longer shows it; a function
// whose term has not ended is left alone.
func TestClanHallFunctionFees(t *testing.T) {
	t.Parallel()
	past := time.Now().UnixMilli() - dayMs
	srv := bootHall(t, hallSetup{
		owned: moonstoneHall,
		functions: []hallFunctionRow{
			{moonstoneHall, 1, 80, 1000, dayMs, past},
			{moonstoneHall, 2, 25, 12000, dayMs, future()},
		},
		warehouseAdena: 2500,
	})
	if !srv.DrivesClock() {
		t.Skip("the terms are waited out on the test clock")
	}
	bootMs := time.Now().UnixMilli()
	srv.Advance(t, 0)
	if got := warehouseAdena(t, srv); got != 1500 {
		t.Fatalf("warehouse adena after the overdue fee = %d, want 1500", got)
	}
	end, ok := functionRow(t, srv, moonstoneHall, 1)
	if slack := time.Minute.Milliseconds(); !ok || end < bootMs+dayMs-slack || end > time.Now().UnixMilli()+dayMs+slack {
		t.Fatalf("HP function row end = %d (present %v), want a day after boot %d", end, ok, bootMs)
	}
	if got := srv.HallFunctions.Level(moonstoneHall, 1); got != 80 {
		t.Fatalf("HP function level = %d, want 80", got)
	}

	srv.Advance(t, 24*time.Hour)
	if got := warehouseAdena(t, srv); got != 500 {
		t.Fatalf("warehouse adena after the second term = %d, want 500", got)
	}
	if next, ok := functionRow(t, srv, moonstoneHall, 1); !ok || next != end+dayMs {
		t.Fatalf("HP function row end after the second term = %d (present %v), want %d", next, ok, end+dayMs)
	}

	srv.Advance(t, 24*time.Hour)
	if got := warehouseAdena(t, srv); got != 500 {
		t.Fatalf("warehouse adena after the unpaid term = %d, want 500 untouched", got)
	}
	if _, ok := functionRow(t, srv, moonstoneHall, 1); ok {
		t.Fatal("unpaid HP function row kept")
	}
	if got := srv.HallFunctions.Level(moonstoneHall, 1); got != 0 {
		t.Fatalf("unpaid HP function level = %d, want 0", got)
	}
	deco, ok := srv.HallFunctions.Decoration(moonstoneHall)
	if !ok || deco.RestoreHP != 0 || deco.RestoreMP != 2 {
		t.Fatalf("decoration after removal = %+v (%v), want HP 0, MP 2", deco, ok)
	}
	if _, ok := functionRow(t, srv, moonstoneHall, 2); !ok {
		t.Fatal("MP function row, not yet due, removed")
	}
}
