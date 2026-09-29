package combat

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rs/zerolog"

	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// apprenticeKnifeID is the shipped Apprentice Adventurer's Knife (rhand):
// its <cond msgId="1685"><player pkCount="0"/></cond> lets only a
// PK-free player wear it.
const apprenticeKnifeID int32 = 7818

// shippedItemTemplate loads one item template from the shared datapack,
// skipping the calling test when the datapack is not checked out next to
// the module.
func shippedItemTemplate(t *testing.T, id int32) *item.Template {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "aCis_datapack", "data", "xml", "items")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("aCis_datapack not checked out near the module root")
	}
	table, err := xmldata.LoadItemTemplates(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}
	tmpl, ok := table.Get(id)
	if !ok {
		t.Fatalf("shipped item %d missing", id)
	}
	return tmpl
}

// pkKillScene is a PvP-flagged killer wearing the shipped PK-free knife
// next to an innocent level 1 victim, both in world with quiet clients.
type pkKillScene struct {
	srv             *gameservertest.Server
	c, vc           *scriptedClient
	objID, victimID int32
	knifeObjID      int32
	inv             *itemcontainer.Inventory
	flags           *task.PvPFlags
	killer          interface{ PvPFlagState() task.PvPFlagState }
	karma           func() int
	setRollSource   func(func(int) int)
}

func bootPKKillScene(t *testing.T) *pkKillScene {
	t.Helper()
	knife := shippedItemTemplate(t, apprenticeKnifeID)
	if len(knife.UseConditions) != 1 || knife.UseConditions[0].MessageID != 1685 {
		t.Fatalf("shipped knife conditions = %+v, want the one pkCount clause with message 1685", knife.UseConditions)
	}
	// The fixture character's weight limit is tiny; a weightless copy keeps
	// the knife from adding a weight-penalty update to EnterWorld.
	light := *knife
	light.Weight = 0
	templates := append(gameservertest.ItemTemplates().All(), &light)
	flags := task.NewPvPFlags(task.DefaultPvPFlagOptions(), nil)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Killer", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, offensiveKillSkillDefs())),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithPvPFlags(flags),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 42, 1)
	knifeObjID := srv.GiveItem(t, objID, apprenticeKnifeID, 1)
	victim := srv.SeedCharacterFor(t, "victim", "Victim", 1, 0)
	vc := srv.DialClient(t, "victim", 1)
	startInWorld(t, vc)
	startInWorld(t, c)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("killer missing from world state")
	}
	inv := obj.(interface {
		Inventory() *itemcontainer.Inventory
	}).Inventory()
	c.Send(encodeUseItem(knifeObjID, false))
	srv.AdvanceUntil(t, "knife equipped", func() bool { return inv.ItemByObjectID(knifeObjID).Equipped() })
	killer := obj.(task.PvPFlagActor)
	runOnKiller := func(fn func()) {
		done := make(chan struct{})
		if !srv.PlayerQueue(t, objID).Post(func() { defer close(done); fn() }) {
			t.Fatal("killer queue closed")
		}
		<-done
	}
	runOnKiller(func() { flags.AddNormal(killer) })
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	return &pkKillScene{
		srv: srv, c: c, vc: vc, objID: objID, victimID: victim.ID,
		knifeObjID: knifeObjID, inv: inv, flags: flags,
		killer: obj.(interface{ PvPFlagState() task.PvPFlagState }),
		karma:  obj.(interface{ Karma() int }).Karma,
		setRollSource: func(roll func(int) int) {
			runOnKiller(func() { obj.(interface{ SetRollSource(func(int) int) }).SetRollSource(roll) })
		},
	}
}

// assertKnifeTakenOff checks the killer's frames after its karma change for
// the knife's removal through the equip toggle: S1_DISARMED, the refresh,
// then ActionFailed for the aborted attack. It returns the frames and the
// ActionFailed's index.
func (s *pkKillScene) assertKnifeTakenOff(t *testing.T) ([][]byte, int) {
	t.Helper()
	frames := readQuiet(s.c)
	disarmed := -1
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(f[1:])
		if r.ReadInt32() != serverpackets.SystemMessageS1Disarmed {
			continue
		}
		if params, typ, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); params != 1 || typ != serverpackets.SystemMessageParamItemName || id != apprenticeKnifeID {
			t.Fatalf("S1_DISARMED params = %d type %d item %d, want the knife", params, typ, id)
		}
		disarmed = i
		break
	}
	if disarmed < 0 {
		t.Fatalf("no S1_DISARMED for the knife among %x", opcodesOf(frames))
	}
	refresh := indexOf(frames, disarmed, serverpackets.OpcodeUserInfo, -1)
	aborted := indexOf(frames, disarmed, serverpackets.OpcodeActionFailed, -1)
	if refresh < 0 || aborted < refresh {
		t.Fatalf("frames after the karma change = %x, want S1_DISARMED, UserInfo, then ActionFailed", opcodesOf(frames))
	}
	if s.inv.ItemByObjectID(s.knifeObjID).Equipped() {
		t.Fatal("knife still equipped after the PK kill")
	}
	return frames, aborted
}

