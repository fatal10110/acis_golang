package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	// feedbackStrikeSkill is the attacker's single-target skill on the victim.
	feedbackStrikeSkill = 44
	// feedbackChantSkill is the victim's own ten-second magic self cast.
	feedbackChantSkill = 9104
)

// frameLog is every frame one client read, in arrival order.
type frameLog [][]byte

// readFrameLog reads c until the server stays quiet.
func readFrameLog(c *testsupport.ScriptedClient) frameLog {
	var log frameLog
	for range 200 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			break
		}
		log = append(log, frame)
	}
	return log
}

// index returns the position of the first frame match accepts, or -1.
func (l frameLog) index(match func(frame []byte) bool) int {
	for i, frame := range l {
		if match(frame) {
			return i
		}
	}
	return -1
}

// systemMessage matches a SystemMessage frame with id and returns its
// parameter reader.
func systemMessage(frame []byte, id int) (*wire.Reader, bool) {
	if frame[0] != serverpackets.OpcodeSystemMessage {
		return nil, false
	}
	r := wireReader(frame[1:])
	return r, r.ReadInt32() == int32(id)
}

// isSystemMessage reports a SystemMessage frame with id.
func isSystemMessage(id int) func([]byte) bool {
	return func(frame []byte) bool {
		_, ok := systemMessage(frame, id)
		return ok
	}
}

// statusValue returns the value frame's StatusUpdate for objID carries for
// attr, or false when frame is not one or carries no such attribute.
func statusValue(frame []byte, objID int32, attr serverpackets.StatusType) (int32, bool) {
	if frame[0] != serverpackets.OpcodeStatusUpdate {
		return 0, false
	}
	r := wireReader(frame[1:])
	if r.ReadInt32() != objID {
		return 0, false
	}
	for range r.ReadInt32() {
		typ, value := r.ReadInt32(), r.ReadInt32()
		if typ == int32(attr) {
			return value, true
		}
	}
	return 0, false
}

// assertGaveYouDamage asserts frame is S1_GAVE_YOU_S2_DMG naming attacker
// with amount.
func assertGaveYouDamage(t *testing.T, frame []byte, attacker string, amount int32) {
	t.Helper()
	r, ok := systemMessage(frame, serverpackets.SystemMessageS1GaveYouS2Dmg)
	if !ok {
		t.Fatalf("frame %#x is not S1_GAVE_YOU_S2_DMG", frame[0])
	}
	if params := r.ReadInt32(); params != 2 {
		t.Fatalf("S1_GAVE_YOU_S2_DMG params = %d, want 2", params)
	}
	if typ, name := r.ReadInt32(), r.ReadString(); typ != serverpackets.SystemMessageParamText || name != attacker {
		t.Fatalf("S1_GAVE_YOU_S2_DMG first parameter = type %d %q, want text %q", typ, name, attacker)
	}
	if typ, got := r.ReadInt32(), r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber || got != amount {
		t.Fatalf("S1_GAVE_YOU_S2_DMG second parameter = type %d %d, want number %d", typ, got, amount)
	}
}

// youDidDamage returns the amount of the first YOU_DID_S1_DMG in log.
func youDidDamage(t *testing.T, log frameLog) int32 {
	t.Helper()
	at := log.index(isSystemMessage(serverpackets.SystemMessageYouDidS1Dmg))
	if at < 0 {
		t.Fatal("attacker never read YOU_DID_S1_DMG")
	}
	r, _ := systemMessage(log[at], serverpackets.SystemMessageYouDidS1Dmg)
	r.ReadInt32() // params
	r.ReadInt32() // number type
	return r.ReadInt32()
}

// pvpPair is a Mage and a Victim in the world together, the Mage holding a
// single-target skill aimed at the Victim.
type pvpPair struct {
	srv                *gameservertest.Server
	c, vc              *testsupport.ScriptedClient
	mageID, victimID   int32
	strikeHit, strikeR int32
}

// bootPVPPair boots the pair with strike as the Mage's skill and the
// Victim knowing the ten-second magic chant, and has the Mage target the
// Victim with magic rolls that never fail or crit.
func bootPVPPair(t *testing.T, strike modelskill.Definition, victimLevel int) *pvpPair {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			strike,
			{
				ID: feedbackChantSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				Magic: true, HitTime: 10_000, StaticHitTime: true, SkillType: "DUMMY",
			},
		})),
	)
	c, mageID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, mageID, int(strike.ID), strike.Level)
	victim := srv.SeedCharacterFor(t, "victim", "Victim", victimLevel, 0)
	seedKnownSkill(t, srv, victim.ID, feedbackChantSkill, 1)
	vc := srv.DialClient(t, "victim", 1)
	startInWorldAmongPlayers(t, vc)
	startInWorldAmongPlayers(t, c)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)

	c.Send(encodeAction(victim.ID, hostileX, hostileY, hostileZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeValidateLocation, "click ValidateLocation")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMyTargetSelected, "MyTargetSelected")
	drainUntilQuiet(t, c)
	setCasterMagicRolls(t, srv, mageID, func() int { return 500 })
	return &pvpPair{srv: srv, c: c, vc: vc, mageID: mageID, victimID: victim.ID, strikeHit: int32(strike.HitTime), strikeR: int32(strike.ReuseDelay)}
}

