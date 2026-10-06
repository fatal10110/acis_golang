package quest

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// partyRange is the party range the harness configures the helpers with.
const partyRange = 1500

// npcsAround spawns two NPCs east of objID: one just inside the party
// range, one on its edge, which is outside.
func npcsAround(t *testing.T, srv *gameservertest.Server, objID int32) (near, edge *script.NPC) {
	t.Helper()
	x, y, z := srv.PlayerPosition(t, objID)
	spawn := func(dx int) *script.NPC {
		h := srv.SpawnHostileNPCAt(t, location.Location{X: x + dx, Y: y, Z: z})
		if nx, ny, nz := h.Position(); nx != x+dx || ny != y || nz != z {
			t.Fatalf("NPC placed at %d,%d,%d, want %d,%d,%d", nx, ny, nz, x+dx, y, z)
		}
		return script.NPCOf(h)
	}
	return spawn(partyRange - 1), spawn(partyRange)
}

// condOf returns a state's condition, -1 for no state.
func condOf(st *script.QuestState) int32 {
	if st == nil {
		return -1
	}
	return st.Cond()
}

// TestQuestStateChecks checks a player's Q001 state by condition, variable
// and status: each check returns the state only when it matches and the
// player stands strictly within the party range of the NPC, and nothing for
// a missing player, NPC or state.
func TestQuestStateChecks(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t)
	insertJournal(t, srv.DB,
		journalRow{objID, q001, "<state>", val("STARTED")},
		journalRow{objID, q001, "<cond>", val("2")},
		journalRow{objID, q001, "ex", val("Yes")},
	)
	enterWorld(t, srv)
	near, edge := npcsAround(t, srv, objID)

	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		for _, tc := range []struct {
			name string
			got  *script.QuestState
			want bool
		}{
			{"cond near", sc.CheckPlayerCondition(p, near, 2), true},
			{"cond other", sc.CheckPlayerCondition(p, near, 1), false},
			{"cond edge", sc.CheckPlayerCondition(p, edge, 2), false},
			{"cond no npc", sc.CheckPlayerCondition(p, nil, 2), false},
			{"cond no player", sc.CheckPlayerCondition(nil, near, 2), false},
			{"var any case", sc.CheckPlayerVariable(p, near, "ex", "yES"), true},
			{"var other", sc.CheckPlayerVariable(p, near, "ex", "No"), false},
			{"var missing", sc.CheckPlayerVariable(p, near, "gone", "Yes"), false},
			{"var edge", sc.CheckPlayerVariable(p, edge, "ex", "Yes"), false},
			{"state started", sc.CheckPlayerState(p, near, questlog.StatusStarted), true},
			{"state completed", sc.CheckPlayerState(p, near, questlog.StatusCompleted), false},
			{"state edge", sc.CheckPlayerState(p, edge, questlog.StatusStarted), false},
		} {
			if (tc.got != nil) != tc.want {
				t.Errorf("%s: state %v, want one: %v", tc.name, tc.got, tc.want)
			}
		}
		if st := sc.CheckPlayerCondition(p, near, 2); st.Cond() != 2 || st.Player() == nil {
			t.Errorf("the state found has cond %d, player %v", st.Cond(), st.Player())
		}
		// Alone, the party lookups see the player only.
		if got := sc.PartyMembers(p, near, script.ByCond(2)); len(got) != 1 || got[0].Cond() != 2 {
			t.Errorf("party members alone = %v", got)
		}
		if got := sc.PartyMembers(p, edge, script.ByCond(2)); len(got) != 0 {
			t.Errorf("party members alone, out of range = %v", got)
		}
		if got := sc.PartyMembers(nil, near, script.ByCond(2)); got != nil {
			t.Errorf("party members of no player = %v", got)
		}
	})
	if !srv.RunScript(t, objID, "Q002_WhatWomenWant", func(sc *script.Script, p *script.Player) {
		if st := sc.CheckPlayerState(p, near, questlog.StatusStarted); st != nil {
			t.Error("a quest the player has no state in matched")
		}
	}) {
		t.Fatal("the check of a quest without state panicked")
	}
}

