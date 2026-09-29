package combat

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Player.reviveRequest / reviveAnswer (Player.java:6015-6084),
// Player.doDie's Phoenix Blessing offer (Player.java:2670-2674),
// Playable.doDie's blessing branch (Playable.java:146-165), Playable.doRevive
// (Playable.java:189-207), Player.doRevive (Player.java:5993-6013) and the
// Resurrect handler (Resurrect.java:24-52), aCis revision in the outer repo.

const (
	resurrectSkillID  = 1016
	phoenixSkillID    = 1002
	charmOfLuckSkill  = 1003
	resurrectionDlgID = serverpackets.ConfirmDlgResurrectionRequest
)

// reviveVictim is the part of the world's player the scenarios inspect.
type reviveVictim interface {
	Dead() bool
	CurrentHP() int
	ResourceValues() player.Resources
	EffectList() *effect.List
	SetCP(float64)
	SetSpawnProtection(bool)
	ReduceHP(float64, attackable.Combatant, modelskill.Definition)
}

func encodeDlgAnswer(messageID, answer, requesterID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeDlgAnswer)
	w.WriteInt32(messageID)
	w.WriteInt32(answer)
	w.WriteInt32(requesterID)
	return w.Bytes()
}

// resurrectSkill is a CORPSE_PLAYER resurrection with full power, so the
// accepted offer restores all of the lost exp whatever the caster's WIT.
func resurrectSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: resurrectSkillID, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetCorpsePlayer, SkillType: "RESURRECT",
		CastRange: 400, HitTime: 500, StaticHitTime: true, Power: 100,
	}
}

func reviveVictimOf(t *testing.T, srv *gameservertest.Server, id int32) reviveVictim {
	t.Helper()
	obj, ok := srv.State.Player(id)
	if !ok {
		t.Fatal("victim missing from world state")
	}
	v, ok := obj.(reviveVictim)
	if !ok {
		t.Fatalf("world victim %T lacks the revive surface", obj)
	}
	return v
}

// killByMonster has a monster take the victim's last HP, so the death runs
// the monster-kill costs (exp loss, death-penalty roll).
func killByMonster(t *testing.T, srv *gameservertest.Server, v reviveVictim) {
	t.Helper()
	hostile := srv.SpawnHostileNPC(t)
	srv.Settle(t)
	v.SetSpawnProtection(false)
	v.SetCP(0)
	v.ReduceHP(float64(v.CurrentHP()), hostile, modelskill.Definition{})
	srv.Settle(t)
	if !v.Dead() {
		t.Fatal("victim survived the monster's hit")
	}
}

// assertResurrectionOffer checks a ConfirmDlg frame is the resurrection
// offer naming reviver.
func assertResurrectionOffer(t *testing.T, frame []byte, reviver string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeConfirmDlg, "resurrection ConfirmDlg")
	r := wireReader(frame[1:])
	if id, params, typ := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != resurrectionDlgID || params != 1 || typ != 0 {
		t.Fatalf("ConfirmDlg = id %d params %d type %d, want %d/1/text", id, params, typ, resurrectionDlgID)
	}
	if got := r.ReadString(); got != reviver {
		t.Fatalf("ConfirmDlg reviver = %q, want %q", got, reviver)
	}
	if r.Remaining() != 0 {
		t.Fatalf("ConfirmDlg carries %d trailing bytes, want none", r.Remaining())
	}
}

func indexOfOpcode(frames [][]byte, from int, op byte) int {
	for i := from; i < len(frames); i++ {
		if frames[i][0] == op {
			return i
		}
	}
	return -1
}

func countOpcode(frames [][]byte, to int, op byte) int {
	n := 0
	for _, f := range frames[:to] {
		if f[0] == op {
			n++
		}
	}
	return n
}

// persistedExp logs the client out and reads the saved exp and the exp it
// had before its last death.
func persistedExp(t *testing.T, srv *gameservertest.Server, c *scriptedClient, id int32) (exp, before int64) {
	t.Helper()
	logoutPersisted(t, srv, c)
	ch, err := srv.Chars.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("load victim: %v", err)
	}
	return ch.Exp, ch.ExpBeforeDeath
}

