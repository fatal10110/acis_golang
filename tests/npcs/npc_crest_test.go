package npcs

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: AbstractNpcInfo.NpcInfo (AbstractNpcInfo.java:108-127) shows,
// with npcs.properties ShowNpcCrest (Config.java:736, shipped False), the
// clan id, crest id, alliance id and alliance crest id of the clan owning
// the NPC's castle (Npc.getCastle), else of Npc.getClanId's clan: the
// residence owner, a clan hall's for a hall NPC (Npc.java:1032-1043,
// NpcTemplate.java:148-169). Without the switch every crest field is 0.

const (
	crestClanID   = 0x10000091
	crestID       = 0x2001
	crestAllyID   = crestClanID // the clan leads its own alliance
	crestAllyMark = 0x2002
	crestGuardID  = 35071 // a hostile in the castle's npcs list
)

// npcCrestFields decodes an NpcInfo frame's clan id, clan crest, alliance
// id and alliance crest.
func npcCrestFields(t *testing.T, frame []byte) [4]int32 {
	t.Helper()
	if frame[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("frame %#x is no NpcInfo", frame[0])
	}
	r := wire.NewReader(frame[1:])
	for range 18 { // object id .. walk speeds
		r.ReadInt32()
	}
	for range 4 { // speed multipliers, collision radius and height
		r.ReadFloat64()
	}
	for range 3 { // right hand, chest, left hand
		r.ReadInt32()
	}
	for range 5 { // name shown .. summon animation
		r.ReadUint8()
	}
	r.ReadString()
	r.ReadString()
	for range 4 { // summon, PvP flag, karma, abnormal effect
		r.ReadInt32()
	}
	return [4]int32{r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()}
}

// npcInfoOf is the one NpcInfo about objectID among frames.
func npcInfoOf(t *testing.T, frames [][]byte, objectID int32) []byte {
	t.Helper()
	var found []byte
	for _, f := range frames {
		if len(f) >= 5 && objectFrame(f, serverpackets.OpcodeNPCInfo, objectID) {
			if found != nil {
				t.Fatalf("two NpcInfo frames about %d", objectID)
			}
			found = f
		}
	}
	if found == nil {
		t.Fatalf("no NpcInfo about %d among %d frames", objectID, len(frames))
	}
	return found
}

// crestCastles is Gludio serving the merchant and a guard.
func crestCastles(t *testing.T) *castledata.Table {
	t.Helper()
	gludio, err := castledata.NewCastle(castledata.CastleAttrs{
		ID: taxCastleID, Alias: "gludio_castle", Name: "Gludio Castle", CircletID: 6838,
		Tax: residence.Tax{Rate: taxCastleTax}, NPCs: []int{merchantID, crestGuardID},
	}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err := castledata.NewTable([]*castledata.Castle{gludio})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// bootCrestWorld boots Gludio, owned when owned by Talker's clan, which
// bears a crest and leads an alliance bearing one.
func bootCrestWorld(t *testing.T, owned bool, extra ...gameservertest.Option) *folkWorld {
	t.Helper()
	castleID := 0
	if owned {
		castleID = taxCastleID
	}
	dir := t.TempDir()
	for name, size := range map[string]int{
		"Crest_" + strconv.Itoa(crestID) + ".dds":           256,
		"AllyCrest_" + strconv.Itoa(crestAllyMark) + ".dds": 192,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	crests, _, err := datacache.LoadCrests(dir)
	if err != nil {
		t.Fatal(err)
	}
	opts := append([]gameservertest.Option{
		gameservertest.WithCastles(crestCastles(t)),
		gameservertest.WithCrests(crests),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, s := range []struct {
				q    string
				args []any
			}{
				{"UPDATE characters SET clanid = ? WHERE char_name = 'Talker'", []any{crestClanID}},
				{
					`INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, ally_id, ally_name, leader_id, crest_id, ally_crest_id)
					SELECT ?, 'Lords', 5, ?, ?, 'Realm', obj_Id, ?, ? FROM characters WHERE char_name = 'Talker'`,
					[]any{crestClanID, castleID, crestAllyID, crestID, crestAllyMark},
				},
			} {
				if _, err := db.ExecContext(context.Background(), s.q, s.args...); err != nil {
					t.Fatalf("seed crest clan: %v", err)
				}
			}
		}),
	}, extra...)
	return bootFolkWorld(t, nil, opts...)
}

// spawnFolkFrames places tmpl dx units east of the player and returns what
// the player is shown.
func (w *folkWorld) spawnFolkFrames(t *testing.T, tmpl *npc.Template, dx int) (*npc.Folk, [][]byte) {
	t.Helper()
	f := w.srv.SpawnFolkNPCAt(t, tmpl, location.Location{X: w.at.X + dx, Y: w.at.Y, Z: w.at.Z})
	return f, drainFrames(t, w.c)
}

var ownerCrest = [4]int32{crestClanID, crestID, crestAllyID, crestAllyMark}

// TestCastleNPCInfoCrest pins NpcInfo for a castle's merchant: with
// ShowNpcCrest it shows the owning clan's ids and crests; without it, or
// with a free castle, all four stay 0, the bytes the switch leaves alone.
func TestCastleNPCInfoCrest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		owned bool
		show  bool
		want  [4]int32
	}{
		{"shown", true, true, ownerCrest},
		{"switched off", true, false, [4]int32{}},
		{"free castle", false, true, [4]int32{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var extra []gameservertest.Option
			if tc.show {
				extra = append(extra, gameservertest.WithShowNPCCrest())
			}
			w := bootCrestWorld(t, tc.owned, extra...)
			f, frames := w.spawnFolkFrames(t, folkTemplate("Merchant", merchantID), 50)
			if got := npcCrestFields(t, npcInfoOf(t, frames, f.ObjectID())); got != tc.want {
				t.Fatalf("merchant NpcInfo crest fields = %#x, want %#x", got, tc.want)
			}
			// An NPC of no residence shows none either way.
			other, frames := w.spawnFolkFrames(t, folkTemplate("Folk", 30100), 80)
			if got := npcCrestFields(t, npcInfoOf(t, frames, other.ObjectID())); got != ([4]int32{}) {
				t.Fatalf("NpcInfo crest fields of an NPC of no castle = %#x, want 0", got)
			}
		})
	}
}