// TestQuestCheckOfABadConditionPanics stores a condition that is no
// integer: the condition check panics, as the reference throws, so the
// invocation aborts there and is logged.
func TestQuestCheckOfABadConditionPanics(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t, gameservertest.WithCapturedLog())
	insertJournal(t, srv.DB,
		journalRow{objID, q001, "<state>", val("STARTED")},
		journalRow{objID, q001, "<cond>", val("two")},
	)
	enterWorld(t, srv)
	near, _ := npcsAround(t, srv, objID)
	reached := false
	if srv.RunScript(t, objID, q001, func(sc *script.Script, p *script.Player) {
		sc.CheckPlayerCondition(p, near, 2)
		reached = true
	}) {
		t.Fatal("the check of a bad condition returned")
	}
	if reached {
		t.Fatal("the invocation went on past the panic")
	}
	if !strings.Contains(srv.LogText(), "script: hook panicked") {
		t.Fatal("the panic was not logged")
	}
}

// partyPair boots Leader and Mate, both in the world at the same spot with
// a Q001 state of the given conditions whose variable who names them, and
// puts them in one party, Leader
// leading.
func partyPair(t *testing.T, leaderCond, mateCond string, opts ...gameservertest.Option) (srv *gameservertest.Server, leader, mate int32, mateClient *testsupport.ScriptedClient) {
	t.Helper()
	srv, leader = bootJournal(t, opts...)
	mate = srv.SeedCharacterFor(t, "mate", "Mate", 1, 0).ID
	for id, who := range map[int32]string{leader: "leader", mate: "mate"} {
		cond := leaderCond
		if id == mate {
			cond = mateCond
		}
		insertJournal(t, srv.DB,
			journalRow{id, q001, "<state>", val("STARTED")},
			journalRow{id, q001, "<cond>", val(cond)},
			journalRow{id, q001, "who", val(who)},
		)
	}
	enterWorld(t, srv)
	mateClient = srv.DialClient(t, "mate", 1)
	mateClient.Send(encodeRequestGameStart(0))
	readUntil(t, mateClient, serverpackets.OpcodeCharSelected)
	mateClient.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	srv.ReadQueued(t, mateClient)
	srv.ReadQueued(t, srv.Client)

	srv.Client.Send(encodeJoinParty("Mate"))
	srv.ReadQueued(t, mateClient)
	mateClient.Send(encodeAnswerJoinParty(1))
	srv.ReadQueued(t, mateClient)
	srv.ReadQueued(t, srv.Client)
	return srv, leader, mate, mateClient
}

// TestPartyMemberLookups looks up the party's Q001 states: the members
// whose state matches and who stand within the party range of the NPC, in
// party order, and one of them at random through the script random source.
func TestPartyMemberLookups(t *testing.T) {
	t.Parallel()
	rs := &rolls{}
	srv, leader, mate, _ := partyPair(t, "2", "2", gameservertest.WithScriptRand(rs.intn))
	near, _ := npcsAround(t, srv, leader)
	owners := func(states []*script.QuestState) []string {
		var out []string
		for _, st := range states {
			who, _ := st.Get("who")
			out = append(out, who)
		}
		return out
	}
	rs.script([]int{1})
	run(t, srv, leader, func(sc *script.Script, p *script.Player) {
		if got := owners(sc.PartyMembers(p, near, script.ByCond(2))); !slices.Equal(got, []string{"leader", "mate"}) {
			t.Errorf("members at cond 2 = %v", got)
		}
		if got := owners(sc.PartyMembersState(p, near, questlog.StatusStarted)); !slices.Equal(got, []string{"leader", "mate"}) {
			t.Errorf("members started = %v", got)
		}
		if got := owners(sc.PartyMembers(p, near, script.ByCond(1))); len(got) != 0 {
			t.Errorf("members at cond 1 = %v", got)
		}
		if got := owners(sc.PartyMembers(p, near, script.ByVar("WHO", "Mate"))); len(got) != 0 {
			t.Errorf("members by a variable they lack = %v", got)
		}
		if got := owners(sc.PartyMembers(p, near, script.ByVar("who", "MATE"))); !slices.Equal(got, []string{"mate"}) {
			t.Errorf("members by variable = %v", got)
		}
		if st := sc.RandomPartyMember(p, near, script.ByCond(2)); st == nil || !slices.Equal(owners([]*script.QuestState{st}), []string{"mate"}) {
			t.Errorf("random member = %v", st)
		}
		if st := sc.RandomPartyMember(p, near, script.ByCond(1)); st != nil {
			t.Errorf("random member of none = %v", st)
		}
	})
	if got := rs.taken(); !slices.Equal(got, []string{"2:1"}) {
		t.Fatalf("draws = %v, want one of two", got)
	}

	// The mate walks out of range: only the leader is left.
	srv.RunQuest(t, mate, q001, func(_ *script.Quests, c *player.Character, _ *script.Script) {
		x, y, z := c.Position()
		c.SyncPosition(location.Location{X: x - partyRange, Y: y, Z: z})
	})
	run(t, srv, leader, func(sc *script.Script, p *script.Player) {
		if got := owners(sc.PartyMembers(p, near, script.ByCond(2))); !slices.Equal(got, []string{"leader"}) {
			t.Errorf("members in range = %v", got)
		}
	})
}

