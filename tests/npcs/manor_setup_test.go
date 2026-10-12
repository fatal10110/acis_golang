package npcs

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: ManorManagerNpc.onBypassFeedback, CastleChamberlain
// onBypassFeedback (manor_menu_select), RequestSetSeed, RequestSetCrop and
// the ExShowSeedInfo, ExShowCropInfo, ExShowManorDefaultInfo,
// ExShowSeedSetting and ExShowCropSetting windows.
//
// The manor table: Gludio grows Red Coda (seed 5012, crop 5068) and Dark
// Coda (seed 5016, crop 5073); Dion's seed 5100 grows crop 5074. Seeds
// iterate by id modulo 16: 5012, 5016, 5100. Reference prices: 5012 is 10
// (seed bounds 6-100), 5068 is 30 (crop bounds 18-300), 5016 is 5 (3-50),
// 5073 is 51 (30-510), 5100 is 7 (4-70), 5074 is 20.
var (
	manorRedCoda  = manor.Seed{CropID: 5068, SeedID: 5012, MatureID: 5098, Level: 12, Reward1: 1865, Reward2: 1879, CastleID: 1, SeedsLimit: 50, CropsLimit: 80}
	manorDarkCoda = manor.Seed{CropID: 5073, SeedID: 5016, MatureID: 5103, Level: 10, Reward1: 1864, Reward2: 1878, CastleID: 1, SeedsLimit: 100, CropsLimit: 200}
	manorDionSeed = manor.Seed{CropID: 5074, SeedID: 5100, MatureID: 5104, Level: 11, Reward1: 1866, Reward2: 1880, CastleID: 2, SeedsLimit: 30, CropsLimit: 40}
)

func manorSetupSeeds() *manor.Table {
	table := manor.NewTable([]manor.Manor{
		{ID: 1, Name: "gludio", Seeds: []manor.Seed{manorDarkCoda, manorRedCoda}},
		{ID: 2, Name: "dion", Seeds: []manor.Seed{manorDionSeed}},
	})
	prices := map[int32]int32{5012: 10, 5068: 30, 5016: 5, 5073: 51, 5100: 7, 5074: 20}
	table.ApplyReferencePrices(func(id int32) (int32, bool) {
		p, ok := prices[id]
		return p, ok
	})
	return table
}

// manorSetupStart is a modifiable morning: the lists are approved at
// 06:00, the period rolls over at 20:00.
var manorSetupStart = time.Date(2026, 10, 5, 5, 58, 0, 0, time.UTC)

// manorSetupRows are Gludio's stored lists: 10 Dark Coda seeds at 5, 7
// left, and 100 Dark Coda crops wanted at 50, 40 still, paid with reward 1,
// over the running period; 20 Red Coda seeds at 8 over the next one.
var manorSetupRows = []string{
	"INSERT INTO castle_manor_production VALUES (1, 5016, 7, 10, 5, 0), (1, 5012, 20, 20, 8, 1)",
	"INSERT INTO castle_manor_procure VALUES (1, 5073, 40, 100, 50, 1, 0)",
}

func manorSetupPages() map[string]string {
	pages := residencePages()
	for _, file := range []string{"chamberlain/noprivs.htm", "chamberlain/busy.htm"} {
		pages[file] = residencePage(file)
	}
	return pages
}

// bootManorSetup boots the castle world of residenceSetup s with the manor
// running from manorSetupStart over Gludio's stored lists, the harvester
// shop of the manor manager selling adena.
func bootManorSetup(t *testing.T, s residenceSetup) *folkWorld {
	t.Helper()
	cfg := castlemanor.DefaultConfig()
	cfg.SavePeriod = 0
	opts := []gameservertest.Option{
		gameservertest.WithCastles(residenceCastles(t)),
		gameservertest.WithManor(network.ManorConfig{Allowed: true, CropRate: 1, Seeds: manorSetupSeeds()}),
		gameservertest.WithCastleManor(cfg, manorSetupStart),
		noBypassReuse,
		gameservertest.WithBuyLists(buylist.NewList(300000+castleManorID, castleManorID, []buylist.Product{{ItemID: 57, Price: 1, MaxCount: -1}})),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seedResidenceChat(t, db, s)
			for _, q := range manorSetupRows {
				if _, err := db.ExecContext(context.Background(), q); err != nil {
					t.Fatalf("seed manor: %v", err)
				}
			}
		}),
	}
	if s.siege {
		opts = append(opts, gameservertest.WithSieges(siege.DefaultConfig()), gameservertest.WithZones(gludioBattlefield(t)))
	}
	w := bootFolkWorld(t, manorSetupPages(), opts...)
	if s.siege {
		gludio, ok := w.srv.Sieges.Get(taxCastleID)
		if !ok {
			t.Fatal("no Gludio siege")
		}
		gludio.Start()
		drainUntilQuiet(t, w.c)
	}
	return w
}