// TestCastleNPCInfoCrestOnRefresh pins the crest on the NpcInfo a castle
// NPC is shown again with: the walk stance it idles to on its second AI
// tick (Npc.setWalkOrRun) keeps the owner's crest.
func TestCastleNPCInfoCrestOnRefresh(t *testing.T) {
	t.Parallel()
	w := bootCrestWorld(t, true, gameservertest.WithShowNPCCrest(), gameservertest.WithAITask())
	f, _ := w.spawnFolkFrames(t, folkTemplate("Merchant", merchantID), 50)
	w.tickAI(t)
	frames := aboutFolk(w.tickAI(t), f)
	if got := npcOpcodes(frames); string(got) != string([]byte{serverpackets.OpcodeChangeMoveType, serverpackets.OpcodeNPCInfo}) {
		t.Fatalf("frames about the NPC on its second AI tick = %#x, want ChangeMoveType then NpcInfo", got)
	}
	if got := npcCrestFields(t, frames[1].raw); got != ownerCrest {
		t.Fatalf("refreshed NpcInfo crest fields = %#x, want %#x", got, ownerCrest)
	}
}

// TestCastleHostileNPCInfoCrest pins the crest on a hostile NPC of the
// castle: a guard in its npcs list shows the owner's crest.
func TestCastleHostileNPCInfoCrest(t *testing.T) {
	t.Parallel()
	w := bootCrestWorld(t, true, gameservertest.WithShowNPCCrest())
	tmpl := stillHostileTemplate("Monster")
	tmpl.ID, tmpl.TemplateID = crestGuardID, crestGuardID
	h := w.srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: w.at.X - 100, Y: w.at.Y, Z: w.at.Z})
	if got := npcCrestFields(t, npcInfoOf(t, drainFrames(t, w.c), h.ObjectID())); got != ownerCrest {
		t.Fatalf("guard NpcInfo crest fields = %#x, want %#x", got, ownerCrest)
	}
}

// TestClanHallNPCInfoCrest pins a clan hall NPC's NpcInfo with
// ShowNpcCrest: its hall serves no castle, so it shows the hall owner's
// clan, here bearing no crest and in no alliance.
func TestClanHallNPCInfoCrest(t *testing.T) {
	t.Parallel()
	w := bootHallManager(t, hallManagerSetup{extra: []gameservertest.Option{gameservertest.WithShowNPCCrest()}})
	tmpl := folkTemplate("ClanHallManagerNpc", hallManagerID)
	f, frames := w.spawnFolkFrames(t, tmpl, 90)
	if got, want := npcCrestFields(t, npcInfoOf(t, frames, f.ObjectID())), [4]int32{hallManagerClan, 0, 0, 0}; got != want {
		t.Fatalf("hall manager NpcInfo crest fields = %#x, want %#x", got, want)
	}
}
