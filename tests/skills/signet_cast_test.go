package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: PlayerAI.thinkCast (PlayerAI.java:299-303) sends FUSION and
// SIGNET_CASTTIME to PlayerCast.doFusionCast (PlayerCast.java:49-91) and every
// other skill to doCast. doFusionCast charges reuse, the mastery proc and the
// initial MP from the template's raw hit time, cool time and reuse; lands
// the skill (callSkill, or the fusion channel) before MagicSkillUse, USE_S1
// and a gauge sent whatever the hit time; and never destroys the skill's
// consume item, which PlayableCast.canCast (PlayableCast.java:88-96) still
// requires. Its hit timer at hitTime-400 (onMagicEffectHitTimer,
// PlayerCast.java:391-424) broadcasts MagicSkillLaunched, recharges shots and
// pays the final MP (NOT_ENOUGH_MP and a stop when short); its finalizer 400
// ms later (onMagicEffectFinalizer, PlayerCast.java:430-441) recharges again,
// raises the attack stance and ends the cast. There is no cool phase.

const (
	signetCastID       = 1419
	signetEffectNpcID  = 13018
	signetConsumeItem  = 20
	signetSpiritshotID = 2509
	signetSpiritVisual = 2047
	signetWeaponID     = 30
)

// signetCastSkill is a SIGNET_CASTTIME skill shaped like the shipped Volcano
// (1419): a SignetMDam self effect, a consume item and initial and final MP.
// Its hit time is not static and it carries a cool time, so a cast that
// scaled its timings or ran a cool phase would show it.
func signetCastSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: signetCastID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		Offensive: true, HitTime: 2000, CoolTime: 1000, ReuseDelay: 60_000, StaticReuse: true,
		SkillType: "SIGNET_CASTTIME", EffectNpcID: signetEffectNpcID, Radius: 180, Power: 1,
		MPInitialConsume: 2, MPConsume: 3, ItemConsumeID: signetConsumeItem, ItemConsumeCount: 1,
		SelfEffects: []modelskill.EffectTemplate{{Name: "SignetMDam", Self: true, Count: 9, Time: 2}},
	}
}

func signetPointTemplate() *npc.Template {
	return &npc.Template{ID: signetEffectNpcID, Type: "EffectPoint", CollisionRadius: 8}
}

// bootSignetCaster boots a Mage who knows def, holding held of the signet
// consume item when held is positive.
func bootSignetCaster(t *testing.T, def modelskill.Definition, held int32) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{signetPointTemplate()})),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	if !srv.DrivesClock() {
		t.Skip("exact cast phase timing needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	if held > 0 {
		srv.GiveItem(t, objID, signetConsumeItem, held)
	}
	return srv, c, objID
}

// signetPointSpawned reports whether a signet effect point is in the world.
func signetPointSpawned(srv *gameservertest.Server) bool {
	for _, obj := range srv.State.Objects() {
		if _, ok := obj.(*npc.EffectPoint); ok {
			return true
		}
	}
	return false
}

// readSignetCastStart reads a SIGNET_CASTTIME cast's frames up to its
// MagicSkillUse and returns it. The signet's effect lands on its caster
// before the cast is announced, so the AbnormalStatusUpdate of the effect it
// now carries comes first.
func readSignetCastStart(t *testing.T, c clientReader) []byte {
	t.Helper()
	sawEffect := false
	for range 20 {
		frame := c.Read()
		switch frame[0] {
		case serverpackets.OpcodeAbnormalStatusUpdate:
			sawEffect = true
		case serverpackets.OpcodeMagicSkillUse:
			if !sawEffect {
				t.Fatal("MagicSkillUse before the signet effect's AbnormalStatusUpdate; want the effect to land first")
			}
			return frame
		}
	}
	t.Fatal("signet cast MagicSkillUse never arrived")
	return nil
}

// readSignetCastStartFrames reads a SIGNET_CASTTIME cast's start through its
// launch: the effect's frames, then the cast start and launch as for any
// cast.
func readSignetCastStartFrames(t *testing.T, c clientReader, objID, skillID, level, hitTime, reuse, targetID int32) {
	t.Helper()
	readCastStartFramesFrom(t, c, readSignetCastStart(t, c), objID, skillID, level, hitTime, reuse, targetID)
}

