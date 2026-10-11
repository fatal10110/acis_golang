package quest

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The handle operations of the third-class saga core (#3598), driven
// through a booted world as its hooks call them.

// warriorClass is the first occupation a Human Fighter (class 0) takes.
const warriorClass = 1

// warriorClassTemplate lets the boot character change to warriorClass.
func warriorClassTemplate() gameservertest.Option {
	tmpl := gameservertest.ClassTemplate()
	tmpl.ID = warriorClass
	return gameservertest.WithClassTemplates(tmpl)
}

// msu is the head of a MagicSkillUse: who casts what at whom, from where.
type msu struct {
	caster, target, skill, level, hitTime, reuse int32
	x, y, z                                      int32
}

// readMSU reads the head of a MagicSkillUse frame.
func readMSU(f opsFrame) msu {
	r := f.r
	return msu{
		caster: r.ReadInt32(), target: r.ReadInt32(), skill: r.ReadInt32(), level: r.ReadInt32(),
		hitTime: r.ReadInt32(), reuse: r.ReadInt32(), x: r.ReadInt32(), y: r.ReadInt32(), z: r.ReadInt32(),
	}
}

// A hook below reports a failure with t.Errorf and returns: t.Fatal would
// end the player's queue goroutine and leave the invocation unanswered.

// TestScriptClassChangeAndCastHelper runs, from a quest hook, what the saga
// core does when a player finishes its saga (ThirdClassQuest lines 239-244
// and the cast helper of line 198): the class change (Player.setClassId),
// the base class moved when no subclass is active and the base is the old
// class (isSubClassActive, getBaseClass, setBaseClass), UserInfo broadcast
// (broadcastUserInfo), then two visual-only MagicSkillUse broadcast from the
// player: player to player, then NPC to NPC, hit time 6000, reuse 1.
//
// The player sees, in order: the class change's own MagicSkillUse (5103,
// hit time 1000, reuse 0), the class transfer message, UserInfo, then the
// two animations. A party mate standing by sees the 5103 animation,
// CharInfo, then the two animations.
func TestScriptClassChangeAndCastHelper(t *testing.T) {
	t.Parallel()
	srv, leader, _, mateClient := partyPair(t, "1", "1", warriorClassTemplate())
	x, y, z := srv.PlayerPosition(t, leader)
	tablet := script.NPCOf(srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 31646), location.Location{X: x + 40, Y: y, Z: z}))
	at := script.LocOf(tablet)
	srv.ReadQueued(t, srv.Client)
	srv.ReadQueued(t, mateClient)

	run(t, srv, leader, func(_ *script.Script, p *script.Player) {
		prev := script.ClassParent(warriorClass)
		if p.ClassID() != prev || p.BaseClass() != prev || p.SubClassActive() {
			t.Errorf("before: class %d base %d subclass %v, want %d, %d, false", p.ClassID(), p.BaseClass(), p.SubClassActive(), prev, prev)
			return
		}
		p.SetClassID(warriorClass)
		if p.ClassID() != warriorClass || p.BaseClass() != prev {
			t.Errorf("after SetClassID: class %d base %d, want %d, %d", p.ClassID(), p.BaseClass(), warriorClass, prev)
			return
		}
		if !p.SubClassActive() && p.BaseClass() == prev {
			p.SetBaseClass(warriorClass)
		}
		if p.ClassID() != warriorClass || p.BaseClass() != warriorClass {
			t.Errorf("after SetBaseClass: class %d base %d, want both %d", p.ClassID(), p.BaseClass(), warriorClass)
			return
		}
		p.BroadcastUserInfo()
		p.BroadcastMagicSkillUse(p, p, 4339, 1, 6000, 1)
		p.BroadcastMagicSkillUse(tablet, tablet, 4339, 1, 6000, 1)
	})

	want5103 := msu{caster: leader, target: leader, skill: 5103, level: 1, hitTime: 1000, reuse: 0, x: int32(x), y: int32(y), z: int32(z)}
	wantSelf := msu{caster: leader, target: leader, skill: 4339, level: 1, hitTime: 6000, reuse: 1, x: int32(x), y: int32(y), z: int32(z)}
	wantNPC := msu{caster: tablet.ObjectID(), target: tablet.ObjectID(), skill: 4339, level: 1, hitTime: 6000, reuse: 1, x: int32(at.X), y: int32(at.Y), z: int32(at.Z)}
	interest := []byte{serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeUserInfo, serverpackets.OpcodeCharInfo}

	got := framesOf(srv.ReadQueued(t, srv.Client), interest...)
	want := []byte{serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeUserInfo, serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeMagicSkillUse}
	if !slices.Equal(opcodesOf(got), want) {
		t.Fatalf("player frames %x, want %x", opcodesOf(got), want)
	}
	if m := readMSU(got[0]); m != want5103 {
		t.Errorf("class change animation %+v, want %+v", m, want5103)
	}
	if id := got[1].r.ReadInt32(); id != serverpackets.SystemMessageClassTransfer {
		t.Errorf("system message %d, want the class transfer %d", id, serverpackets.SystemMessageClassTransfer)
	}
	if m := readMSU(got[3]); m != wantSelf {
		t.Errorf("player animation %+v, want %+v", m, wantSelf)
	}
	if m := readMSU(got[4]); m != wantNPC {
		t.Errorf("NPC animation %+v, want %+v", m, wantNPC)
	}

	seen := framesOf(srv.ReadQueued(t, mateClient), interest...)
	wantSeen := []byte{serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeCharInfo, serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeMagicSkillUse}
	if !slices.Equal(opcodesOf(seen), wantSeen) {
		t.Fatalf("mate frames %x, want %x", opcodesOf(seen), wantSeen)
	}
	for i, w := range map[int]msu{0: want5103, 2: wantSelf, 3: wantNPC} {
		if m := readMSU(seen[i]); m != w {
			t.Errorf("mate animation %d %+v, want %+v", i, m, w)
		}
	}
}

