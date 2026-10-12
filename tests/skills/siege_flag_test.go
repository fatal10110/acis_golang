package skills

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: L2SkillSiegeFlag.check refuses, in this order, a caster whose
// clan does not attack the castle siege in progress where it stands
// (S1_CANNOT_BE_USED naming Build Headquarters level 1, whichever flag
// skill was cast), one that does not lead its clan
// (ONLY_CLAN_LEADER_CAN_ISSUE_COMMANDS), a clan that holds a flag
// (NOT_ANOTHER_HEADQUARTERS), a caster outside an HQ zone
// (NOT_SET_UP_BASE_HERE) and another SiegeFlag within 400
// (HEADQUARTERS_TOO_CLOSE). PlayerCast.canCast runs it with the message
// told; useSkill runs it again silently, then spawns NPC 35062 as a
// SiegeFlag named after the clan, with 50000 HP (100000 advanced), 500
// P.Def and M.Def, run speed 0, radius 10 and height 80, and no built-in
// stat bonuses (SiegeFlag.addFuncsToNewCharacter).

const (
	buildHQSkillID    int32 = 247
	buildAdvHQSkillID int32 = 326

	hqClanID        int32 = 0x7f000301
	hqRivalClanID   int32 = 0x7f000302
	hqRivalLeaderID int32 = 0x7f000303
	hqDeputyID      int32 = 0x7f000304

	// hqCostItemID stands in for the flag skills' Gemstone cost: the
	// harness item catalog does not carry the shipped one, and the cost
	// is the generic cast cost.
	hqCostItemID int32 = 20
	hqCostHeld   int32 = 1000
	hqCostCount        = 300
)

// hqSetup is where the caster stands and what it is when it puts up a
// flag.
type hqSetup struct {
	// defends registers the caster's clan to defend Gludio, the rival
	// attacking; otherwise the caster's clan attacks.
	defends bool
	// offBattlefield puts Gludio's battlefield away from the caster.
	offBattlefield bool
	// offGround puts the HQ zone away from the caster.
	offGround bool
	// deputy makes the caster a member of its clan, led by an offline
	// deputy.
	deputy bool
}

// hqWorld is a booted server over the shipped castles with Gludio's siege
// under way and the caster in the world on its battlefield.
type hqWorld struct {
	srv    *gameservertest.Server
	c      *testsupport.ScriptedClient
	objID  int32
	at     location.Location
	gludio *siege.Siege
	clan   *clan.Clan
	rivals *clan.Clan
}

// hqSkills are the shipped flag skills, their cost moved to hqCostItemID.
func hqSkills(t *testing.T) []modelskill.Definition {
	t.Helper()
	var defs []modelskill.Definition
	for _, id := range []int32{buildHQSkillID, buildAdvHQSkillID} {
		def := shippedSkill(t, modelskill.ID(id), 1)
		if def.SkillType != "SIEGE_FLAG" || def.ItemConsumeCount != hqCostCount {
			t.Fatalf("shipped skill %d = %s costing %d, want SIEGE_FLAG costing %d", id, def.SkillType, def.ItemConsumeCount, hqCostCount)
		}
		def.ItemConsumeID = int(hqCostItemID)
		defs = append(defs, def)
	}
	return defs
}