// TestSignetCastRunsTheFusionCastTimeline casts a SIGNET_CASTTIME skill
// (#2839, #2801). Its effect point is in the world when the cast is
// announced; MagicSkillUse and the gauge carry the template hit time
// unscaled; the launch comes at hitTime-400 and pays the final MP right
// after MagicSkillLaunched; the cast ends 400 ms later, the template's cool
// time never run; and the consume item is only held, never taken or
// reported.
func TestSignetCastRunsTheFusionCastTimeline(t *testing.T) {
	t.Parallel()
	def := signetCastSkill()
	srv, c, objID := bootSignetCaster(t, def, 5)
	startInWorld(t, c)
	inv := srv.PlayerInventory(t, objID)

	start := c.Now()
	c.Send(encodeRequestMagicSkillUse(signetCastID, false, false))
	use := readSignetCastStart(t, c)
	if !signetPointSpawned(srv) {
		t.Fatal("no signet effect point in the world when the cast was announced; want it spawned at cast start")
	}
	mpAtStart := srv.PlayerCurrentMP(t, objID)
	readCastStartFramesFrom(t, c, use, objID, signetCastID, 1, int32(def.HitTime), int32(def.ReuseDelay), objID)
	if got, want := c.Now().Sub(start), time.Duration(def.HitTime-400)*time.Millisecond; got != want {
		t.Fatalf("MagicSkillLaunched %v after the request, want %v", got, want)
	}
	assertCasterMPStatus(t, srv, c.Read(), objID, mpAtStart-def.MPConsume)

	srv.Advance(t, 399*time.Millisecond)
	if !srv.PlayerCastingNow(t, objID) {
		t.Fatal("cast over before 400 ms past its launch")
	}
	srv.Advance(t, time.Millisecond)
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("cast still running 400 ms past its launch; want no cool phase")
	}

	log := readFrameLog(c)
	for _, id := range []int{serverpackets.SystemMessageS1Disappeared, serverpackets.SystemMessageS2S1Disappeared} {
		if at := log.index(isSystemMessage(id)); at >= 0 {
			t.Fatalf("system message %d at frame %d; the signet keeps its consume item", id, at)
		}
	}
	if got := inv.ItemCount(signetConsumeItem, -1, true); got != 5 {
		t.Fatalf("consume item count after the cast = %d, want 5 untouched", got)
	}
}

// TestSignetCastShortOfMPAtLaunchStops drains a SIGNET_CASTTIME caster's MP
// mid-cast: the launch still broadcasts MagicSkillLaunched, then reports
// NOT_ENOUGH_MP ahead of the cancel, and the stop ends the cast and takes
// the signet's effect point down with it (CreatureCast.stop exits the
// SIGNET_GROUND effect, CreatureCast.java:403-433).
func TestSignetCastShortOfMPAtLaunchStops(t *testing.T) {
	t.Parallel()
	def := signetCastSkill()
	srv, c, objID := bootSignetCaster(t, def, 5)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(signetCastID, false, false))
	readSignetCastStart(t, c)
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, signetCastID, 1)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSetupGauge, "SetupGauge")
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.ReduceMP(pc.MPValue()) })
	srv.AdvanceUntil(t, "cast stop", func() bool { return !srv.PlayerCastingNow(t, objID) })

	log := readFrameLog(c)
	launched := log.index(objectFrame(serverpackets.OpcodeMagicSkillLaunched, objID))
	short := log.index(isSystemMessage(serverpackets.SystemMessageNotEnoughMP))
	canceled := log.index(objectFrame(serverpackets.OpcodeMagicSkillCanceled, objID))
	if launched < 0 || short < launched || canceled < short {
		t.Fatalf("caster frames: MagicSkillLaunched at %d, NOT_ENOUGH_MP at %d, MagicSkillCanceled at %d; want them in that order",
			launched, short, canceled)
	}
	if signetPointSpawned(srv) {
		t.Fatal("signet effect point still in the world after the cast stopped")
	}
}