// strike casts the Mage's skill on the Victim, forced since the Victim is an
// unflagged innocent, and advances until done reports the hit landed.
func (p *pvpPair) strike(t *testing.T, id, level int32, done func() bool) {
	t.Helper()
	p.c.Send(encodeRequestMagicSkillUse(id, true, false))
	readCastStartFrames(t, p.c, p.mageID, id, level, p.strikeHit, p.strikeR, p.victimID)
	p.srv.AdvanceUntil(t, "the Mage's skill landing on the Victim", done)
}

func mdamStrike(power float32) modelskill.Definition {
	return modelskill.Definition{
		ID: feedbackStrikeSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "MDAM", Power: power, Magic: true,
	}
}

// TestSkillHitBreaksPlayerCastBeforeItsStatus drives an MDAM into a player
// mid-way through a magic cast under a breaking roll. The skill hit rolls
// the cast break before it touches the target, so the target reads
// MagicSkillCanceled and CASTING_INTERRUPTED first; then the damage report
// naming the attacker with the full damage, and only then the StatusUpdate
// with the HP the hit left (issues #2697, #2588).
func TestSkillHitBreaksPlayerCastBeforeItsStatus(t *testing.T) {
	t.Parallel()
	p := bootPVPPair(t, mdamStrike(30), 60)
	onPlayerQueue(t, p.srv, p.victimID, func(pc *player.Character) {
		pc.SetRollSource(func(int) int { return 0 })
		pc.SetCP(0)
	})
	drainUntilQuiet(t, p.vc)
	p.vc.Send(encodeRequestMagicSkillUse(feedbackChantSkill, false, false))
	readUntil(t, p.vc, serverpackets.OpcodeSetupGauge)
	drainUntilQuiet(t, p.vc)
	drainUntilQuiet(t, p.c)

	before := p.srv.PlayerCurrentHP(t, p.victimID)
	p.strike(t, feedbackStrikeSkill, 1, func() bool { return p.srv.PlayerCurrentHP(t, p.victimID) < before })
	after := int32(p.srv.PlayerCurrentHP(t, p.victimID))
	if after <= 0 {
		t.Fatalf("the Victim died from %d HP; the hit must leave it alive", before)
	}

	dealt := youDidDamage(t, readFrameLog(p.c))
	log := readFrameLog(p.vc)
	canceled := log.index(func(frame []byte) bool {
		return frame[0] == serverpackets.OpcodeMagicSkillCanceled && wireReader(frame[1:]).ReadInt32() == p.victimID
	})
	interrupted := log.index(isSystemMessage(serverpackets.SystemMessageCastingInterrupted))
	gave := log.index(isSystemMessage(serverpackets.SystemMessageS1GaveYouS2Dmg))
	status := log.index(func(frame []byte) bool {
		hp, ok := statusValue(frame, p.victimID, serverpackets.StatusCurrentHP)
		return ok && hp == after
	})
	if canceled < 0 || interrupted < 0 || gave < 0 || status < 0 {
		t.Fatalf("Victim frames: MagicSkillCanceled at %d, CASTING_INTERRUPTED at %d, S1_GAVE_YOU_S2_DMG at %d, StatusUpdate HP %d at %d; want all present",
			canceled, interrupted, gave, after, status)
	}
	if !(canceled < interrupted && interrupted < gave && gave < status) {
		t.Fatalf("Victim frame order: MagicSkillCanceled %d, CASTING_INTERRUPTED %d, S1_GAVE_YOU_S2_DMG %d, StatusUpdate %d; want that order",
			canceled, interrupted, gave, status)
	}
	assertGaveYouDamage(t, log[gave], "Mage", dealt)
}