// lastVictimRelation returns the relation, karma and PvP flag of the last
// RelationChanged the victim received for the killer.
func (s *pkKillScene) lastVictimRelation(t *testing.T) (relation, karma, pvpFlag int32) {
	t.Helper()
	var last []byte
	for _, f := range readQuiet(s.vc) {
		if f[0] == serverpackets.OpcodeRelationChanged && wireReader(f[1:]).ReadInt32() == s.objID {
			last = f
		}
	}
	if last == nil {
		t.Fatal("victim never received the killer's RelationChanged")
	}
	r := wireReader(last[1:])
	r.ReadInt32()
	relation, _, karma = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	return relation, karma, r.ReadInt32()
}

// TestPKKillUnequipsPKFreeWeaponAndEndsTheFlag has a PvP-flagged player,
// wearing the shipped PK-free knife, kill an innocent player with a
// physical attack. The hit flags the attacker before its damage lands; the
// kill then makes it a PKer: after the karma announcement the knife comes
// off through the equip toggle (S1_DISARMED, the refresh, and ActionFailed
// for the aborted attack), then the PvP flag task stops and the flag
// resets, which the victim sees as a RelationChanged without the flag.
func TestPKKillUnequipsPKFreeWeaponAndEndsTheFlag(t *testing.T) {
	t.Parallel()
	s := bootPKKillScene(t)
	// Every swing hits, so the scenario never waits on a lucky roll.
	s.setRollSource(func(int) int { return 0 })

	selectPlayerTarget(t, s.c, s.victimID)
	s.c.Send(encodeAttackRequest(s.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertKarmaChangeFrames(t, s.c, s.objID, 240)
	s.srv.Settle(t)

	frames, aborted := s.assertKnifeTakenOff(t)
	if indexOf(frames, aborted+1, serverpackets.OpcodeUserInfo, -1) < 0 {
		t.Fatalf("frames after the karma change = %x, want the flag reset's UserInfo after ActionFailed", opcodesOf(frames))
	}
	if state := s.killer.PvPFlagState(); state != task.PvPFlagNone {
		t.Fatalf("killer PvP flag = %v after the physical PK kill, want none", state)
	}
	if n := s.flags.Len(); n != 0 {
		t.Fatalf("PvP flag task tracks %d players after the physical PK kill, want none", n)
	}
	if relation, karma, pvpFlag := s.lastVictimRelation(t); relation&serverpackets.RelationPvPFlag != 0 || pvpFlag != 0 || karma != 240 {
		t.Fatalf("victim's last RelationChanged for the killer = relation %#x karma %d flag %d, want karma 240 and no flag", relation, karma, pvpFlag)
	}
}

// TestPKSkillKillEndsWithTheKillerFlagged has the same flagged killer kill
// the innocent player with an offensive skill. The kill's karma gain takes
// the knife off and resets the flag as for a physical kill, but an
// offensive skill flags its caster once its effects have run, so the killer
// is flagged again after the reset and ends with both karma and the flag.
func TestPKSkillKillEndsWithTheKillerFlagged(t *testing.T) {
	t.Parallel()
	s := bootPKKillScene(t)

	selectPlayerTarget(t, s.c, s.victimID)
	castKillSkill(t, s.srv, s.c, s.objID, s.victimID, true)
	assertKarmaChangeFrames(t, s.c, s.objID, 240)
	s.srv.Settle(t)

	frames, aborted := s.assertKnifeTakenOff(t)
	flagReset := indexOf(frames, aborted+1, serverpackets.OpcodeUserInfo, -1)
	reflag := indexOf(frames, flagReset+1, serverpackets.OpcodeUserInfo, -1)
	if flagReset < 0 || reflag < 0 {
		t.Fatalf("frames after the karma change = %x, want the flag reset's UserInfo, then the re-flag's", opcodesOf(frames))
	}
	if karma := s.karma(); karma != 240 {
		t.Fatalf("killer karma = %d after the skill PK kill, want 240", karma)
	}
	if state := s.killer.PvPFlagState(); state == task.PvPFlagNone {
		t.Fatal("killer unflagged after the skill PK kill, want the skill's flag")
	}
	if n := s.flags.Len(); n != 1 {
		t.Fatalf("PvP flag task tracks %d players after the skill PK kill, want the killer", n)
	}
	if relation, karma, pvpFlag := s.lastVictimRelation(t); relation&serverpackets.RelationPvPFlag == 0 || pvpFlag == 0 || karma != 240 {
		t.Fatalf("victim's last RelationChanged for the killer = relation %#x karma %d flag %d, want karma 240 and the flag", relation, karma, pvpFlag)
	}
}

// opcodesOf lists each frame's opcode, for failure messages.
func opcodesOf(frames [][]byte) []byte {
	out := make([]byte, len(frames))
	for i, f := range frames {
		out[i] = f[0]
	}
	return out
}