// TestPhoenixBlessedDeathOffersFullRevive: a Phoenix-Blessed player killed
// by a monster keeps the blessing, loses its Charm of Luck (appearance
// refreshed for the effect's end and for the stop), skips the death-penalty
// level its karma would otherwise earn, and is offered its own
// resurrection after Die, followed by the retained icons. Accepting
// restores all of the lost exp, full HP and MP, and uses up the blessing;
// declining uses up the blessing and leaves the player dead, so the next
// restart revives at the normal HP share.
func TestPhoenixBlessedDeathOffersFullRevive(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		answer int32
	}{
		{name: "accepted", answer: 1},
		{name: "declined", answer: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithWantChars(1),
				gameservertest.WithSeed(seedExperiencedCharacter(1500, 240)),
				gameservertest.WithLevels(deathLossTable(t)),
				gameservertest.WithAllowDelevel(true),
				gameservertest.WithRateKarmaExpLost(1.0),
				gameservertest.WithRestartPoints(restartTable()),
				gameservertest.WithSkills(combatPersistence(t, deathEffectSkills())),
			)
			c, id := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, id, phoenixSkillID, 1)
			seedKnownSkill(t, srv, id, charmOfLuckSkill, 1)
			startInWorld(t, c)
			for _, skillID := range []int32{phoenixSkillID, charmOfLuckSkill} {
				c.Send(encodeRequestMagicSkillUse(skillID, false, false))
				drainUntilQuiet(t, c)
			}
			victim := reviveVictimOf(t, srv, id)

			killByMonster(t, srv, victim)
			frames := readQuiet(c)
			die := indexOf(frames, 0, serverpackets.OpcodeDie, id)
			if die < 0 {
				t.Fatal("victim never received its own Die")
			}
			if got := countOpcode(frames, die, serverpackets.OpcodeUserInfo); got != 2 {
				t.Fatalf("UserInfo frames before Die = %d, want 2 for the Charm of Luck's end and its stop", got)
			}
			if penalty := indexOfSystemMessage(frames, 0, serverpackets.SystemMessageDeathPenaltyLevelS1Added); penalty >= 0 {
				t.Fatalf("death-penalty level added at frame %d, want none under a Phoenix Blessing", penalty)
			}
			offer := indexOfOpcode(frames, die+1, serverpackets.OpcodeConfirmDlg)
			if offer < 0 {
				t.Fatal("no resurrection offer after Die")
			}
			assertResurrectionOffer(t, frames[offer], "Victim")
			if icons := indexOfOpcode(frames, offer+1, serverpackets.OpcodeAbnormalStatusUpdate); icons < 0 {
				t.Fatal("no effect-icon refresh after the resurrection offer")
			}
			if got := effectIDs(victim.EffectList().All()); len(got) != 1 || got[0] != phoenixSkillID {
				t.Fatalf("effects after death = %v, want only the Phoenix Blessing %d", got, phoenixSkillID)
			}

			c.Send(encodeDlgAnswer(resurrectionDlgID, tc.answer, 0))
			srv.Settle(t)
			frames = readQuiet(c)
			if hasEffectID(victim.EffectList().All(), phoenixSkillID) {
				t.Fatal("Phoenix Blessing still active after the answer")
			}

			if tc.answer == 0 {
				if !victim.Dead() {
					t.Fatal("declined offer revived the player")
				}
				if revive := indexOf(frames, 0, serverpackets.OpcodeRevive, id); revive >= 0 {
					t.Fatalf("declined offer sent Revive at frame %d", revive)
				}
				c.Send(encodeRequestRestartPoint(0))
				assertRestartTeleport(t, c, id, chaosRestartPoint)
				res := victim.ResourceValues()
				if want := res.MaxHP * 0.7; res.CurrentHP != want {
					t.Fatalf("HP after restart = %v, want the respawn share %v of %v", res.CurrentHP, want, res.MaxHP)
				}
				if exp, before := persistedExp(t, srv, c, id); exp != 1300 || before != 1500 {
					t.Fatalf("persisted exp/before-death = %d/%d, want 1300/1500 (loss kept)", exp, before)
				}
				return
			}

			if victim.Dead() {
				t.Fatal("accepted offer left the player dead")
			}
			status := indexOfOpcode(frames, 0, serverpackets.OpcodeStatusUpdate)
			revive := indexOf(frames, 0, serverpackets.OpcodeRevive, id)
			etc := indexOfOpcode(frames, max(revive, 0), serverpackets.OpcodeEtcStatusUpdate)
			if status < 0 || revive < status || etc < revive {
				t.Fatalf("revive frames: StatusUpdate %d, Revive %d, EtcStatusUpdate %d; want them in that order", status, revive, etc)
			}
			res := victim.ResourceValues()
			if res.CurrentHP != res.MaxHP || res.CurrentMP != res.MaxMP {
				t.Fatalf("HP/MP after blessed revive = %v/%v, want full %v/%v", res.CurrentHP, res.CurrentMP, res.MaxHP, res.MaxMP)
			}
			if exp, before := persistedExp(t, srv, c, id); exp != 1500 || before != 0 {
				t.Fatalf("persisted exp/before-death = %d/%d, want 1500/0 (all restored)", exp, before)
			}
		})
	}
}

// resurrectionScene boots a victim with lost exp and a healer who knows the
// resurrection, kills the victim to a monster, and has the healer target
// the corpse.
type resurrectionScene struct {
	srv      *gameservertest.Server
	victim   *scriptedClient
	victimID int32
	healer   *scriptedClient
	healerID int32
	v        reviveVictim
}