// TestPlayerHitReportsDamageAroundItsStatusWrites pins where the damaged
// player's S1_GAVE_YOU_S2_DMG falls against its StatusUpdate. The report
// sits between the CP write and the HP write, and only the write that ends
// the hit reports the status: a hit that gets past CP reports the damage
// first, and a hit CP absorbs whole reports the status first. Either way
// the report carries the full damage (issue #2588).
func TestPlayerHitReportsDamageAroundItsStatusWrites(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name        string
		power       float32
		victimLevel int
		cp          bool
		statusFirst bool
	}{
		{name: "hit past CP reports damage first", power: 30, victimLevel: 5},
		{name: "hit CP absorbs reports status first", power: 1, victimLevel: 60, cp: true, statusFirst: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := bootPVPPair(t, mdamStrike(tt.power), tt.victimLevel)
			onPlayerQueue(t, p.srv, p.victimID, func(pc *player.Character) {
				pc.SetCP(0)
				if tt.cp {
					pc.SetCP(pc.MaxCPValue())
				}
			})
			drainUntilQuiet(t, p.vc)

			hp, cp := p.srv.PlayerCurrentHP(t, p.victimID), p.srv.PlayerCurrentCP(t, p.victimID)
			p.strike(t, feedbackStrikeSkill, 1, func() bool {
				return p.srv.PlayerCurrentHP(t, p.victimID) < hp || p.srv.PlayerCurrentCP(t, p.victimID) < cp
			})
			if tt.cp && p.srv.PlayerCurrentHP(t, p.victimID) != hp {
				t.Fatalf("Victim HP = %d after the hit, want CP to absorb it whole", p.srv.PlayerCurrentHP(t, p.victimID))
			}

			dealt := youDidDamage(t, readFrameLog(p.c))
			log := readFrameLog(p.vc)
			gave := log.index(isSystemMessage(serverpackets.SystemMessageS1GaveYouS2Dmg))
			status := log.index(func(frame []byte) bool { return frame[0] == serverpackets.OpcodeStatusUpdate })
			if gave < 0 || status < 0 {
				t.Fatalf("Victim frames: S1_GAVE_YOU_S2_DMG at %d, StatusUpdate at %d; want both", gave, status)
			}
			if got := status < gave; got != tt.statusFirst {
				t.Fatalf("StatusUpdate before S1_GAVE_YOU_S2_DMG = %v (status %d, message %d), want %v", got, status, gave, tt.statusFirst)
			}
			assertGaveYouDamage(t, log[gave], "Mage", dealt)
		})
	}
}

// TestNPCAutoAttackReportsDamageToThePlayer lands a monster's melee hit on a
// player: the player reads S1_GAVE_YOU_S2_DMG naming the monster with the
// HP it lost, right before the StatusUpdate reporting that HP (issue #2588).
func TestNPCAutoAttackReportsDamageToThePlayer(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	var victim attackable.Combatant
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { victim = pc })

	before := srv.PlayerCurrentHP(t, objID)
	attacker.DoAttack(t, victim)
	after := srv.PlayerCurrentHP(t, objID)
	if after >= before {
		t.Fatalf("player HP = %d after the monster's hit, want below %d", after, before)
	}

	log := readFrameLog(c)
	gave := log.index(isSystemMessage(serverpackets.SystemMessageS1GaveYouS2Dmg))
	if gave < 0 || gave+1 >= len(log) {
		t.Fatalf("S1_GAVE_YOU_S2_DMG at %d of %d frames, want one followed by the status", gave, len(log))
	}
	assertGaveYouDamage(t, log[gave], attacker.CharacterName(), int32(before-after))
	if hp, ok := statusValue(log[gave+1], objID, serverpackets.StatusCurrentHP); !ok || hp != int32(after) {
		t.Fatalf("frame after S1_GAVE_YOU_S2_DMG = %#x (HP %d, ok %v), want the StatusUpdate with HP %d", log[gave+1][0], hp, ok, after)
	}
}

// TestSignetMDamTickReportsDamageToBothSides drives a SignetMDam tick into
// another player: the caster reads YOU_DID_S1_DMG before the signet's
// MagicSkillUse on that target, and the target reads S1_GAVE_YOU_S2_DMG
// naming the caster with the same damage, right before its StatusUpdate
// (issues #2587, #2588).
func TestSignetMDamTickReportsDamageToBothSides(t *testing.T) {
	t.Parallel()
	srv, c, vc, objID, victimID := bootSignetMDamPair(t)
	setCasterMagicRolls(t, srv, objID, func() int { return 500 })

	c.Send(encodeRequestMagicSkillUse(1419, false, false))
	readCastStartFrames(t, c, objID, 1419, 1, 500, 60_000, objID)
	tickSignetMDamLive(t, srv)

	log := readFrameLog(c)
	dealt := log.index(isSystemMessage(serverpackets.SystemMessageYouDidS1Dmg))
	signetUse := log.index(func(frame []byte) bool {
		if frame[0] != serverpackets.OpcodeMagicSkillUse {
			return false
		}
		r := wireReader(frame[1:])
		caster, target := r.ReadInt32(), r.ReadInt32()
		return caster != objID && target == victimID
	})
	if dealt < 0 || signetUse < 0 || dealt > signetUse {
		t.Fatalf("caster frames: YOU_DID_S1_DMG at %d, signet MagicSkillUse on the victim at %d; want the damage report first", dealt, signetUse)
	}
	amount := youDidDamage(t, log)

	vlog := readFrameLog(vc)
	gave := vlog.index(isSystemMessage(serverpackets.SystemMessageS1GaveYouS2Dmg))
	if gave < 0 || gave+1 >= len(vlog) {
		t.Fatalf("S1_GAVE_YOU_S2_DMG at %d of %d victim frames, want one followed by the status", gave, len(vlog))
	}
	assertGaveYouDamage(t, vlog[gave], "Mage", amount)
	if _, ok := statusValue(vlog[gave+1], victimID, serverpackets.StatusCurrentHP); !ok {
		t.Fatalf("victim frame after S1_GAVE_YOU_S2_DMG = %#x, want its StatusUpdate", vlog[gave+1][0])
	}
}