// le lays out fields as the client reads them: int32 a 4-byte word, uint16
// a 2-byte word, byte one byte.
func le(fields ...any) []byte {
	var b []byte
	for _, f := range fields {
		switch v := f.(type) {
		case int:
			b = binary.LittleEndian.AppendUint32(b, uint32(int32(v)))
		case uint16:
			b = binary.LittleEndian.AppendUint16(b, v)
		case byte:
			b = append(b, v)
		}
	}
	return b
}

// seedRewards is a manor window row's level and reward items for s.
func seedRewards(s manor.Seed) []byte {
	return le(s.Level, byte(1), s.Reward1, byte(1), s.Reward2)
}

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

// wantOnly checks frames are exactly want.
func wantOnly(t *testing.T, what string, frames [][]byte, want ...[]byte) {
	t.Helper()
	if len(frames) != len(want) {
		t.Fatalf("%s: opcodes % x, want %d frames", what, opcodes(frames), len(want))
	}
	for i := range want {
		if !bytes.Equal(frames[i], want[i]) {
			t.Fatalf("%s: frame %d = % x\nwant % x", what, i, frames[i], want[i])
		}
	}
}

func encodeSetSeed(manorID int, rows ...[3]int) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestSetSeed)
	w.WriteInt32(int32(manorID))
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(int32(r[0]))
		w.WriteInt32(int32(r[1]))
		w.WriteInt32(int32(r[2]))
	}
	return w.Bytes()
}

func encodeSetCrop(manorID int, rows ...[4]int) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestSetCrop)
	w.WriteInt32(int32(manorID))
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(int32(r[0]))
		w.WriteInt32(int32(r[1]))
		w.WriteInt32(int32(r[2]))
		w.WriteUint8(byte(r[3]))
	}
	return w.Bytes()
}

// productions and procures are a period list as (id, amount, start, price
// [, reward]) rows.
func productions(list []*castlemanor.Production) [][4]int32 {
	var out [][4]int32
	for _, sp := range list {
		out = append(out, [4]int32{sp.ID, sp.Amount(), sp.StartAmount, sp.Price})
	}
	return out
}

func procures(list []*castlemanor.Procure) [][5]int32 {
	var out [][5]int32
	for _, cp := range list {
		out = append(out, [5]int32{cp.ID, cp.Amount(), cp.StartAmount, cp.Price, cp.Reward})
	}
	return out
}

func spawnManorNPC(t *testing.T, w *folkWorld, kind string, id, dx int) *npc.Folk {
	t.Helper()
	f := w.spawnFolk(t, folkTemplate(kind, id), dx)
	w.selectFolk(t, f)
	return f
}

var (
	opcodeFE         byte = serverpackets.OpcodeExtended
	maintenanceOrder      = []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeSystemMessage}
)

