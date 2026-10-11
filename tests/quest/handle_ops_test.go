package quest

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The beginner shots RewardNewbieShots hands out.
const (
	soulshotNoGrade   = 5789
	spiritshotNoGrade = 5790
)

// opsItemTemplates is questItemTemplates plus the beginner shots.
func opsItemTemplates() gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range append(slices.Clone(q001Items), dropItem, soulshotNoGrade, spiritshotNoGrade) {
		templates = append(templates, &item.Template{
			ID: id, Name: "Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// opsFrame is one frame of interest the client got: its opcode and fields.
type opsFrame struct {
	opcode byte
	r      *wire.Reader
}

// framesOf returns the frames among frames whose opcode is one of opcodes,
// in order, each with a reader past its opcode.
func framesOf(frames [][]byte, opcodes ...byte) []opsFrame {
	var out []opsFrame
	for _, f := range frames {
		if len(f) > 0 && slices.Contains(opcodes, f[0]) {
			out = append(out, opsFrame{opcode: f[0], r: wire.NewReader(f[1:])})
		}
	}
	return out
}

// opcodesOf lists the opcode of each frame.
func opcodesOf(frames []opsFrame) []byte {
	out := make([]byte, len(frames))
	for i, f := range frames {
		out[i] = f.opcode
	}
	return out
}

// runOps runs fn as an invocation of the script name on objID's queue and
// fails the test when it panics.
func runOps(t *testing.T, srv *gameservertest.Server, objID int32, name string, fn func(sc *script.Script, p *script.Player)) {
	t.Helper()
	if !srv.RunScript(t, objID, name, fn) {
		t.Fatalf("%s invocation panicked", name)
	}
}

// TestScriptItemCountCountsUnitsHeld: a stack counts its units, a
// non-stackable item counts its instances, and an item the player does not
// hold counts 0 (ItemContainer.getItemCount).
func TestScriptItemCountCountsUnitsHeld(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t, opsItemTemplates())
	runOps(t, srv, objID, q001, func(sc *script.Script, p *script.Player) {
		sc.GiveItems(p, dropItem, 7)
		sc.GiveItems(p, swordID, 3)
		for _, tc := range []struct {
			id, want int32
		}{{dropItem, 7}, {swordID, 3}, {soulshotNoGrade, 0}} {
			if got := p.ItemCount(tc.id); got != tc.want {
				t.Errorf("ItemCount(%d) = %d, want %d", tc.id, got, tc.want)
			}
		}
	})
}

// TestNPCSayScriptValueAndSocialAction drives the NPC handle on a hostile
// and a civilian NPC: Say sends the players around the NPC one NpcSay of
// type ALL named by the NPC's npc id (not its client template id), with an
// NpcString's text; the script value starts at 0 and holds what is set;
// SocialAction shows the animation on the NPC.
func TestNPCSayScriptValueAndSocialAction(t *testing.T) {
	t.Parallel()
	w := bootDialog(t, nil, nil)
	hostileTmpl := &npc.Template{
		ID: 20040, TemplateID: 20041, Type: "Monster", Level: 20, HPMax: 1000, AtkSpd: 300,
		RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
	h := w.srv.SpawnHostileNPCTemplateAt(t, hostileTmpl, location.Location{X: w.at.X + 30, Y: w.at.Y, Z: w.at.Z})
	folkTmpl := gameservertest.FolkTemplate("Folk", 30868)
	folkTmpl.TemplateID = 30869
	f := w.srv.SpawnFolkNPCAt(t, folkTmpl, location.Location{X: w.at.X, Y: w.at.Y + 30, Z: w.at.Z})
	w.srv.ReadQueued(t, w.srv.Client)

	for _, tc := range []struct {
		name     string
		npc      attackable.Combatant
		objectID int32
		npcID    int32
	}{
		{"hostile", h, h.ObjectID(), 20040},
		{"folk", f, f.ObjectID(), 30868},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runOps(t, w.srv, w.player, q001, func(_ *script.Script, _ *script.Player) {
				n := script.NPCOf(tc.npc)
				if got := n.ScriptValue(); got != 0 {
					t.Errorf("ScriptValue() of a fresh spawn = %d, want 0", got)
				}
				if n.IsScriptValue(1) {
					t.Error("IsScriptValue(1) of a fresh spawn = true")
				}
				n.Say(script.NpcString(31603))
				n.SetScriptValue(1)
				if got := n.ScriptValue(); got != 1 || !n.IsScriptValue(1) {
					t.Errorf("ScriptValue() after SetScriptValue(1) = %d", got)
				}
				n.Say("Who awoke me?")
				n.SocialAction(3)
			})
			got := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeNpcSay, serverpackets.OpcodeSocialAction)
			if want := []byte{serverpackets.OpcodeNpcSay, serverpackets.OpcodeNpcSay, serverpackets.OpcodeSocialAction}; !slices.Equal(opcodesOf(got), want) {
				t.Fatalf("frames %x, want %x", opcodesOf(got), want)
			}
			for i, text := range []string{"Why do you oppress us so?", "Who awoke me?"} {
				r := got[i].r
				if obj, typ, id, line := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadString(); obj != tc.objectID || typ != serverpackets.SayTypeAll || id != 1_000_000+tc.npcID || line != text {
					t.Errorf("NpcSay %d = obj %d type %d npc %d %q, want obj %d type 0 npc %d %q", i, obj, typ, id, line, tc.objectID, 1_000_000+tc.npcID, text)
				}
			}
			if r := got[2].r; r.ReadInt32() != tc.objectID || r.ReadInt32() != 3 {
				t.Error("SocialAction is not the NPC's action 3")
			}
		})
	}
}

