package combat

import (
	"testing"

	"github.com/rs/zerolog"

	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// apprenticeKnifeID is the shipped Apprentice Adventurer's Knife (rhand):
// its <cond msgId="1685"><player pkCount="0"/></cond> lets only a
// PK-free player wear it.
const apprenticeKnifeID int32 = 7818

// shippedItemTemplate loads one item template from the shared datapack,
// skipping the calling test when no parent directory of the checkout holds
// aCis_datapack (it fails instead when ACIS_REQUIRE_DATAPACK is set).
func shippedItemTemplate(t *testing.T, id int32) *item.Template {
	t.Helper()
	dir := datapack.Path(t, "data", "xml", "items")
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
// The killer knows the lethal skill 42 and every skill the scene was booted
// with.
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
	// karmaTail is the opcodes of the frames the killer reads between the
	// karma change's StatusUpdate and S1_DISARMED: the karma UserInfo, and
	// the owner's own RelationChanged for its summon when it has one.
	karmaTail []byte
}

func bootPKKillScene(t *testing.T, known ...modelskill.Definition) *pkKillScene {
	t.Helper()
	return bootPKKillSceneWith(t, nil, known...)
}

// bootPKKillSceneWith is bootPKKillScene with extra boot options.
func bootPKKillSceneWith(t *testing.T, extra []gameservertest.Option, known ...modelskill.Definition) *pkKillScene {
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
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Killer", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, append(offensiveKillSkillDefs(), known...))),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithPvPFlags(flags),
	}, extra...)...)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 42, 1)
	for _, def := range known {
		seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	}
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
		karmaTail: []byte{serverpackets.OpcodeUserInfo},
		killer:    obj.(interface{ PvPFlagState() task.PvPFlagState }),
		karma:     obj.(interface{ Karma() int }).Karma,
		setRollSource: func(roll func(int) int) {
			runOnKiller(func() { obj.(interface{ SetRollSource(func(int) int) }).SetRollSource(roll) })
		},
	}
}