// TestChamberlainManorWindowsAndSettings walks the lord of Gludio through
// its chamberlain's manor windows and the setting requests over one day:
// while modifiable, the setting windows show each Gludio seed with its
// bounds and both periods' settings, the next period shows nothing, and
// the requests keep only the entries within the limits and bounds; once
// approved, the setting windows and requests are refused and the next
// period shows; under maintenance every window is refused.
func TestChamberlainManorWindowsAndSettings(t *testing.T) {
	t.Parallel()
	w := bootManorSetup(t, residenceSetup{})
	m := w.srv.CastleManor
	chamberlain := spawnManorNPC(t, w, "CastleChamberlain", castleChamberlainID, 30)

	wantOnly(t, "seed setting", w.bypass(t, "manor_menu_select?ask=7&state=-1&time=0"), cat(
		le(opcodeFE, uint16(0x1f), 1, 2),
		le(5012), seedRewards(manorRedCoda), le(50, 10, 6, 100), le(0, 0), le(20, 8),
		le(5016), seedRewards(manorDarkCoda), le(100, 5, 3, 50), le(10, 5), le(0, 0),
	))
	wantOnly(t, "crop setting", w.bypass(t, "manor_menu_select?ask=8&state=-1&time=0"), cat(
		le(opcodeFE, uint16(0x20), 1, 2),
		le(5068), seedRewards(manorRedCoda), le(80, 0, 18, 300), le(0, 0, byte(0)), le(0, 0, byte(0)),
		le(5073), seedRewards(manorDarkCoda), le(200, 0, 30, 510), le(100, 50, byte(1)), le(0, 0, byte(0)),
	))
	wantOnly(t, "running seeds", w.bypass(t, "manor_menu_select?ask=3&state=-1&time=0"), cat(
		le(opcodeFE, uint16(0x1c), byte(1), 1, 0, 1), le(5016, 7, 10, 5), seedRewards(manorDarkCoda),
	))
	wantOnly(t, "next seeds before approval", w.bypass(t, "manor_menu_select?ask=3&state=-1&time=1"),
		le(opcodeFE, uint16(0x1c), byte(1), 1, 0, 0))
	wantOnly(t, "running crops", w.bypass(t, "manor_menu_select?ask=4&state=1&time=0"), cat(
		le(opcodeFE, uint16(0x1d), byte(1), 1, 0, 1), le(5073, 40, 100, 50, byte(1)), seedRewards(manorDarkCoda),
	))
	wantOnly(t, "another castle's crops", w.bypass(t, "manor_menu_select?ask=4&state=2&time=0"),
		le(opcodeFE, uint16(0x1d), byte(1), 2, 0, 0))
	wantOnly(t, "manor crops", w.bypass(t, "manor_menu_select?ask=5&state=-1&time=0"), cat(
		le(opcodeFE, uint16(0x1e), byte(1), 3),
		le(5068, 12, 10, 30), le(byte(1), 1865, byte(1), 1879),
		le(5073, 10, 5, 51), le(byte(1), 1864, byte(1), 1878),
		le(5074, 11, 7, 20), le(byte(1), 1866, byte(1), 1880),
	))
	wantOnly(t, "unknown ask", w.bypass(t, "manor_menu_select?ask=1&state=-1&time=0"))

	// Seeds: within the limit and bounds, another castle's seed included,
	// at no sales; over the limit, under or over the bounds, or unknown,
	// left out. The list set answers nothing.
	wantOnly(t, "set seeds", w.send(t, encodeSetSeed(1,
		[3]int{5016, 100, 50}, [3]int{5016, 101, 5}, [3]int{5012, 10, 5}, [3]int{5012, 10, 101},
		[3]int{5100, 30, 4}, [3]int{9999, 1, 1}, [3]int{5012, 0, 6},
	)))
	if got, want := productions(m.SeedProduction(1, true)), [][4]int32{{5016, 100, 100, 50}, {5100, 30, 30, 4}, {5012, 0, 0, 6}}; !equalRows(got, want) {
		t.Fatalf("next seeds = %v, want %v", got, want)
	}
	// Crops: only Gludio's, within the limit and bounds; a crop with no
	// sales is dropped before anything.
	wantOnly(t, "set crops", w.send(t, encodeSetCrop(1,
		[4]int{5073, 200, 30, 2}, [4]int{5073, 201, 50, 1}, [4]int{5068, 10, 17, 1}, [4]int{5068, 10, 300, 1},
		[4]int{5068, 0, 50, 1}, [4]int{5074, 10, 20, 1},
	)))
	if got, want := procures(m.CropProcure(1, true)), [][5]int32{{5073, 200, 200, 30, 2}, {5068, 10, 10, 300, 1}}; !equalRows(got, want) {
		t.Fatalf("next crops = %v, want %v", got, want)
	}
	wantOnly(t, "no crop with sales", w.send(t, encodeSetCrop(1, [4]int{5073, 0, 30, 1})))
	wantOnly(t, "another castle", w.send(t, encodeSetSeed(2, [3]int{5100, 1, 7})), []byte{serverpackets.OpcodeActionFailed})
	if len(m.CropProcure(1, true)) != 2 || len(m.SeedProduction(2, true)) != 0 {
		t.Fatal("a refused request changed a list")
	}

	// The chamberlain answers its manor windows at any distance, but a
	// list is set only within reach of the NPC talked to.
	spawnManorNPC(t, w, "CastleChamberlain", castleChamberlainID, 1000)
	if got := w.bypass(t, "manor_menu_select?ask=5&state=-1&time=0"); len(got) != 1 || got[0][0] != opcodeFE {
		t.Fatalf("far chamberlain window = % x, want ExShowManorDefaultInfo", opcodes(got))
	}
	wantOnly(t, "set seeds out of reach", w.send(t, encodeSetSeed(1, [3]int{5016, 1, 5})), []byte{serverpackets.OpcodeActionFailed})
	if len(m.SeedProduction(1, true)) != 3 {
		t.Fatal("a list was set out of reach")
	}
	w.selectFolk(t, chamberlain)

	w.srv.CastleManorClock.Advance(3 * time.Minute)
	if !m.IsManorApproved() {
		t.Fatalf("manor at 06:01 is %s, want APPROVED", m.Status())
	}
	for _, ask := range []string{"7", "8"} {
		frames := w.bypass(t, "manor_menu_select?ask="+ask+"&state=-1&time=0")
		wantMessages(t, "setting window once approved", frames, serverpackets.SystemMessageManorCannotBeSetUp)
		if len(frames) != 1 {
			t.Fatalf("setting window once approved = % x, want the message alone", opcodes(frames))
		}
	}
	wantOnly(t, "next seeds once approved", w.bypass(t, "manor_menu_select?ask=3&state=-1&time=1"), cat(
		le(opcodeFE, uint16(0x1c), byte(1), 1, 0, 3),
		le(5016, 100, 100, 50), seedRewards(manorDarkCoda),
		le(5100, 30, 30, 4), seedRewards(manorDionSeed),
		le(5012, 0, 0, 6), seedRewards(manorRedCoda),
	))
	wantOnly(t, "set seeds once approved", w.send(t, encodeSetSeed(1, [3]int{5016, 1, 5})), []byte{serverpackets.OpcodeActionFailed})
	wantOnly(t, "set crops once approved", w.send(t, encodeSetCrop(1, [4]int{5073, 1, 50, 1})), []byte{serverpackets.OpcodeActionFailed})
	if len(m.SeedProduction(1, true)) != 3 || len(m.CropProcure(1, true)) != 2 {
		t.Fatal("a request changed an approved list")
	}

	w.srv.CastleManorClock.Advance(13*time.Hour + 59*time.Minute)
	if !m.IsUnderMaintenance() {
		t.Fatalf("manor at 20:00 is %s, want MAINTENANCE", m.Status())
	}
	frames := w.bypass(t, "manor_menu_select?ask=3&state=-1&time=0")
	if !bytes.Equal(opcodes(frames), maintenanceOrder) {
		t.Fatalf("window under maintenance = % x, want ActionFailed then the message", opcodes(frames))
	}
	wantMessages(t, "window under maintenance", frames, serverpackets.SystemMessageManorUnderMaintenance)
}