// TestCPDamagePercentReportsDamageToBothSides casts CPDAMPERCENT at another
// player: the caster reads YOU_DID_S1_DMG with the CP taken, and the target
// reads its StatusUpdate with the lowered CP, then S1_GAVE_YOU_S2_DMG naming
// the caster with the same amount (issue #2587).
func TestCPDamagePercentReportsDamageToBothSides(t *testing.T) {
	t.Parallel()
	p := bootPVPPair(t, modelskill.Definition{
		ID: feedbackStrikeSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "CPDAMPERCENT", Power: 50,
	}, 5)
	onPlayerQueue(t, p.srv, p.victimID, func(pc *player.Character) { pc.SetCP(pc.MaxCPValue()) })
	drainUntilQuiet(t, p.vc)
	cp := p.srv.PlayerCurrentCP(t, p.victimID)
	want := int32(cp / 2)
	if want <= 0 {
		t.Fatalf("Victim CP = %d, want enough for a visible half", cp)
	}

	p.strike(t, feedbackStrikeSkill, 1, func() bool { return p.srv.PlayerCurrentCP(t, p.victimID) < cp })
	left := int32(p.srv.PlayerCurrentCP(t, p.victimID))

	if dealt := youDidDamage(t, readFrameLog(p.c)); dealt != want {
		t.Fatalf("caster YOU_DID_S1_DMG = %d, want %d", dealt, want)
	}
	log := readFrameLog(p.vc)
	gave := log.index(isSystemMessage(serverpackets.SystemMessageS1GaveYouS2Dmg))
	status := log.index(func(frame []byte) bool {
		got, ok := statusValue(frame, p.victimID, serverpackets.StatusCurrentCP)
		return ok && got == left
	})
	if gave < 0 || status < 0 || status > gave {
		t.Fatalf("Victim frames: StatusUpdate CP %d at %d, S1_GAVE_YOU_S2_DMG at %d; want the status first", left, status, gave)
	}
	assertGaveYouDamage(t, log[gave], "Mage", want)
}

// TestSelfCPDamagePercentReportsInCastOrder casts CPDAMPERCENT on the
// caster itself: it reads the damage it dealt, its own status with the
// lowered CP, then the damage it took, and no further status (issue #2587).
func TestSelfCPDamagePercentReportsInCastOrder(t *testing.T) {
	t.Parallel()
	const skillID = 9105
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, StaticHitTime: true, SkillType: "CPDAMPERCENT", Power: 50,
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 1)
	startInWorld(t, c)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetCP(pc.MaxCPValue()) })
	drainUntilQuiet(t, c)
	cp := srv.PlayerCurrentCP(t, objID)
	want := int32(cp / 2)

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 0, objID)
	srv.AdvanceUntil(t, "CPDAMPERCENT on the caster", func() bool { return srv.PlayerCurrentCP(t, objID) < cp })
	left := int32(srv.PlayerCurrentCP(t, objID))

	log := readFrameLog(c)
	dealt := log.index(isSystemMessage(serverpackets.SystemMessageYouDidS1Dmg))
	if dealt < 0 || dealt+2 >= len(log) {
		t.Fatalf("YOU_DID_S1_DMG at %d of %d frames, want it followed by the status and the damage taken", dealt, len(log))
	}
	if amount := youDidDamage(t, log); amount != want {
		t.Fatalf("YOU_DID_S1_DMG = %d, want %d", amount, want)
	}
	assertCasterStatus(t, srv, log[dealt+1], objID, srv.PlayerCurrentHP(t, objID), srv.PlayerCurrentMP(t, objID))
	if got, _ := statusValue(log[dealt+1], objID, serverpackets.StatusCurrentCP); got != left {
		t.Fatalf("status CP = %d, want %d", got, left)
	}
	assertGaveYouDamage(t, log[dealt+2], "Newbie", want)
	for _, frame := range log[dealt+3:] {
		if frame[0] == serverpackets.OpcodeStatusUpdate {
			t.Fatal("a further StatusUpdate followed the damage taken, want the cast's single status")
		}
	}
}