// chiefClan seeds a clan led by Chief, a copy of the boot character on
// account chief, with the boot character as a member.
func chiefClan(t *testing.T) gameservertest.Option {
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
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) VALUES (7001, 'Chiefs', 1, 0, 900001)`,
			`DROP TEMPORARY TABLE chief`,
		} {
			if _, err := conn.ExecContext(ctx, q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
	})
}

// TestClanLeaderLookups looks up the Q001 state of the clan leader: none
// while the leader is offline, the leader's once it is in the world, the
// leader's own when the leader asks, and with an NPC only while both stand
// within the party range of it.
func TestClanLeaderLookups(t *testing.T) {
	t.Parallel()
	srv, member := bootJournal(t, chiefClan(t))
	const chief = 900001
	insertJournal(t, srv.DB,
		journalRow{member, q001, "<state>", val("STARTED")},
		journalRow{member, q001, "<cond>", val("1")},
		journalRow{chief, q001, "<state>", val("STARTED")},
		journalRow{chief, q001, "<cond>", val("3")},
		journalRow{chief, q001, "ex", val("Lead")},
	)
	enterWorld(t, srv)
	near, _ := npcsAround(t, srv, member)

	run(t, srv, member, func(sc *script.Script, p *script.Player) {
		if st := sc.ClanLeaderQuestState(p, nil); st != nil {
			t.Errorf("an offline leader's state = %v", st)
		}
	})

	chiefClient := srv.DialClient(t, "chief", 1)
	chiefClient.Send(encodeRequestGameStart(0))
	readUntil(t, chiefClient, serverpackets.OpcodeCharSelected)
	chiefClient.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	srv.ReadQueued(t, chiefClient)

	run(t, srv, member, func(sc *script.Script, p *script.Player) {
		for _, tc := range []struct {
			name string
			got  *script.QuestState
			want int32
		}{
			{"leader no npc", sc.ClanLeaderQuestState(p, nil), 3},
			{"leader near", sc.ClanLeaderQuestState(p, near), 3},
			{"leader cond", sc.CheckClanLeaderCondition(p, near, 3), 3},
			{"leader other cond", sc.CheckClanLeaderCondition(p, near, 1), -1},
			{"leader var", sc.CheckClanLeaderVariable(p, near, "ex", "LEAD"), 3},
			{"leader other var", sc.CheckClanLeaderVariable(p, near, "ex", "Follow"), -1},
			{"leader state", sc.CheckClanLeaderState(p, near, questlog.StatusStarted), 3},
			{"leader other state", sc.CheckClanLeaderState(p, near, questlog.StatusCompleted), -1},
			{"no player", sc.ClanLeaderQuestState(nil, near), -1},
		} {
			if got := condOf(tc.got); got != tc.want {
				t.Errorf("%s: cond %d, want %d", tc.name, got, tc.want)
			}
		}
	})
	if !srv.RunScript(t, chief, q001, func(sc *script.Script, p *script.Player) {
		if got := condOf(sc.ClanLeaderQuestState(p, near)); got != 3 {
			t.Errorf("the leader's own state: cond %d", got)
		}
	}) {
		t.Fatal("the leader's lookup panicked")
	}

	// The leader walks out of range of the NPC.
	srv.RunQuest(t, chief, q001, func(_ *script.Quests, c *player.Character, _ *script.Script) {
		x, y, z := c.Position()
		c.SyncPosition(location.Location{X: x - partyRange, Y: y, Z: z})
	})
	run(t, srv, member, func(sc *script.Script, p *script.Player) {
		if got := condOf(sc.ClanLeaderQuestState(p, near)); got != -1 {
			t.Errorf("an out-of-range leader's state: cond %d", got)
		}
		if got := condOf(sc.ClanLeaderQuestState(p, nil)); got != 3 {
			t.Errorf("the leader's state with no NPC: cond %d", got)
		}
	})
}

func encodeJoinParty(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	w.WriteString(name)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeAnswerJoinParty(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	w.WriteInt32(response)
	return w.Bytes()
}
