package party

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelplayer "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// Reference: EffectList.updateEffectIcons (EffectList.java:778-864) sends a
// player its own AbnormalStatusUpdate and, in a party, a PartySpelled of
// the same in-use icon effects to every member, itself included.
// PartySpelled (opcode 0xee) writes D type (0 for a player), D object id,
// D count, then per effect D skill id, H skill level, D remaining ms /
// 1000. Party's constructor and addPartyMember run
// member.updateEffectIcons(true) before each broadcastUserInfo
// (Party.java:96-101, 363-368): the party-only pass sends no
// AbnormalStatusUpdate, and a member whose list never held an effect sends
// nothing (EffectList.java:428-431). Player.doDie ends with the same guarded
// updateEffectIcons() (Player.java:2674).

const (
	spelledSkill = 1068
	spelledSecs  = 60
)

// playerPartySpelledBytes is the expected PartySpelled of a player,
// written field by field from the reference layout; with no seconds it
// lists no effect.
func playerPartySpelledBytes(objectID int32, seconds ...int32) []byte {
	b := []byte{0xee}
	b = binary.LittleEndian.AppendUint32(b, 0)
	b = binary.LittleEndian.AppendUint32(b, uint32(objectID))
	b = binary.LittleEndian.AppendUint32(b, uint32(len(seconds)))
	for _, s := range seconds {
		b = binary.LittleEndian.AppendUint32(b, spelledSkill)
		b = binary.LittleEndian.AppendUint16(b, 1)
		b = binary.LittleEndian.AppendUint32(b, uint32(s))
	}
	return b
}

// onPlayerQueue runs fn on objID's own queue and waits for it.
func onPlayerQueue(t *testing.T, g *group, objID int32, fn func(*modelplayer.Character)) {
	t.Helper()
	obj, ok := g.srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d not in the world", objID)
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %d is %T, not an online character", objID, obj)
	}
	done := make(chan struct{})
	if !pc.Queue().Post(func() { fn(pc); close(done) }) {
		t.Fatal("post to player queue: queue closed")
	}
	<-done
}

// buff lands a 60 s icon buff on objID.
func (g *group) buff(t *testing.T, objID int32) {
	t.Helper()
	g.buffFor(t, objID, spelledSecs)
}

// buffFor lands an icon buff of secs seconds on objID; -1 is permanent.
func (g *group) buffFor(t *testing.T, objID int32, secs int) {
	t.Helper()
	onPlayerQueue(t, g, objID, func(pc *modelplayer.Character) {
		e, err := effect.New(effect.Skill{ID: spelledSkill, Level: 1, SkillType: "BUFF"}, modelskill.EffectTemplate{Name: "Buff", Time: secs, Icon: true})
		if err != nil {
			t.Errorf("effect.New: %v", err)
			return
		}
		e.Effector, e.Effected = pc, pc
		pc.EffectList().Add(e)
	})
}

func framesOf(frames [][]byte, opcode byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == opcode {
			out = append(out, f)
		}
	}
	return out
}

// TestBuffShowsIconsToParty buffs a party's leader: the leader gets its
// AbnormalStatusUpdate, then every member, the leader too, gets the
// leader's PartySpelled. A partyless player's buff sends no PartySpelled.
func TestBuffShowsIconsToParty(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Loner", 40}})
	leader, member, loner := g.players[0], g.players[1], g.players[2]
	g.invite(t, 0, 1, 0)

	g.buff(t, leader.id)
	want := playerPartySpelledBytes(leader.id, spelledSecs)
	leaderFrames := drainFrames(t, leader.c)
	icons := withoutPositions(leaderFrames)
	if len(icons) != 2 || icons[0][0] != serverpackets.OpcodeAbnormalStatusUpdate || !sameIcons(icons[1], want) {
		t.Fatalf("leader frames = %x, want AbnormalStatusUpdate then PartySpelled %x", opcodes(icons), want)
	}
	memberFrames := withoutPositions(drainFrames(t, member.c))
	if len(memberFrames) != 1 || !sameIcons(memberFrames[0], want) {
		t.Fatalf("member frames = %x, want only the leader's PartySpelled %x", opcodes(memberFrames), want)
	}

	g.buff(t, loner.id)
	lonerFrames := drainFrames(t, loner.c)
	if n := len(framesOf(lonerFrames, serverpackets.OpcodePartySpelled)); n != 0 {
		t.Fatalf("partyless player got %d PartySpelled, want none", n)
	}
	if n := len(framesOf(lonerFrames, serverpackets.OpcodeAbnormalStatusUpdate)); n != 1 {
		t.Fatalf("partyless player got %d AbnormalStatusUpdate, want 1", n)
	}
}