func equalRows[T comparable](got, want []T) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestManorManagerWindows pins the manor manager's windows for a member of
// no clan, at any distance: the castle's seeds and crops and the manor's
// crops with their buttons shown, the harvester shop and the trade
// windows (pinned in manor_trade_test.go); under maintenance every window
// is refused.
func TestManorManagerWindows(t *testing.T) {
	t.Parallel()
	w := bootManorSetup(t, residenceSetup{clanless: true})
	spawnManorNPC(t, w, "ManorManagerNpc", castleManorID, 1000)

	wantOnly(t, "running seeds", w.bypass(t, "manor_menu_select?ask=3&state=-1&time=0"), cat(
		le(opcodeFE, uint16(0x1c), byte(0), 1, 0, 1), le(5016, 7, 10, 5), seedRewards(manorDarkCoda),
	))
	wantOnly(t, "running crops", w.bypass(t, "manor_menu_select?ask=4&state=-5&time=0"), cat(
		le(opcodeFE, uint16(0x1d), byte(0), 1, 0, 1), le(5073, 40, 100, 50, byte(1)), seedRewards(manorDarkCoda),
	))
	if got := w.bypass(t, "manor_menu_select?ask=5&state=-1&time=0"); len(got) != 1 || !bytes.HasPrefix(got[0], le(opcodeFE, uint16(0x1e), byte(0), 3)) {
		t.Fatalf("manor crops = % x, want ExShowManorDefaultInfo with the buttons", got)
	}
	if got := w.bypass(t, "manor_menu_select?ask=6&state=-1&time=0"); len(got) != 1 || got[0][0] != serverpackets.OpcodeBuyList {
		t.Fatalf("harvester shop = % x, want BuyList", opcodes(got))
	}
	for _, ask := range []string{"1", "2", "9"} {
		if got := w.bypass(t, "manor_menu_select?ask="+ask+"&state=-1&time=0"); len(got) != 1 || got[0][0] == serverpackets.OpcodeActionFailed {
			t.Fatalf("trade window %s = % x, want its window", ask, opcodes(got))
		}
	}
	for _, command := range []string{"manor_menu_select?ask=7&state=-1&time=0", "manor_menu_select?ask=x&state=-1&time=0", "manor_menu_select?ask=3&state=-1"} {
		wantOnly(t, command, w.bypass(t, command))
	}
	wantOnly(t, "set seeds", w.send(t, encodeSetSeed(1, [3]int{5016, 1, 5})), []byte{serverpackets.OpcodeActionFailed})

	w.srv.CastleManorClock.Advance(14*time.Hour + 2*time.Minute)
	frames := w.bypass(t, "manor_menu_select?ask=5&state=-1&time=0")
	if !bytes.Equal(opcodes(frames), maintenanceOrder) {
		t.Fatalf("window under maintenance = % x, want ActionFailed then the message", opcodes(frames))
	}
	wantMessages(t, "window under maintenance", frames, serverpackets.SystemMessageManorUnderMaintenance)
}