func newResurrectionScene(t *testing.T) *resurrectionScene {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(seedExperiencedCharacter(1500, 0)),
		gameservertest.WithLevels(deathLossTable(t)),
		gameservertest.WithAllowDelevel(true),
		gameservertest.WithRestartPoints(restartTable()),
		gameservertest.WithSkills(combatPersistence(t, []modelskill.Definition{resurrectSkill()})),
	)
	s := &resurrectionScene{srv: srv, victim: srv.Client, victimID: srv.SoleObjectID(t)}
	startInWorld(t, s.victim)
	healer := srv.SeedCharacterFor(t, "healer", "Healer", 5, 0)
	s.healerID = healer.ID
	seedKnownSkill(t, srv, s.healerID, resurrectSkillID, 1)
	s.healer = srv.DialClient(t, "healer", 1)
	startInWorld(t, s.healer)
	drainUntilQuiet(t, s.healer)
	drainUntilQuiet(t, s.victim)

	s.v = reviveVictimOf(t, srv, s.victimID)
	killByMonster(t, srv, s.v)
	drainUntilQuiet(t, s.victim)
	drainUntilQuiet(t, s.healer)
	selectPlayerTarget(t, s.healer, s.victimID)
	drainUntilQuiet(t, s.healer)
	return s
}

// castResurrection has the healer cast on its target and lets the hit land.
func (s *resurrectionScene) castResurrection(t *testing.T) {
	t.Helper()
	s.healer.Send(encodeRequestMagicSkillUse(resurrectSkillID, false, false))
	s.srv.Settle(t)
	s.srv.Advance(t, 600*time.Millisecond)
}

// TestResurrectionAsksTheCorpse: a player's resurrection offers the dead
// player the revive instead of reviving it; a second offer while the first
// is open is refused to the healer. Accepting revives at the respawn HP
// share (the power restores exp only), shows the revive to the healer, and
// restores the power's share of the lost exp; a later answer does nothing.
func TestResurrectionAsksTheCorpse(t *testing.T) {
	t.Parallel()
	s := newResurrectionScene(t)

	s.castResurrection(t)
	if !s.v.Dead() {
		t.Fatal("resurrection revived the corpse without asking")
	}
	frames := readQuiet(s.victim)
	offer := indexOfOpcode(frames, 0, serverpackets.OpcodeConfirmDlg)
	if offer < 0 {
		t.Fatal("victim never got the resurrection offer")
	}
	assertResurrectionOffer(t, frames[offer], "Healer")
	drainUntilQuiet(t, s.healer)

	s.castResurrection(t)
	healerFrames := readQuiet(s.healer)
	if i := indexOfSystemMessage(healerFrames, 0, serverpackets.SystemMessageResHasAlreadyBeenProposed); i < 0 {
		t.Fatal("second offer was not refused to the healer with RES_HAS_ALREADY_BEEN_PROPOSED")
	}
	if i := indexOfOpcode(readQuiet(s.victim), 0, serverpackets.OpcodeConfirmDlg); i >= 0 {
		t.Fatal("victim got a second offer while the first was open")
	}

	s.victim.Send(encodeDlgAnswer(resurrectionDlgID, 1, 0))
	s.srv.Settle(t)
	if s.v.Dead() {
		t.Fatal("accepted offer left the victim dead")
	}
	res := s.v.ResourceValues()
	if want := res.MaxHP * 0.7; res.CurrentHP != want {
		t.Fatalf("HP after resurrection = %v, want the respawn share %v of %v", res.CurrentHP, want, res.MaxHP)
	}
	if i := indexOf(readQuiet(s.healer), 0, serverpackets.OpcodeRevive, s.victimID); i < 0 {
		t.Fatal("healer never saw the victim revive")
	}
	drainUntilQuiet(t, s.victim)

	s.victim.Send(encodeDlgAnswer(resurrectionDlgID, 1, 0))
	s.srv.Settle(t)
	if extra := s.victim.ReadWithTimeout(300 * time.Millisecond); extra != nil {
		t.Fatalf("repeated answer sent opcode %#x, want nothing", extra[0])
	}
	if exp, before := persistedExp(t, s.srv, s.victim, s.victimID); exp != 1500 || before != 0 {
		t.Fatalf("persisted exp/before-death = %d/%d, want 1500/0", exp, before)
	}
}

// TestResurrectionAcceptedAfterRestartGivesNothing: the offer lapses when
// the victim goes back to town, so accepting it afterwards neither revives
// again nor returns the lost exp.
func TestResurrectionAcceptedAfterRestartGivesNothing(t *testing.T) {
	t.Parallel()
	s := newResurrectionScene(t)

	s.castResurrection(t)
	if offer := indexOfOpcode(readQuiet(s.victim), 0, serverpackets.OpcodeConfirmDlg); offer < 0 {
		t.Fatal("victim never got the resurrection offer")
	}
	s.victim.Send(encodeRequestRestartPoint(0))
	assertRestartTeleport(t, s.victim, s.victimID, townRestartPoint)
	drainUntilQuiet(t, s.victim)

	s.victim.Send(encodeDlgAnswer(resurrectionDlgID, 1, 0))
	s.srv.Settle(t)
	if extra := s.victim.ReadWithTimeout(300 * time.Millisecond); extra != nil {
		t.Fatalf("answer after restart sent opcode %#x, want nothing", extra[0])
	}
	if exp, before := persistedExp(t, s.srv, s.victim, s.victimID); exp != 1300 || before != 1500 {
		t.Fatalf("persisted exp/before-death = %d/%d, want 1300/1500 (loss kept)", exp, before)
	}
}