func bootHQ(t *testing.T, setup hqSetup) *hqWorld {
	t.Helper()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	zones := zone.NewIndex()
	field := zone.NewSiege(1, gateBox(t, !setup.offBattlefield))
	field.ResidenceID = gludioCastleID
	zones.Add(field)
	zones.Add(zone.NewHQ(2, gateBox(t, !setup.offGround)))
	side, rivalSide := "ATTACKER", "DEFENDER"
	if setup.defends {
		side, rivalSide = "DEFENDER", "ATTACKER"
	}
	leader := "(SELECT obj_Id FROM characters WHERE char_name = 'Sieger')"
	if setup.deputy {
		leader = "?"
	}
	defs := hqSkills(t)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Sieger", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithZones(zones),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			leaderArgs := []any{hqClanID}
			if setup.deputy {
				leaderArgs = append(leaderArgs, hqDeputyID)
			}
			for _, s := range []struct {
				q    string
				args []any
			}{
				{"UPDATE characters SET clanid = ?, curMp = 500 WHERE char_name = 'Sieger'", []any{hqClanID}},
				{"INSERT INTO characters (account_name, obj_Id, char_name, level, clanid) VALUES ('deputy', ?, 'Deputy', 40, ?)", []any{hqDeputyID, hqClanID}},
				{"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (?, 'Siegers', 5, " + leader + ")", leaderArgs},
				{"INSERT INTO characters (account_name, obj_Id, char_name, level, clanid) VALUES ('rival', ?, 'Red', 40, ?)", []any{hqRivalLeaderID, hqRivalClanID}},
				{"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (?, 'Rivals', 5, ?)", []any{hqRivalClanID, hqRivalLeaderID}},
				{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, ?)", []any{gludioCastleID, hqClanID, side}},
				{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, ?)", []any{gludioCastleID, hqRivalClanID, rivalSide}},
			} {
				if _, err := db.ExecContext(context.Background(), s.q, s.args...); err != nil {
					t.Fatalf("seed siege clans: %v", err)
				}
			}
		}),
	)
	if !srv.DrivesClock() {
		t.Skip("the flag cast's hit time needs the driven clock")
	}
	objID := srv.SoleObjectID(t)
	for _, def := range defs {
		seedKnownSkill(t, srv, objID, int(def.ID), 1)
	}
	srv.GiveItem(t, objID, hqCostItemID, hqCostHeld)
	gludio, ok := srv.Sieges.Get(gludioCastleID)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	// The siege starts before the caster comes in, so its start does not
	// banish the caster off the battlefield.
	gludio.Start()
	if !gludio.InProgress() {
		t.Fatal("Gludio siege not under way")
	}
	enterBesieged(t, srv.Client)
	w := &hqWorld{srv: srv, c: srv.Client, objID: objID, gludio: gludio}
	x, y, z := srv.PlayerPosition(t, objID)
	w.at = location.Location{X: x, Y: y, Z: z}
	w.clan = hqClan(t, srv, hqClanID)
	w.rivals = hqClan(t, srv, hqRivalClanID)
	return w
}

func hqClan(t *testing.T, srv *gameservertest.Server, id int32) *clan.Clan {
	t.Helper()
	cl, ok := srv.Clans.Table().Get(id)
	if !ok {
		t.Fatalf("clan %#x not loaded", id)
	}
	return cl
}

// flags returns every siege headquarters in the world.
func (w *hqWorld) flags() []*npc.Folk {
	var out []*npc.Folk
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && f.SiegeFlag() {
			out = append(out, f)
		}
	}
	return out
}

// costHeld is how many of the flag cost items the caster holds.
func (w *hqWorld) costHeld(t *testing.T) int {
	return w.srv.PlayerInventory(t, w.objID).ItemCount(hqCostItemID, -1, true)
}

// otherFlag stands a headquarters of a clan named name dx east of the
// caster.
func (w *hqWorld) otherFlag(t *testing.T, name string, dx int) *npc.Folk {
	t.Helper()
	f := w.srv.SpawnFolkNPCAt(t, npc.SiegeFlagTemplate(name, false), location.Location{X: w.at.X + dx, Y: w.at.Y, Z: w.at.Z})
	drainUntilQuiet(t, w.c)
	return f
}

// putUp casts skillID and lets it land, returning every frame the caster
// was sent meanwhile.
func (w *hqWorld) putUp(t *testing.T, skillID int32) [][]byte {
	t.Helper()
	w.c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	frames := collectUntilQuiet(t, w.c)
	if !slices.Contains(opcodes(frames), serverpackets.OpcodeMagicSkillUse) {
		t.Fatalf("flag cast %d answered %x, want it started", skillID, opcodes(frames))
	}
	w.srv.Advance(t, 5*time.Second)
	return append(frames, collectUntilQuiet(t, w.c)...)
}

// flagShown returns the NpcInfo among frames that shows the headquarters
// f, failing when there is none.
func flagShown(t *testing.T, frames [][]byte, f *npc.Folk) []byte {
	t.Helper()
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeNPCInfo && wireReader(frame[1:]).ReadInt32() == f.ObjectID() {
			return frame
		}
	}
	t.Fatalf("headquarters %d never shown; got %x", f.ObjectID(), opcodes(frames))
	return nil
}

