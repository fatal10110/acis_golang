package olympiad

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestOlympiadMatchLethalHitOnlyMarksDead pins a competitor whose HP a hit
// empties during the fight (PlayerStatus.reduceHp's Olympiad branch): it is
// told the damage and its emptied status, its attack and cast are stopped,
// and it is marked dead, nothing more. No one sees it die, its opponent is
// credited no kill and it loses no exp; the fight ends on the mark, won by
// the opponent, and the stadium lifts the mark and heals it without a
// revive.
func TestOlympiadMatchLethalHitOnlyMarksDead(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 60_000 })
	a, b := m.ones[0], m.ones[1]

	m.fighting(t)
	alpha, bravo := m.online(t, a), m.online(t, b)
	alphaBefore, bravoBefore := alpha.ProgressionValues(), bravo.ProgressionValues()
	penaltyBefore := bravo.DeathPenaltyLevel()
	for _, p := range m.ones {
		p.frames = nil
	}

	var killed bool
	m.onQueue(t, bravo, func() { killed = bravo.TakeDamage(1_000_000, alpha) })
	if !killed || !bravo.Dead() {
		t.Fatalf("lethal hit: killed %v, dead %v; want both", killed, bravo.Dead())
	}
	// A second lethal hit finds the competitor already down.
	m.onQueue(t, bravo, func() { killed = bravo.TakeDamage(1_000_000, alpha) })
	if killed {
		t.Fatal("a second lethal hit killed the fallen competitor again")
	}
	m.read(t)

	// The damage report, the emptied HP's status, then one ActionFailed
	// each for the stopped attack and the stopped cast.
	want := []string{"SystemMessage 36", "StatusUpdate", "ActionFailed", "ActionFailed"}
	if got := opcodeNames(b.frames); !slices.Equal(got, want) {
		t.Fatalf("Bravo's frames for the lethal hit = %q, want %q", got, want)
	}
	for _, p := range m.ones {
		if slices.ContainsFunc(p.frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeDie }) {
			t.Fatalf("%s saw a Die packet for an Olympiad fall", p.name)
		}
	}
	if got := alpha.ProgressionValues(); got != alphaBefore {
		t.Errorf("Alpha's progression after the kill = %+v, want unchanged %+v", got, alphaBefore)
	}
	if got := bravo.ProgressionValues(); got != bravoBefore {
		t.Errorf("Bravo's progression after the fall = %+v, want unchanged %+v", got, bravoBefore)
	}
	if got := bravo.DeathPenaltyLevel(); got != penaltyBefore {
		t.Errorf("Bravo's death penalty = %d, want unchanged %d", got, penaltyBefore)
	}

	m.over(t)
	result := []string{"SM 1497 Alpha", "SM 1657 Alpha 3", "SM 1658 Bravo 3"}
	got := a.course()
	if i := slices.Index(got, result[0]); i < 0 || i+len(result) > len(got) || !slices.Equal(got[i:i+len(result)], result) {
		t.Fatalf("Alpha course %q does not hold the result %q", got, result)
	}
	if bravo.Dead() {
		t.Fatal("Bravo still dead once the match is over")
	}
	if res := bravo.ResourceValues(); res.CurrentHP != res.MaxHP {
		t.Fatalf("Bravo's HP after the match = %v of %v, want full", res.CurrentHP, res.MaxHP)
	}
	for _, p := range m.ones {
		if slices.ContainsFunc(p.frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeDie || f[0] == serverpackets.OpcodeRevive }) {
			t.Fatalf("%s saw a Die or Revive packet over the match", p.name)
		}
	}
}

// TestOlympiadMatchLethalHitIdlesTheSummon pins the summon of a competitor
// a lethal hit fells: sent after its owner's opponent, it is sent idle and
// goes back to following its owner.
func TestOlympiadMatchLethalHitIdlesTheSummon(t *testing.T) {
	t.Parallel()
	const servitorID = 12564
	m := bootMatch(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 60_000 },
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{
			ID: servitorID, TemplateID: servitorID, Type: "Servitor", Name: "Sin Eater", Level: 20,
			HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		}})))
	a, b := m.ones[0], m.ones[1]

	m.fighting(t)
	alpha, bravo := m.online(t, a), m.online(t, b)
	m.onQueue(t, bravo, func() {
		bravo.SummonServitor(modelskill.Definition{ID: 1111, Level: 1, SkillType: "SUMMON", NpcID: servitorID, SummonTotalLifeTime: 1_200_000})
	})
	var servitor *summon.Actor
	m.srv.AdvanceUntil(t, "the servitor is in the world", func() bool {
		obj, ok := m.srv.State.Summon(b.id)
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	m.read(t)
	// Bravo selects Alpha, then orders its servitor to attack; the servitor
	// is still walking to Alpha when the lethal hit lands.
	b.c.Send(encodeAttackRequest(a.id))
	m.read(t)
	b.c.Send(encodeServitorAttack())
	m.read(t)
	if got := servitor.Intent(); got != summon.IntentAttackTarget {
		t.Fatalf("servitor sent after Alpha: intent %v, want attack-target", got)
	}

	m.onQueue(t, bravo, func() { bravo.TakeDamage(1_000_000, alpha) })
	if !bravo.Dead() {
		t.Fatal("Bravo not marked dead by the lethal hit")
	}
	if got := servitor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("servitor of the fallen Bravo: intent %v, want follow-owner", got)
	}
	m.read(t)
}

// encodeServitorAttack is a RequestActionUse ordering the servitor to
// attack its owner's target.
func encodeServitorAttack() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestActionUse)
	w.WriteInt32(22)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

// online is p's live character.
func (m *match) online(t *testing.T, p *competitor) *player.Character {
	t.Helper()
	obj, ok := m.srv.State.Player(p.id)
	if !ok {
		t.Fatalf("%s not in world", p.name)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%s is %T, not an online character", p.name, obj)
	}
	return c
}

// onQueue runs fn on c's queue and waits for it.
func (m *match) onQueue(t *testing.T, c *player.Character, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !c.Queue().Post(func() { fn(); close(done) }) {
		t.Fatalf("%s's queue is closed", c.CharacterName())
	}
	<-done
}

// opcodeNames names the frames a lethal hit sends: status updates, system
// messages by id, action failures, and any other opcode by number.
func opcodeNames(frames [][]byte) []string {
	var out []string
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeStatusUpdate:
			out = append(out, "StatusUpdate")
		case serverpackets.OpcodeSystemMessage:
			out = append(out, fmt.Sprintf("SystemMessage %d", wire.NewReader(f[1:]).ReadInt32()))
		case serverpackets.OpcodeActionFailed:
			out = append(out, "ActionFailed")
		default:
			out = append(out, fmt.Sprintf("0x%02x", f[0]))
		}
	}
	return out
}