// bootShotsNewbie boots a level 10 human fighter in the world with the journal
// scripts and the beginner shots.
func bootShotsNewbie(t *testing.T) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 10, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		journalScripts(),
		opsItemTemplates(),
		gameservertest.WithScriptRates(script.Rates{Drop: 1, Reward: 2, RewardAdena: 1, XP: 1, SP: 1}),
	)
	enterWorld(t, srv)
	return srv, srv.SoleObjectID(t)
}

// TestRewardNewbieShots pins Quest.rewardNewbieShots: a newbie gets, once
// per quest, question mark 26, the shots for its class scaled by the reward
// rate, then the tutorial voice. A mystic gets spiritshots and voice 027,
// except an orc mystic, which gets soulshots and voice 026 as a fighter
// does. The quest's one-time memo is set even when the count for the
// class is 0. A player past its first class change gets nothing and no
// memo.
func TestRewardNewbieShots(t *testing.T) {
	t.Parallel()
	srv, objID := bootShotsNewbie(t)
	srv.ReadQueued(t, srv.Client)
	cases := []struct {
		quest   string
		class   int
		ss, sps int32
		itemID  int32
		want    int32 // units the call gives
		voice   string
		memo    bool
	}{
		{quest: q001, class: 0, ss: 6000, sps: 3000, itemID: soulshotNoGrade, want: 12000, voice: "tutorial_voice_026", memo: true},
		{quest: "Q002_WhatWomenWant", class: 10, ss: 6000, sps: 3000, itemID: spiritshotNoGrade, want: 6000, voice: "tutorial_voice_027", memo: true},
		{quest: "Q003_WillTheSealBeBroken", class: 49, ss: 7000, sps: 3000, itemID: soulshotNoGrade, want: 14000, voice: "tutorial_voice_026", memo: true},
		{quest: "Q006_StepIntoTheFuture", class: 49, ss: 0, sps: 3000, itemID: soulshotNoGrade, memo: true},
		{quest: noblesse, class: 2, ss: 6000, sps: 3000, itemID: soulshotNoGrade},
	}
	for _, tc := range cases {
		t.Run(tc.quest, func(t *testing.T) {
			flag := tc.quest + "_OneTimeQuestFlag"
			var before int32
			srv.RunQuest(t, objID, q001, func(_ *script.Quests, c *player.Character, _ *script.Script) {
				c.SetClassID(tc.class)
				before = int32(c.ItemCount(int(tc.itemID)))
			})
			runOps(t, srv, objID, tc.quest, func(sc *script.Script, p *script.Player) { sc.RewardNewbieShots(p, tc.ss, tc.sps) })
			frames := framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodeTutorialShowQuestionMark, serverpackets.OpcodePlaySound)
			after := int32(srv.PlayerItemCount(t, objID, tc.itemID))
			if got := after - before; got != tc.want {
				t.Errorf("item %d given = %d, want %d", tc.itemID, got, tc.want)
			}
			if tc.want == 0 {
				if len(frames) != 0 {
					t.Errorf("frames %x, want none", opcodesOf(frames))
				}
			} else {
				if want := []byte{serverpackets.OpcodeTutorialShowQuestionMark, serverpackets.OpcodePlaySound}; !slices.Equal(opcodesOf(frames), want) {
					t.Fatalf("frames %x, want question mark then voice", opcodesOf(frames))
				}
				if mark := frames[0].r.ReadInt32(); mark != 26 {
					t.Errorf("question mark %d, want 26", mark)
				}
				if typ, file := frames[1].r.ReadInt32(), frames[1].r.ReadString(); typ != 2 || file != tc.voice {
					t.Errorf("voice = type %d %q, want type 2 %q", typ, file, tc.voice)
				}
			}
			var memo string
			var set bool
			srv.RunQuest(t, objID, q001, func(_ *script.Quests, c *player.Character, _ *script.Script) {
				memo, set = c.Memos().Get(flag)
			})
			if set != tc.memo || (set && memo != "true") {
				t.Errorf("memo %s = %q set %v, want set %v", flag, memo, set, tc.memo)
			}

			// A second reward from the same quest gives nothing.
			runOps(t, srv, objID, tc.quest, func(sc *script.Script, p *script.Player) { sc.RewardNewbieShots(p, tc.ss, tc.sps) })
			if again := framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodeTutorialShowQuestionMark, serverpackets.OpcodePlaySound); tc.memo && len(again) != 0 {
				t.Errorf("repeat sent %x, want nothing", opcodesOf(again))
			}
			if tc.memo {
				if got := int32(srv.PlayerItemCount(t, objID, tc.itemID)); got != after {
					t.Errorf("repeat changed item %d from %d to %d", tc.itemID, after, got)
				}
			}
		})
	}
}