// npcInfoNameTitle reads NpcInfo's template id, collision body, name and
// title.
func npcInfoNameTitle(t *testing.T, frame []byte) (templateID int32, radius, height float64, name, title string) {
	t.Helper()
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // object id
	templateID = r.ReadInt32()
	for range 16 { // attackable, position, heading, 0, attack speeds, move speeds
		r.ReadInt32()
	}
	r.ReadFloat64()
	r.ReadFloat64()
	radius, height = r.ReadFloat64(), r.ReadFloat64()
	for range 3 { // hands and chest
		r.ReadInt32()
	}
	for range 5 {
		r.ReadUint8()
	}
	name, title = r.ReadString(), r.ReadString()
	if err := r.Err(); err != nil {
		t.Fatalf("read NpcInfo: %v", err)
	}
	return templateID, radius, height, name, title
}

// TestSiegeFlagCastRefusals drives RequestMagicSkillUse for the flag skills
// into each refusal of the headquarters check, two refusals holding at once
// where the order matters. Each answers its message alone: no
// ActionFailed, no cast, no MP or item spent and no headquarters put up.
func TestSiegeFlagCastRefusals(t *testing.T) {
	t.Parallel()
	cannotBeUsed := func(t *testing.T, f []byte) {
		assertSystemMessageSkillFrame(t, f, serverpackets.SystemMessageS1CannotBeUsed, buildHQSkillID, 1)
	}
	static := func(id int) func(*testing.T, []byte) {
		return func(t *testing.T, f []byte) { assertStaticSystemMessage(t, f, id) }
	}
	for _, tt := range []struct {
		name    string
		setup   hqSetup
		skillID int32
		// held puts up a headquarters of the caster's clan far away first;
		// near stands another clan's headquarters 200 from the caster.
		held, near bool
		refuse     func(*testing.T, []byte)
	}{
		{name: "defender", setup: hqSetup{defends: true}, refuse: cannotBeUsed},
		{name: "defender casting the advanced flag names Build Headquarters", setup: hqSetup{defends: true}, skillID: buildAdvHQSkillID, refuse: cannotBeUsed},
		{name: "attacker off the battlefield", setup: hqSetup{offBattlefield: true}, refuse: cannotBeUsed},
		{name: "attacker off the battlefield and off the HQ ground", setup: hqSetup{offBattlefield: true, offGround: true}, refuse: cannotBeUsed},
		{name: "member", setup: hqSetup{deputy: true}, refuse: static(serverpackets.SystemMessageOnlyClanLeaderCanIssueCommands)},
		{name: "member off the HQ ground", setup: hqSetup{deputy: true, offGround: true}, refuse: static(serverpackets.SystemMessageOnlyClanLeaderCanIssueCommands)},
		{name: "flag held", held: true, refuse: static(serverpackets.SystemMessageNotAnotherHeadquarters)},
		{name: "flag held off the HQ ground", setup: hqSetup{offGround: true}, held: true, refuse: static(serverpackets.SystemMessageNotAnotherHeadquarters)},
		{name: "off the HQ ground", setup: hqSetup{offGround: true}, refuse: static(serverpackets.SystemMessageNotSetUpBaseHere)},
		{name: "off the HQ ground near a flag", setup: hqSetup{offGround: true}, near: true, refuse: static(serverpackets.SystemMessageNotSetUpBaseHere)},
		{name: "near a flag", near: true, refuse: static(serverpackets.SystemMessageHeadquartersTooClose)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootHQ(t, tt.setup)
			if tt.held && !w.otherFlag(t, "Siegers", 3000).StandFor(w.clan) {
				t.Fatal("the clan's far headquarters did not stand for it")
			}
			if tt.near {
				w.otherFlag(t, "Rivals", 200)
			}
			skillID := cmpOr(tt.skillID, buildHQSkillID)
			flagsBefore := len(w.flags())
			mpBefore := w.srv.PlayerCurrentMP(t, w.objID)
			w.c.Send(encodeRequestMagicSkillUse(skillID, false, false))
			frames := collectUntilQuiet(t, w.c)
			if !slices.Equal(opcodes(frames), []byte{serverpackets.OpcodeSystemMessage}) {
				t.Fatalf("refused flag cast answered %x, want one SystemMessage", opcodes(frames))
			}
			tt.refuse(t, frames[0])
			w.srv.Advance(t, 5*time.Second)
			drainUntilQuiet(t, w.c)
			if w.srv.PlayerCastingNow(t, w.objID) {
				t.Fatal("refused flag cast is in flight")
			}
			if got := w.srv.PlayerCurrentMP(t, w.objID); got != mpBefore {
				t.Fatalf("MP after refused cast = %d, want unchanged %d", got, mpBefore)
			}
			if got := w.costHeld(t); got != int(hqCostHeld) {
				t.Fatalf("cost items after refused cast = %d, want unchanged %d", got, hqCostHeld)
			}
			if got := len(w.flags()); got != flagsBefore {
				t.Fatalf("headquarters in the world = %d, want unchanged %d", got, flagsBefore)
			}
		})
	}
}