// TestManorSetWithoutCurrentNPC pins a setting request from the privileged
// lord of the castle who has talked to no NPC: it answers ActionFailed alone
// and sets nothing; once the lord talks to the chamberlain, the same request
// sets the list.
func TestManorSetWithoutCurrentNPC(t *testing.T) {
	t.Parallel()
	w := bootManorSetup(t, residenceSetup{})
	m := w.srv.CastleManor
	before := productions(m.SeedProduction(1, true))

	wantOnly(t, "set seeds with no NPC talked to", w.send(t, encodeSetSeed(1, [3]int{5016, 1, 5})), []byte{serverpackets.OpcodeActionFailed})
	if got := productions(m.SeedProduction(1, true)); !equalRows(got, before) {
		t.Fatalf("next seeds = %v, want unchanged %v", got, before)
	}

	spawnManorNPC(t, w, "CastleChamberlain", castleChamberlainID, 30)
	wantOnly(t, "set seeds after talking to the chamberlain", w.send(t, encodeSetSeed(1, [3]int{5016, 1, 5})))
	if got, want := productions(m.SeedProduction(1, true)), [][4]int32{{5016, 1, 1, 5}}; !equalRows(got, want) {
		t.Fatalf("next seeds = %v, want %v", got, want)
	}
}

// TestChamberlainManorRefusals pins who the chamberlain's manor windows and
// the setting requests turn away: a member without the manor privilege and
// a clan not owning the castle get the refusal page and ActionFailed, any
// clan member gets the busy page while the siege runs; a member holding the
// privilege is served.
func TestChamberlainManorRefusals(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		s      residenceSetup
		page   string
		allows bool
	}{
		{"member", residenceSetup{rank: 5}, "chamberlain/noprivs.htm", false},
		{"rival", residenceSetup{rival: true}, "chamberlain/noprivs.htm", false},
		{"siege", residenceSetup{siege: true}, "chamberlain/busy.htm", true},
		{"steward", residenceSetup{rank: 5, privs: 65536}, "", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := bootManorSetup(t, c.s)
			chamberlain := spawnManorNPC(t, w, "CastleChamberlain", castleChamberlainID, 30)
			frames := w.bypass(t, "manor_menu_select?ask=7&state=-1&time=0")
			if c.page == "" {
				if len(frames) != 1 || !bytes.HasPrefix(frames[0], le(opcodeFE, uint16(0x1f), 1, 2)) {
					t.Fatalf("seed setting = % x, want ExShowSeedSetting", opcodes(frames))
				}
			} else {
				if got := pageIn(t, frames); !strings.Contains(got, c.page) || len(frames) != 1 {
					t.Fatalf("seed setting = % x %q, want %s alone", opcodes(frames), got, c.page)
				}
				if objectID, _, _ := htmlMessage(t, frames[0]); objectID != chamberlain.ObjectID() {
					t.Fatalf("page of object %d, want the chamberlain", objectID)
				}
			}
			var want [][]byte
			if !c.allows {
				want = [][]byte{{serverpackets.OpcodeActionFailed}}
			}
			wantOnly(t, "set seeds", w.send(t, encodeSetSeed(1, [3]int{5016, 1, 5})), want...)
			want5016 := [][4]int32{{5016, 1, 1, 5}}
			if got := productions(w.srv.CastleManor.SeedProduction(1, true)); equalRows(got, want5016) != c.allows {
				t.Fatalf("next seeds = %v, set = %t", got, c.allows)
			}
		})
	}
}