// clanOf seeds clan 7001 at level, with reputation score 100, led by Chief
// (a copy of the boot character on account chief, standing where it
// stands) and with the boot character as a member.
func clanOf(t *testing.T, level int) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		ctx := context.Background()
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		for _, q := range []string{
			`DROP TEMPORARY TABLE IF EXISTS chief`,
			`CREATE TEMPORARY TABLE chief SELECT * FROM characters WHERE char_name = 'Newbie'`,
			`UPDATE chief SET obj_Id = 900001, account_name = 'chief', char_name = 'Chief', clanid = 7001`,
			`INSERT INTO characters SELECT * FROM chief`,
			`UPDATE characters SET clanid = 7001 WHERE char_name = 'Newbie'`,
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) VALUES (7001, 'Chiefs', ` + strconv.Itoa(level) + `, 100, 900001)`,
			`DROP TEMPORARY TABLE chief`,
		} {
			if _, err := conn.ExecContext(ctx, q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
	})
}

// enterChief brings Chief into the world on its own client.
func enterChief(t *testing.T, srv *gameservertest.Server) *testsupport.ScriptedClient {
	t.Helper()
	c := srv.DialClient(t, "chief", 1)
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	srv.ReadQueued(t, c)
	srv.ReadQueued(t, srv.Client)
	return c
}

// clanReputation reads clan 7001's stored reputation score, waiting for
// the write to land.
func clanReputation(t *testing.T, srv *gameservertest.Server, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var got int
	for {
		if err := srv.DB.QueryRow(`SELECT reputation_score FROM clan_data WHERE clan_id = 7001`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got == want || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got != want {
		t.Fatalf("stored reputation = %d, want %d", got, want)
	}
}

// TestClanReputationRewardFromScript plays Q508's reward branch on a
// member of a level 5 clan with its leader online: the clan level reads 5;
// the reputation add shows every online member the clan header; the member
// then gets system message 1777 with the points; the header broadcast
// shows it to every member again. The score is stored. The player's own
// social action reaches the player and the leader beside it.
func TestClanReputationRewardFromScript(t *testing.T) {
	t.Parallel()
	srv, member := bootJournal(t, clanOf(t, 5))
	enterWorld(t, srv)
	chief := enterChief(t, srv)

	runOps(t, srv, member, q001, func(_ *script.Script, p *script.Player) {
		if got := p.ClanLevel(); got != 5 {
			t.Errorf("ClanLevel() = %d, want 5", got)
		}
		p.AddClanReputation(50)
		p.SystemMessageNumbers(1777, 50)
		p.BroadcastClanInfo()
		p.SocialAction(3)
	})
	own := framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSocialAction)
	if want := []byte{serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSocialAction}; !slices.Equal(opcodesOf(own), want) {
		t.Fatalf("member frames %x, want %x", opcodesOf(own), want)
	}
	if r := own[1].r; r.ReadInt32() != 1777 || r.ReadInt32() != 1 || r.ReadInt32() != serverpackets.SystemMessageParamNumber || r.ReadInt32() != 50 {
		t.Error("system message is not 1777 with the number 50")
	}
	if r := own[3].r; r.ReadInt32() != member || r.ReadInt32() != 3 {
		t.Error("SocialAction is not the member's action 3")
	}
	// The leader gets the clan frames on its own queue and the member's
	// social action from the member's, so only their counts are pinned.
	theirs := framesOf(srv.ReadQueued(t, chief), serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSocialAction)
	counts := map[byte]int{}
	for _, f := range theirs {
		counts[f.opcode]++
	}
	if counts[serverpackets.OpcodePledgeShowInfoUpdate] != 2 || counts[serverpackets.OpcodeSystemMessage] != 0 || counts[serverpackets.OpcodeSocialAction] != 1 {
		t.Fatalf("leader frames %x, want two clan headers and one social action", opcodesOf(theirs))
	}
	clanReputation(t, srv, 150)
}

// TestClanReputationBelowLevel5: a clan below level 5 gains no reputation
// and nobody is shown the add, while the message and the header broadcast
// still go out.
func TestClanReputationBelowLevel5(t *testing.T) {
	t.Parallel()
	srv, member := bootJournal(t, clanOf(t, 4))
	enterWorld(t, srv)
	runOps(t, srv, member, q001, func(_ *script.Script, p *script.Player) {
		if got := p.ClanLevel(); got != 4 {
			t.Errorf("ClanLevel() = %d, want 4", got)
		}
		p.AddClanReputation(50)
		p.SystemMessageNumbers(1777, 50)
		p.BroadcastClanInfo()
	})
	own := framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage)
	if want := []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowInfoUpdate}; !slices.Equal(opcodesOf(own), want) {
		t.Fatalf("member frames %x, want %x", opcodesOf(own), want)
	}
	clanReputation(t, srv, 100)
}

// TestClanOpsOfClanlessPlayerAbort: a player in no clan aborts the
// invocation at the clan level or the reputation add, as the reference
// throws on a null clan.
func TestClanOpsOfClanlessPlayerAbort(t *testing.T) {
	t.Parallel()
	loner, lonerID := bootJournal(t)
	enterWorld(t, loner)
	if loner.RunScript(t, lonerID, q001, func(_ *script.Script, p *script.Player) { p.ClanLevel() }) {
		t.Error("ClanLevel() of a clanless player returned")
	}
	if loner.RunScript(t, lonerID, q001, func(_ *script.Script, p *script.Player) { p.AddClanReputation(10) }) {
		t.Error("AddClanReputation() of a clanless player returned")
	}
}