// assertKnifeTakenOff checks the killer's frames after its karma change for
// the knife's removal through the equip toggle: S1_DISARMED right after the
// rest of the karma change (karmaTail), so no frame of the killing blow
// slips in ahead of it, then the refresh and ActionFailed for the aborted
// attack. It returns the frames and the ActionFailed's index.
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
	if disarmed != len(s.karmaTail) || string(opcodesOf(frames[:disarmed])) != string(s.karmaTail) {
		t.Fatalf("frames after the karma change = %x, want %x then S1_DISARMED first", opcodesOf(frames), s.karmaTail)
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
//
// All of it happens inside the killing hit's damage (CreatureAttack.doHit
// → reduceCurrentHp → onKillUpdatePvPKarma), ahead of the rest of the hit:
// the wounded killer absorbs part of the damage, and the StatusUpdate that
// reports it follows the flag reset.
func TestPKKillUnequipsPKFreeWeaponAndEndsTheFlag(t *testing.T) {
	t.Parallel()
	s := bootPKKillScene(t)
	// Every swing hits, so the scenario never waits on a lucky roll.
	s.setRollSource(func(int) int { return 0 })
	onPlayerQueue(t, s.srv, s.objID, func(pc *player.Character) {
		pc.AddStatFuncs([]effect.Mod{{Stat: stat.AbsorbDamagePercent, Op: effect.OpAdd, Value: 50}})
		pc.SetHP(1)
	})
	drainUntilQuiet(t, s.c)

	selectPlayerTarget(t, s.c, s.victimID)
	s.c.Send(encodeAttackRequest(s.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertKarmaChangeFrames(t, s.c, s.objID, 240)
	s.srv.Settle(t)

	frames, aborted := s.assertKnifeTakenOff(t)
	flagReset := indexOf(frames, aborted+1, serverpackets.OpcodeUserInfo, -1)
	if flagReset < 0 {
		t.Fatalf("frames after the karma change = %x, want the flag reset's UserInfo after ActionFailed", opcodesOf(frames))
	}
	absorbed := indexOfSelfHPAbove(frames, s.objID, 1)
	if absorbed < flagReset {
		t.Fatalf("frames after the karma change = %x, want S1_DISARMED and the flag reset's UserInfo before the absorbed HP's StatusUpdate", opcodesOf(frames))
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
// the knife off and resets the flag as for a physical kill, inside the
// killing blow: both reach the killer before the skill's YOU_DID_S1_DMG
// (Pdam sends it after reduceCurrentHp). An offensive skill flags its
// caster once its effects have run, so the killer is flagged again after
// the damage message and ends with both karma and the flag.
func TestPKSkillKillEndsWithTheKillerFlagged(t *testing.T) {
	t.Parallel()
	s := bootPKKillScene(t)

	selectPlayerTarget(t, s.c, s.victimID)
	castKillSkill(t, s.srv, s.c, s.objID, s.victimID, true)
	assertKarmaChangeFrames(t, s.c, s.objID, 240)
	s.srv.Settle(t)

	frames, aborted := s.assertKnifeTakenOff(t)
	flagReset := indexOf(frames, aborted+1, serverpackets.OpcodeUserInfo, -1)
	damage := indexOfSystemMessage(frames, 0, serverpackets.SystemMessageYouDidS1Dmg)
	if flagReset < 0 || damage < flagReset {
		t.Fatalf("frames after the karma change = %x, want S1_DISARMED and the flag reset's UserInfo before YOU_DID_S1_DMG", opcodesOf(frames))
	}
	reflag := indexOf(frames, damage+1, serverpackets.OpcodeUserInfo, -1)
	if reflag < 0 {
		t.Fatalf("frames after the karma change = %x, want the re-flag's UserInfo after YOU_DID_S1_DMG", opcodesOf(frames))
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

// TestPKProcKillTakesTheKnifeOffBeforeTheProcsDamageMessage has the same
// flagged killer's physical hit set off its passive ON_HIT chance skill,
// which triggers the lethal skill 42 on the innocent victim; the victim's
// raised max HP outlasts the hit itself, so the proc makes the kill. As for
// a physical or a skill kill, the karma gain takes the knife off and resets
// the flag inside the killing blow: both reach the killer before the proc's
// YOU_DID_S1_DMG, which the triggered skill's handler sends after its
// damage.
func TestPKProcKillTakesTheKnifeOffBeforeTheProcsDamageMessage(t *testing.T) {
	t.Parallel()
	s := bootPKKillScene(t, modelskill.Definition{
		ID: 43, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", ChanceType: "ON_HIT", ActivationChance: -1,
		TriggeredID: 42, TriggeredLevel: 1,
	})
	// Every swing hits, so the scenario never waits on a lucky roll.
	s.setRollSource(func(int) int { return 0 })
	onPlayerQueue(t, s.srv, s.victimID, func(pc *player.Character) {
		pc.AddStatFuncs([]effect.Mod{{Stat: stat.MaxHP, Op: effect.OpAdd, Value: 10_000}})
		pc.SetHP(pc.MaxHPValue())
	})
	drainUntilQuiet(t, s.vc)
	drainUntilQuiet(t, s.c)

	selectPlayerTarget(t, s.c, s.victimID)
	s.c.Send(encodeAttackRequest(s.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertKarmaChangeFrames(t, s.c, s.objID, 240)
	s.srv.Settle(t)

	frames, aborted := s.assertKnifeTakenOff(t)
	flagReset := indexOf(frames, aborted+1, serverpackets.OpcodeUserInfo, -1)
	damage := indexOfSystemMessage(frames, 0, serverpackets.SystemMessageYouDidS1Dmg)
	if flagReset < 0 || damage < flagReset {
		t.Fatalf("frames after the karma change = %x, want S1_DISARMED and the flag reset's UserInfo before the proc's YOU_DID_S1_DMG", opcodesOf(frames))
	}
	if karma := s.karma(); karma != 240 {
		t.Fatalf("killer karma = %d after the proc PK kill, want 240", karma)
	}
}

// indexOfSelfHPAbove finds the first StatusUpdate for objID that reports a
// CUR_HP above hp, or -1.
func indexOfSelfHPAbove(frames [][]byte, objID, hp int32) int {
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeStatusUpdate {
			continue
		}
		r := wireReader(f[1:])
		if r.ReadInt32() != objID {
			continue
		}
		for n := r.ReadInt32(); n > 0; n-- {
			if attr, value := r.ReadInt32(), r.ReadInt32(); attr == int32(serverpackets.StatusCurrentHP) && value > hp {
				return i
			}
		}
	}
	return -1
}

// opcodesOf lists each frame's opcode, for failure messages.
func opcodesOf(frames [][]byte) []byte {
	out := make([]byte, len(frames))
	for i, f := range frames {
		out[i] = f[0]
	}
	return out
}