// TestScriptPartyMembersAndWorldPlayer pins the raw party and world
// lookups the saga core's dying and decayed hooks make: Player.isInParty,
// Party.getMembers in party order with no range or state check, and
// World.getPlayer by object id.
func TestScriptPartyMembersAndWorldPlayer(t *testing.T) {
	t.Parallel()
	srv, leader, mate, _ := partyPair(t, "1", "2")
	// The mate walks far away: the party lookups still list it.
	srv.RunQuest(t, mate, q001, func(_ *script.Quests, c *player.Character, _ *script.Script) {
		x, y, z := c.Position()
		c.SyncPosition(location.Location{X: x - 20*partyRange, Y: y, Z: z})
	})
	names := func(ps []*script.Player) []string {
		var out []string
		for _, p := range ps {
			out = append(out, p.Name())
		}
		return out
	}
	for _, objID := range []int32{leader, mate} {
		run(t, srv, objID, func(sc *script.Script, p *script.Player) {
			if !p.InParty() {
				t.Error("InParty() = false in a party")
			}
			if got := names(p.PartyMembers()); !slices.Equal(got, []string{"Newbie", "Mate"}) {
				t.Errorf("PartyMembers() = %v, want the leader then the mate", got)
			}
			if got := sc.WorldPlayer(mate); got == nil || got.Name() != "Mate" || !script.Same(got, p.PartyMembers()[1]) {
				t.Errorf("WorldPlayer(mate) = %v", got)
			}
			if got := sc.WorldPlayer(0); got != nil {
				t.Errorf("WorldPlayer(0) = %v, want nil", got)
			}
		})
	}
}

// TestScriptLoneAndOfflinePlayer: a player in no party is not in one and
// has no members; an object id no player in the world has finds nobody,
// an NPC's included.
func TestScriptLoneAndOfflinePlayer(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t)
	near, _ := npcsAround(t, srv, objID)
	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		if p.InParty() || p.PartyMembers() != nil {
			t.Errorf("alone: InParty %v, PartyMembers %v", p.InParty(), p.PartyMembers())
		}
		if got := sc.WorldPlayer(objID); got == nil || got.ObjectID() != objID {
			t.Errorf("WorldPlayer(self) = %v", got)
		}
		if got := sc.WorldPlayer(near.ObjectID()); got != nil {
			t.Errorf("WorldPlayer of an NPC = %v, want nil", got)
		}
	})
}

// TestQuestStateQuestIsItsRegisteredScript pins QuestState.getQuest as the
// saga core's Shrine of Loyalty hook uses it: the state of another quest a
// script finds answers that quest's registered script, whose helpers then
// check the state as that quest (its condition, its party range).
func TestQuestStateQuestIsItsRegisteredScript(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t)
	insertJournal(t, srv.DB,
		journalRow{objID, q001, "<state>", val("STARTED")},
		journalRow{objID, q001, "<cond>", val("2")},
		journalRow{objID, "q002_whatwomenwant", "<state>", val("STARTED")},
		journalRow{objID, "q002_whatwomenwant", "<cond>", val("15")},
	)
	enterWorld(t, srv)
	near, edge := npcsAround(t, srv, objID)

	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		own := sc.QuestState(p, q001)
		if own == nil || own.Quest() != sc {
			t.Errorf("own state's quest = %v, want the invoking script", own)
			return
		}
		other := sc.QuestState(p, "Q002_WhatWomenWant")
		if other == nil {
			t.Error("no Q002 state")
			return
		}
		q002 := other.Quest()
		if q002 == nil || q002 == sc || q002.Name != "Q002_WhatWomenWant" || q002.QuestID != 2 {
			t.Errorf("Q002 state's quest = %+v", q002)
			return
		}
		if st := q002.CheckPlayerCondition(p, near, 15); st == nil || st.Quest() != q002 {
			t.Errorf("Q002's check at cond 15 = %v, want its state", st)
		}
		if st := q002.CheckPlayerCondition(p, edge, 15); st != nil {
			t.Error("Q002's check out of range found a state")
		}
		if st := sc.CheckPlayerCondition(p, near, 15); st != nil {
			t.Error("Q001's check found Q002's condition")
		}
	})
	if !srv.RunScript(t, objID, "Q003_WillTheSealBeBroken", func(sc *script.Script, p *script.Player) {
		if st := sc.NewQuestState(p); st.Quest() != sc {
			t.Errorf("a new state's quest = %v, want the script that made it", st.Quest())
		}
	}) {
		t.Fatal("Q003 invocation panicked")
	}
}