// TestFusionTimelineCastsKeepTheirConsumeItem casts a FUSION and a
// SIGNET_CASTTIME skill with a consume item (#2801): holding it, the cast
// runs to its end and the stack is unchanged with no item message; without
// it, the cast is refused with S1_CANNOT_BE_USED naming the skill.
func TestFusionTimelineCastsKeepTheirConsumeItem(t *testing.T) {
	t.Parallel()
	fusion := modelskill.Definition{
		ID: signetCastID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 1000, StaticHitTime: true, ReuseDelay: 1000, StaticReuse: true, SkillType: "FUSION",
		ItemConsumeID: signetConsumeItem, ItemConsumeCount: 1,
	}
	for _, tc := range []struct {
		name string
		def  modelskill.Definition
		held int32
	}{
		{name: "signet held", def: signetCastSkill(), held: 5},
		{name: "signet short", def: signetCastSkill()},
		{name: "fusion held", def: fusion, held: 5},
		{name: "fusion short", def: fusion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, c, objID := bootSignetCaster(t, tc.def, tc.held)
			startInWorld(t, c)

			c.Send(encodeRequestMagicSkillUse(signetCastID, false, false))
			if tc.held == 0 {
				assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1CannotBeUsed, signetCastID, 1)
				if extra := c.ReadWithTimeout(300 * time.Millisecond); extra != nil {
					t.Fatalf("refused cast sent extra opcode %#x, want the refusal alone", extra[0])
				}
				return
			}
			srv.AdvanceUntil(t, "cast end", func() bool { return !srv.PlayerCastingNow(t, objID) })
			log := readFrameLog(c)
			if log.index(func(frame []byte) bool { return frame[0] == serverpackets.OpcodeMagicSkillUse }) < 0 {
				t.Fatal("the cast never started")
			}
			for _, id := range []int{serverpackets.SystemMessageS1Disappeared, serverpackets.SystemMessageS2S1Disappeared} {
				if at := log.index(isSystemMessage(id)); at >= 0 {
					t.Fatalf("system message %d at frame %d; the cast keeps its consume item", id, at)
				}
			}
			if got := srv.PlayerInventory(t, objID).ItemCount(signetConsumeItem, -1, true); got != int(tc.held) {
				t.Fatalf("consume item count after the cast = %d, want %d untouched", got, tc.held)
			}
		})
	}
}

// TestSignetCastRechargesSpiritshotsAtItsLaunch casts a magic
// SIGNET_CASTTIME skill with auto-use spiritshots and an uncharged weapon
// (#2839): the launch at hitTime-400 recharges it, ENABLED_SPIRITSHOT then
// the charge MagicSkillUse, after MagicSkillLaunched and before the final
// MP is paid.
func TestSignetCastRechargesSpiritshotsAtItsLaunch(t *testing.T) {
	t.Parallel()
	def := signetCastSkill()
	def.Magic = true
	srv, c, objID := bootSignetCaster(t, def, 5)
	weapon := srv.GiveItem(t, objID, signetWeaponID, 1)
	srv.GiveItem(t, objID, signetSpiritshotID, 5)
	startInWorld(t, c)
	c.Send(encodeSignetAutoSoulShot(signetSpiritshotID, 1))
	drainUntilQuiet(t, c)
	c.Send(encodeSignetUseItem(weapon))
	drainUntilQuiet(t, c)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetChargedShot(item.ShotSpirit, false) })

	start := c.Now()
	c.Send(encodeRequestMagicSkillUse(signetCastID, false, false))
	use := readSignetCastStart(t, c)
	mpAtStart := srv.PlayerCurrentMP(t, objID)
	readCastStartFramesFrom(t, c, use, objID, signetCastID, 1, int32(def.HitTime), int32(def.ReuseDelay), objID)
	if got, want := c.Now().Sub(start), time.Duration(def.HitTime-400)*time.Millisecond; got != want {
		t.Fatalf("MagicSkillLaunched %v after the request, want %v", got, want)
	}

	enabled, charged := false, false
	for range 10 {
		frame := c.Read()
		if c.Now().Sub(start) != time.Duration(def.HitTime-400)*time.Millisecond {
			t.Fatalf("frame %#x left the launch at %v; want the recharge and MP inside it", frame[0], c.Now().Sub(start))
		}
		r := wire.NewReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeSystemMessage:
			if r.ReadInt32() == serverpackets.SystemMessageEnabledSpiritshot {
				enabled = true
			}
		case serverpackets.OpcodeMagicSkillUse:
			if caster, _, skill := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster == objID && skill == signetSpiritVisual {
				if !enabled {
					t.Fatal("charge MagicSkillUse before ENABLED_SPIRITSHOT")
				}
				charged = true
			}
		case serverpackets.OpcodeStatusUpdate:
			if _, ok := statusValue(frame, objID, serverpackets.StatusCurrentMP); !ok {
				continue
			}
			if !enabled || !charged {
				t.Fatalf("final MP paid before the recharge: ENABLED_SPIRITSHOT %v, charge MagicSkillUse %v", enabled, charged)
			}
			assertCasterMPStatus(t, srv, frame, objID, mpAtStart-def.MPConsume)
			drainUntilQuiet(t, c)
			return
		}
	}
	t.Fatal("the launch never paid its final MP")
}