func cmpOr(v, fallback int32) int32 {
	if v != 0 {
		return v
	}
	return fallback
}

// TestSiegeFlagCastPutsUpHeadquarters lands each flag skill for an
// attacking clan leader on HQ ground, with another clan's headquarters
// farther than 400 away: a headquarters of the clan stands where the
// leader does, shown named after the clan with the reference body, with
// the reference HP and defences and no built-in bonus on them, no effect,
// heal or lethal blow landing on it, and the clan holds it. The cost is
// paid. A second flag cast is then refused NOT_ANOTHER_HEADQUARTERS.
func TestSiegeFlagCastPutsUpHeadquarters(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		skillID, other int32
		hp             int
	}{
		{skillID: buildHQSkillID, other: buildAdvHQSkillID, hp: 50000},
		{skillID: buildAdvHQSkillID, other: buildHQSkillID, hp: 100000},
	} {
		t.Run(fmt.Sprintf("skill %d", tt.skillID), func(t *testing.T) {
			t.Parallel()
			w := bootHQ(t, hqSetup{})
			far := w.otherFlag(t, "Rivals", 600)
			frames := w.putUp(t, tt.skillID)

			var flag *npc.Folk
			for _, f := range w.flags() {
				if f != far {
					flag = f
				}
			}
			if flag == nil {
				t.Fatal("no headquarters put up")
			}
			templateID, radius, height, name, title := npcInfoNameTitle(t, flagShown(t, frames, flag))
			if templateID != npc.SiegeFlagNpcID+1000000 || radius != 10 || height != 80 || name != "Siegers" || title != "" {
				t.Fatalf("headquarters shown as template %d body %v/%v name %q title %q, want %d 10/80 %q %q",
					templateID, radius, height, name, title, npc.SiegeFlagNpcID+1000000, "Siegers", "")
			}
			if x, y, _ := flag.Position(); x != w.at.X || y != w.at.Y {
				t.Fatalf("headquarters at %d,%d, want the leader's %d,%d", x, y, w.at.X, w.at.Y)
			}
			if flag.MaxHP() != tt.hp || flag.CurrentHP() != tt.hp || flag.PDef() != 500 || flag.MDef() != 500 || flag.MoveSpeed() != 0 {
				t.Fatalf("headquarters HP %d/%d P.Def %v M.Def %v speed %v, want %d/%d 500 500 0",
					flag.CurrentHP(), flag.MaxHP(), flag.PDef(), flag.MDef(), flag.MoveSpeed(), tt.hp, tt.hp)
			}
			if flag.CanBeHealed() || flag.Lethalable() || !flag.RefusesEffects() || !flag.Muted() {
				t.Fatal("headquarters can be healed, struck lethally, take effects or talk")
			}
			held, ok := w.clan.Headquarters()
			if cl, standsFor := flag.HeadquartersClan(); !ok || held.ObjectID() != flag.ObjectID() || !standsFor || cl != w.clan {
				t.Fatal("the clan does not hold the headquarters put up")
			}
			if got := w.costHeld(t); got != int(hqCostHeld)-hqCostCount {
				t.Fatalf("cost items after the cast = %d, want %d", got, int(hqCostHeld)-hqCostCount)
			}

			w.c.Send(encodeRequestMagicSkillUse(tt.other, false, false))
			refusal := collectUntilQuiet(t, w.c)
			if len(refusal) != 1 {
				t.Fatalf("second flag cast answered %x, want one SystemMessage", opcodes(refusal))
			}
			assertStaticSystemMessage(t, refusal[0], serverpackets.SystemMessageNotAnotherHeadquarters)
		})
	}
}