// TestJoinShowsBuffedMemberIcons forms a party of a leader with a permanent
// buff and an unbuffed member: each member is shown the leader's
// PartySpelled just before the leader's info refresh, the unbuffed
// member's icons are never sent, and nobody gets an AbnormalStatusUpdate.
// The permanent buff's -1 ms reads as 0 seconds (-1 / 1000).
func TestJoinShowsBuffedMemberIcons(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
	leader, member := g.players[0], g.players[1]
	g.buffFor(t, leader.id, -1)
	g.quiet(t)

	leader.c.Send(encodeJoinParty(member.name, 0))
	assertSystemMessageText(t, leader.c.Read(), serverpackets.SystemMessageYouInvitedS1ToParty, member.name)
	assertFrameOpcode(t, member.c.Read(), serverpackets.OpcodeAskJoinParty, "AskJoinParty")
	member.c.Send(encodeAnswerJoinParty(1))
	memberFrames := drainFrames(t, member.c)
	leaderFrames := drainFrames(t, leader.c)

	want := playerPartySpelledBytes(leader.id, 0)
	for _, tc := range []struct {
		who    string
		frames [][]byte
		info   byte
	}{
		{"leader", leaderFrames, serverpackets.OpcodeUserInfo},
		{"member", memberFrames, serverpackets.OpcodeCharInfo},
	} {
		frames := withoutPositions(tc.frames)
		spelled := framesOf(frames, serverpackets.OpcodePartySpelled)
		if len(spelled) != 1 || !bytes.Equal(spelled[0], want) {
			t.Fatalf("%s PartySpelled frames = %x, want one %x", tc.who, spelled, want)
		}
		if n := len(framesOf(frames, serverpackets.OpcodeAbnormalStatusUpdate)); n != 0 {
			t.Fatalf("%s got %d AbnormalStatusUpdate on the join, want none", tc.who, n)
		}
		at := index(frames, serverpackets.OpcodePartySpelled)
		if at+1 >= len(frames) || frames[at+1][0] != tc.info {
			t.Fatalf("%s frames = %x, want the leader's PartySpelled right before its %#x", tc.who, opcodes(frames), tc.info)
		}
	}
}

// TestDeathRefreshesPartyIcons kills a buffed party member: its party mate
// is shown its icon list once more after the death, now empty. An unbuffed
// member's death sends neither an AbnormalStatusUpdate nor a PartySpelled.
func TestDeathRefreshesPartyIcons(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
	leader, member := g.players[0], g.players[1]
	g.invite(t, 0, 1, 0)

	kill := func(objID int32) {
		onPlayerQueue(t, g, objID, func(pc *modelplayer.Character) {
			pc.SetSpawnProtection(false)
			if !pc.Die(attackable.Combatant(nil)) {
				t.Error("Die did not kill a living player")
			}
		})
	}

	kill(member.id)
	for i, p := range g.players {
		frames := drainFrames(t, p.c)
		if n := len(framesOf(frames, serverpackets.OpcodePartySpelled)); n != 0 {
			t.Fatalf("player %d got %d PartySpelled for an unbuffed death, want none", i, n)
		}
		if n := len(framesOf(frames, serverpackets.OpcodeAbnormalStatusUpdate)); n != 0 {
			t.Fatalf("player %d got %d AbnormalStatusUpdate for an unbuffed death, want none", i, n)
		}
	}

	g.buff(t, leader.id)
	g.quiet(t)
	kill(leader.id)
	spelled := framesOf(drainFrames(t, member.c), serverpackets.OpcodePartySpelled)
	if want := playerPartySpelledBytes(leader.id); len(spelled) == 0 || !bytes.Equal(spelled[len(spelled)-1], want) {
		t.Fatalf("member's PartySpelled after the leader's death = %x, want it to end with %x", spelled, want)
	}
}

// sameIcons reports whether got is want, a one-effect icon list, but for
// its seconds left, which may have ticked one lower before the list was
// read.
func sameIcons(got, want []byte) bool {
	if len(got) != len(want) {
		return false
	}
	n := len(want) - 4
	secs, wantSecs := int32(binary.LittleEndian.Uint32(got[n:])), int32(binary.LittleEndian.Uint32(want[n:]))
	return bytes.Equal(got[:n], want[:n]) && (secs == wantSecs || secs == wantSecs-1)
}

func withoutPositions(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] != serverpackets.OpcodePartyMemberPosition {
			out = append(out, f)
		}
	}
	return out
}

func index(frames [][]byte, opcode byte) int {
	for i, f := range frames {
		if f[0] == opcode {
			return i
		}
	}
	return -1
}