// TestSignetMDamTickRefreshesSummonStatus ticks a SignetMDam over its
// caster's own servitor (#2838). Reference: EffectSignetMDam.onActionTime
// (EffectSignetMDam.java:89-107) broadcasts a summon target's status after
// the damage roll and before the damage branch, whether or not the tick
// hurts it; SummonStatus.broadcastStatusUpdate (SummonStatus.java:51-56)
// ends in updateAndBroadcastStatus, the owner's PetStatusUpdate. The owner
// reads it before the damage report for the servitor, and still reads it
// when the caster deals no damage.
func TestSignetMDamTickRefreshesSummonStatus(t *testing.T) {
	t.Parallel()
	const (
		servitorSkill = 1112
		servitorNpcID = 12601
	)
	for _, tc := range []struct {
		name   string
		denied bool
	}{
		{name: "damaging tick"},
		{name: "damage-denied tick", denied: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			signet := signetMDamSkill()
			signet.Power = 1
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Mage", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
					signetPointTemplate(),
					{
						ID: servitorNpcID, TemplateID: servitorNpcID, Type: "Servitor", Name: "Kat the Cat", Level: 20,
						HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
						CollisionRadius: 8, CollisionHeight: 20,
					},
				})),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{signet, {
					ID: servitorSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					SkillType: "SUMMON", NpcID: servitorNpcID, SummonTotalLifeTime: 1_200_000,
					StaticHitTime: true, StaticReuse: true,
				}})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, int(signet.ID), signet.Level)
			seedKnownSkill(t, srv, objID, servitorSkill, 1)
			startInWorld(t, c)

			c.Send(encodeRequestMagicSkillUse(servitorSkill, false, false))
			var servitorID int32
			srv.AdvanceUntil(t, "servitor in world state", func() bool {
				obj, ok := srv.State.Summon(objID)
				if ok {
					servitorID = obj.ObjectID()
				}
				return ok
			})
			drainUntilQuiet(t, c)
			setCasterMagicRolls(t, srv, objID, func() int { return 500 })
			// The signet also strikes its caster, standing in it; the owner
			// must outlive the ticks to read them.
			onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetInvul(true) })
			if tc.denied {
				denyCasterDamage(t, srv, objID)
			}

			c.Send(encodeRequestMagicSkillUse(int32(signet.ID), false, false))
			readSignetCastStartFrames(t, c, objID, int32(signet.ID), 1, int32(signet.HitTime), int32(signet.ReuseDelay), objID)
			drainUntilQuiet(t, c)
			tickSignetMDamLive(t, srv)

			log := readFrameLog(c)
			strike := func(frame []byte) bool {
				if frame[0] != serverpackets.OpcodeMagicSkillUse {
					return false
				}
				return wireReader(frame[1:]).ReadInt32() != objID
			}
			use := log.index(func(frame []byte) bool {
				if !strike(frame) {
					return false
				}
				r := wireReader(frame[1:])
				r.ReadInt32()
				return r.ReadInt32() == servitorID
			})
			if use < 0 {
				t.Fatal("owner never read the signet's MagicSkillUse on the servitor")
			}
			from := log[:use].lastIndex(strike) + 1
			segment := log[from:use]
			status := segment.index(func(frame []byte) bool {
				if frame[0] != serverpackets.OpcodePetStatusUpdate {
					return false
				}
				r := wireReader(frame[1:])
				r.ReadInt32() // summon type
				return r.ReadInt32() == servitorID
			})
			if status < 0 {
				t.Fatal("the tick over the servitor sent its owner no PetStatusUpdate ahead of the strike")
			}
			dealt := segment.index(isSystemMessage(serverpackets.SystemMessageYouDidS1Dmg))
			if tc.denied {
				if dealt >= 0 {
					t.Fatalf("damage-denied tick reported damage at frame %d", dealt)
				}
				return
			}
			if dealt < 0 || status > dealt {
				t.Fatalf("servitor tick frames: PetStatusUpdate at %d, YOU_DID_S1_DMG at %d; want the status first", status, dealt)
			}
		})
	}
}