// TestSiegeFlagDiesFreeingItsClan kills a clan's headquarters: watchers see
// it die and leave the world at once, and the clan, holding none, puts up
// another.
func TestSiegeFlagDiesFreeingItsClan(t *testing.T) {
	t.Parallel()
	w := bootHQ(t, hqSetup{})
	w.putUp(t, buildHQSkillID)
	flags := w.flags()
	if len(flags) != 1 {
		t.Fatalf("headquarters in the world = %d, want 1", len(flags))
	}
	flag := flags[0]
	flag.TakeDamage(200000, nil)
	frames := collectUntilQuiet(t, w.c)
	die, gone := slices.Index(opcodes(frames), serverpackets.OpcodeDie), slices.Index(opcodes(frames), serverpackets.OpcodeDeleteObject)
	if die < 0 || gone < die {
		t.Fatalf("killed headquarters showed %x, want Die then DeleteObject", opcodes(frames))
	}
	if _, ok := w.srv.State.Object(flag.ObjectID()); ok {
		t.Fatal("dead headquarters still in the world")
	}
	if _, ok := w.clan.Headquarters(); ok {
		t.Fatal("the clan still holds its dead headquarters")
	}
	w.putUp(t, buildAdvHQSkillID)
	if held, ok := w.clan.Headquarters(); !ok || held.ObjectID() == flag.ObjectID() {
		t.Fatal("the clan put up no new headquarters")
	}
}

// TestSiegeFlagWarnsItsClanOfAnAttack hits a clan's headquarters: the
// clan's members are told BASE_UNDER_ATTACK on the first hit, not again on
// hits within 30 seconds of it, and again on the first hit after.
func TestSiegeFlagWarnsItsClanOfAnAttack(t *testing.T) {
	t.Parallel()
	w := bootHQ(t, hqSetup{})
	w.putUp(t, buildHQSkillID)
	flag := w.flags()[0]
	warned := func() int {
		n := 0
		for _, id := range drainSystemMessageIDs(t, w.c) {
			if id == serverpackets.SystemMessageBaseUnderAttack {
				n++
			}
		}
		return n
	}
	flag.TakeDamage(10, nil)
	if got := warned(); got != 1 {
		t.Fatalf("first hit warned %d times, want 1", got)
	}
	w.srv.Advance(t, 29*time.Second)
	flag.TakeDamage(10, nil)
	if got := warned(); got != 0 {
		t.Fatalf("hit 29s later warned %d times, want none", got)
	}
	w.srv.Advance(t, time.Second)
	flag.TakeDamage(10, nil)
	if got := warned(); got != 1 {
		t.Fatalf("hit 30s later warned %d times, want 1", got)
	}
	if got := flag.CurrentHP(); got != 50000-30 {
		t.Fatalf("headquarters HP = %d, want %d", got, 50000-30)
	}
}

// TestSiegeFlagsLeaveWithTheSiege pins Siege.midVictory and endSiege: when
// the castle changes hands mid-siege, the clans that owned and defended it
// lose their headquarters while the attackers keep theirs, the clan that
// took it included; when the siege ends every registered clan loses its
// headquarters, out of the world.
func TestSiegeFlagsLeaveWithTheSiege(t *testing.T) {
	t.Parallel()
	w := bootHQ(t, hqSetup{})
	w.putUp(t, buildHQSkillID)
	ours, _ := w.clan.Headquarters()
	theirs := w.otherFlag(t, "Rivals", 3000)
	if !theirs.StandFor(w.rivals) {
		t.Fatal("the rivals' headquarters did not stand for them")
	}
	if _, ok := w.srv.Castles.SetOwner(w.gludio.Castle(), w.clan); !ok {
		t.Fatal("Gludio not taken")
	}
	w.gludio.MidVictory()
	drainUntilQuiet(t, w.c)
	if _, ok := w.rivals.Headquarters(); ok {
		t.Fatal("the former defenders kept their headquarters at mid-victory")
	}
	if _, ok := w.srv.State.Object(theirs.ObjectID()); ok {
		t.Fatal("the former defenders' headquarters still in the world")
	}
	if held, ok := w.clan.Headquarters(); !ok || held.ObjectID() != ours.ObjectID() {
		t.Fatal("the clan that took the castle lost its headquarters at mid-victory")
	}

	w.gludio.End()
	frames := collectUntilQuiet(t, w.c)
	if _, ok := w.clan.Headquarters(); ok {
		t.Fatal("the clan kept its headquarters past the siege's end")
	}
	if _, ok := w.srv.State.Object(ours.ObjectID()); ok {
		t.Fatal("headquarters still in the world after the siege")
	}
	deleted := false
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeDeleteObject && wireReader(f[1:]).ReadInt32() == ours.ObjectID() {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("siege end showed %x, want the headquarters deleted", opcodes(frames))
	}
}