// TestSignetMDamLackMPTellsCasterAndDespawnsPoint drains a SignetMDam
// caster's MP before the effect's first paying tick (#2856). Reference:
// EffectSignetMDam.onActionTime (EffectSignetMDam.java:73-81) sends the
// caster SKILL_REMOVED_DUE_LACK_MP when mpConsume exceeds its MP and ends
// the effect, whose onExit deletes the effect point. The two free ticks
// before it pay nothing and send no such message.
func TestSignetMDamLackMPTellsCasterAndDespawnsPoint(t *testing.T) {
	t.Parallel()
	def := signetMDamSkill()
	def.Power = 1
	srv, c, objID := bootSignetCaster(t, def, 0)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	readSignetCastStartFrames(t, c, objID, int32(def.ID), 1, int32(def.HitTime), int32(def.ReuseDelay), objID)
	var pointID int32
	for _, obj := range srv.State.Objects() {
		if point, ok := obj.(*npc.EffectPoint); ok {
			pointID = point.ObjectID()
		}
	}
	if pointID == 0 {
		t.Fatal("no signet effect point in the world after the cast started")
	}
	srv.Advance(t, 500*time.Millisecond)
	for range 2 {
		srv.Advance(t, 1100*time.Millisecond)
		srv.TickEffects()
	}
	if at := readFrameLog(c).index(isSystemMessage(serverpackets.SystemMessageSkillRemovedDueLackMP)); at >= 0 {
		t.Fatalf("SKILL_REMOVED_DUE_LACK_MP at frame %d during the signet's free ticks", at)
	}
	if !signetPointSpawned(srv) {
		t.Fatal("signet effect point gone before its first paying tick")
	}

	srv.Advance(t, 1100*time.Millisecond)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.ReduceMP(pc.MPValue()) })
	srv.TickEffects()

	log := readFrameLog(c)
	lack := log.index(isSystemMessage(serverpackets.SystemMessageSkillRemovedDueLackMP))
	if lack < 0 {
		t.Fatal("caster never read SKILL_REMOVED_DUE_LACK_MP when its signet tick could not pay its MP")
	}
	if deleted := log.index(objectFrame(serverpackets.OpcodeDeleteObject, pointID)); deleted >= 0 && deleted < lack {
		t.Fatalf("effect point DeleteObject at frame %d before SKILL_REMOVED_DUE_LACK_MP at %d; want the message first", deleted, lack)
	}
	if signetPointSpawned(srv) {
		t.Fatal("signet effect point still in the world after the tick short of MP")
	}
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		for _, e := range pc.EffectList().All() {
			if e.Skill.ID == def.ID {
				t.Errorf("caster still carries the signet's %s effect after the tick short of MP", e.Type)
			}
		}
	})
}

func encodeSignetAutoSoulShot(itemID, typ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestAutoSoulShot)
	w.WriteInt32(itemID)
	w.WriteInt32(typ)
	return w.Bytes()
}

func encodeSignetUseItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	return w.Bytes()
}
