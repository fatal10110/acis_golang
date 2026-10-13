package quest

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The player handle operations the freeze of the script-facing API adds
// (#3748), driven through a booted world as a hook calls them.

// systemMessageIDs returns the id of each SystemMessage among frames.
func systemMessageIDs(frames [][]byte) []int32 {
	var out []int32
	for _, f := range framesOf(frames, serverpackets.OpcodeSystemMessage) {
		out = append(out, f.r.ReadInt32())
	}
	return out
}

// TestScriptPlayerStandingReads pins the player's standing reads:
// Player.getClanId, isHero, getMountType and isFlying (mount type 2),
// isOverweight, getInventory().getItemIdFrom(RHAND) and
// getActiveWeaponInstance() (0 / null for an empty right hand), and
// getAttackType (the class's fists, then the equipped weapon's type), with
// isRiding (mount type 1) false.
func TestScriptPlayerStandingReads(t *testing.T) {
	t.Parallel()
	class := gameservertest.ClassTemplate()
	class.FistsItemID = x1Fists
	srv, objID := bootJournal(t, x1ItemTemplates(), gameservertest.WithClassTemplates(class), clanOf(t, 3))
	sword := srv.GiveItem(t, objID, swordID, 1)
	enterWorld(t, srv)

	read := func() (rightHand int32, attack item.WeaponType) {
		runOps(t, srv, objID, q001, func(_ *script.Script, p *script.Player) {
			rightHand, attack = p.RightHandItemID(), script.AttackType(p)
		})
		return rightHand, attack
	}
	runOps(t, srv, objID, q001, func(_ *script.Script, p *script.Player) {
		if got := p.ClanID(); got != 7001 {
			t.Errorf("ClanID() = %d, want 7001", got)
		}
		if p.IsHero() || p.MountType() != 0 || p.Flying() || script.Riding(p) || p.Overweight() {
			t.Errorf("hero %v, mount %d, flying %v, riding %v, overweight %v; want none", p.IsHero(), p.MountType(), p.Flying(), script.Riding(p), p.Overweight())
		}
	})
	if hand, attack := read(); hand != 0 || attack != item.WeaponFist {
		t.Fatalf("empty right hand: RightHandItemID %d, AttackType %v; want 0, the fists' %v", hand, attack, item.WeaponFist)
	}

	srv.Client.Send(encodeUseItem(sword))
	srv.AdvanceUntil(t, "the sword is in the right hand", func() bool {
		hand, _ := read()
		return hand == swordID
	})
	tmpl, ok := gameservertest.ItemTemplates().Get(swordID)
	if !ok || tmpl.Weapon == nil {
		t.Fatal("fixture: no sword template")
	}
	if _, attack := read(); attack != tmpl.Weapon.Type {
		t.Fatalf("AttackType with the sword = %v, want %v", attack, tmpl.Weapon.Type)
	}
}

// TestScriptPlayerClanless: a player in no clan reads clan id 0.
func TestScriptPlayerClanless(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t)
	runOps(t, srv, objID, q001, func(_ *script.Script, p *script.Player) {
		if got := p.ClanID(); got != 0 {
			t.Errorf("ClanID() = %d, want 0", got)
		}
	})
}

// TestScriptMakeNoble pins Player.setNoble(true, true) (Q247): the player
// gets its skill list, then its UserInfo, reads as a noblesse, and the
// status is stored at once.
func TestScriptMakeNoble(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t)
	srv.ReadQueued(t, srv.Client)
	run(t, srv, objID, func(_ *script.Script, p *script.Player) {
		if p.IsNoble() {
			t.Fatal("fixture: the player is already a noblesse")
		}
		p.MakeNoble()
		if !p.IsNoble() {
			t.Error("IsNoble() after MakeNoble = false")
		}
	})
	got := opcodesOf(framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodeSkillList, serverpackets.OpcodeUserInfo))
	if want := []byte{serverpackets.OpcodeSkillList, serverpackets.OpcodeUserInfo}; !slices.Equal(got, want) {
		t.Fatalf("frames %x, want SkillList then UserInfo %x", got, want)
	}
	srv.FlushPersistence(t)
	var noble int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT nobless FROM characters WHERE obj_Id = ?", objID).Scan(&noble); err != nil {
		t.Fatal(err)
	}
	if noble != 1 {
		t.Fatalf("stored nobless = %d, want 1", noble)
	}
}

// TestScriptRefreshHennaList pins Player.refreshHennaList (the class change
// features): the player alone gets one HennaInfo.
func TestScriptRefreshHennaList(t *testing.T) {
	t.Parallel()
	srv, leader, _, mateClient := partyPair(t, "1", "1")
	srv.ReadQueued(t, srv.Client)
	srv.ReadQueued(t, mateClient)
	run(t, srv, leader, func(_ *script.Script, p *script.Player) { p.RefreshHennaList() })
	if got := framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodeHennaInfo); len(got) != 1 {
		t.Fatalf("player got %d HennaInfo, want 1", len(got))
	}
	if got := framesOf(srv.ReadQueued(t, mateClient), serverpackets.OpcodeHennaInfo); len(got) != 0 {
		t.Fatalf("mate got %d HennaInfo, want none", len(got))
	}
}

// TestScriptFactionAndDungeonFlags pins Player.setAllianceWithVarkaKetra
// (the alliance quests and support features: positive Ketra, negative
// Varka, 0 neither, read back at once) and setIsIn7sDungeon (the Oracle
// teleporters), neither of which sends anything.
func TestScriptFactionAndDungeonFlags(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t)
	srv.ReadQueued(t, srv.Client)
	c := onlineCharacter(t, srv, objID)
	for _, tc := range []struct {
		level        int32
		ketra, varka bool
	}{{3, true, false}, {-5, false, true}, {0, false, false}} {
		run(t, srv, objID, func(_ *script.Script, p *script.Player) {
			p.SetVarkaKetraAlliance(tc.level)
			if p.VarkaKetraAlliance() != tc.level || p.AlliedWithKetra() != tc.ketra || p.AlliedWithVarka() != tc.varka {
				t.Errorf("after SetVarkaKetraAlliance(%d): level %d, Ketra %v, Varka %v", tc.level, p.VarkaKetraAlliance(), p.AlliedWithKetra(), p.AlliedWithVarka())
			}
		})
	}
	for _, in := range []bool{true, false} {
		run(t, srv, objID, func(_ *script.Script, p *script.Player) { p.SetIn7sDungeon(in) })
		if c.In7sDungeon() != in {
			t.Errorf("after SetIn7sDungeon(%v) the player reads %v", in, c.In7sDungeon())
		}
	}
	if got := srv.ReadQueued(t, srv.Client); len(got) != 0 {
		t.Fatalf("the flags sent %d frames, want none", len(got))
	}
}

// TestScriptDestroyItemsNotify pins Player.destroyItemByItemId(id, count,
// sendMessage): with the message, units destroyed are named (S2_S1 for
// several, S1 for one), too few held is NOT_ENOUGH_ITEMS and destroys
// nothing, and adena pays as reduceAdena does; without it nothing is said.
func TestScriptDestroyItemsNotify(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t, opsItemTemplates())
	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		sc.GiveItems(p, dropItem, 6)
		sc.GiveItems(p, item.AdenaID, 100)
	})
	srv.ReadQueued(t, srv.Client)

	for _, tc := range []struct {
		id, count int32
		notify    bool
		ok        bool
		messages  []int32
		left      int
	}{
		{dropItem, 2, true, true, []int32{serverpackets.SystemMessageS2S1Disappeared}, 4},
		{dropItem, 1, true, true, []int32{serverpackets.SystemMessageS1Disappeared}, 3},
		{dropItem, 9, true, false, []int32{serverpackets.SystemMessageNotEnoughItems}, 3},
		{dropItem, 1, false, true, nil, 2},
		{dropItem, 9, false, false, nil, 2},
		{item.AdenaID, 30, true, true, []int32{serverpackets.SystemMessageS1DisappearedAdena}, 70},
		{item.AdenaID, 500, true, false, []int32{serverpackets.SystemMessageYouNotEnoughAdena}, 70},
	} {
		var ok bool
		run(t, srv, objID, func(sc *script.Script, p *script.Player) { ok = sc.DestroyItems(p, tc.id, tc.count, tc.notify) })
		got := systemMessageIDs(srv.ReadQueued(t, srv.Client))
		left := srv.PlayerItemCount(t, objID, tc.id)
		if ok != tc.ok || !slices.Equal(got, tc.messages) || left != tc.left {
			t.Errorf("DestroyItems(%d, %d, %v) = %v, messages %v, %d left; want %v, %v, %d left",
				tc.id, tc.count, tc.notify, ok, got, left, tc.ok, tc.messages, tc.left)
		}
	}
}

// TestScriptMemoStatesAndQuestReads pins Quest.getMemoStateEx and
// setMemoStateEx (the dimensional rift quest), with the slot-0 forms: a
// slot reads -1 while unset, is kept as the quest variable memoState-<slot>,
// and a quest the player has no state in is left alone by a set and
// aborts a read. QuestList.getAllQuests(false).size() counts the started
// real quests; QuestState.getBool reads "true" in any case, else false;
// Item.isStackable reads the template.
func TestScriptMemoStatesAndQuestReads(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t, opsItemTemplates())
	insertJournal(t, srv.DB,
		journalRow{objID, q001, "<state>", val("STARTED")},
		journalRow{objID, "Q002_WhatWomenWant", "<state>", val("COMPLETED")},
		journalRow{objID, "Q003_WillTheSealBeBroken", "<state>", val("STARTED")},
		journalRow{objID, "NoblesseTeleporter", "<state>", val("STARTED")},
	)
	enterWorld(t, srv)

	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		if got := p.StartedQuests(); got != 2 {
			t.Errorf("StartedQuests() = %d, want 2 (Q001, Q003)", got)
		}
		if got := sc.MemoState(p, 1); got != -1 {
			t.Errorf("MemoState of an unset slot = %d, want -1", got)
		}
		sc.SetMemoState(p, 1, 7)
		sc.SetMemoStateEx(p, 1, 1, -1)
		sc.SetMemoStateEx(p, 6, 0, 9)
		if got, ex := sc.MemoState(p, 1), sc.MemoStateEx(p, 1, 1); got != 7 || ex != -1 {
			t.Errorf("MemoState %d, MemoStateEx(1) %d; want 7, -1", got, ex)
		}
		st := sc.QuestState(p, q001)
		for _, v := range []struct {
			value string
			want  bool
		}{{"TRUE", true}, {"true", true}, {"yes", false}, {"1", false}} {
			st.Set("flag", v.value)
			if got := st.GetBool("flag"); got != v.want {
				t.Errorf("GetBool of %q = %v, want %v", v.value, got, v.want)
			}
		}
		if st.GetBool("unset") {
			t.Error("GetBool of an unset variable = true")
		}
		if !sc.ItemStackable(dropItem) || sc.ItemStackable(swordID) {
			t.Errorf("ItemStackable: drop item %v, sword %v; want true, false", sc.ItemStackable(dropItem), sc.ItemStackable(swordID))
		}
	})
	vars := srv.QuestVars(t, objID, q001)
	if vars["memoState-0"] != "7" || vars["memoState-1"] != "-1" {
		t.Fatalf("Q001 variables %v, want memoState-0=7, memoState-1=-1", vars)
	}
	if vars := srv.QuestVars(t, objID, "Q006_StepIntoTheFuture"); vars != nil {
		t.Fatalf("a set on a quest the player has no state in made one: %v", vars)
	}
	if srv.RunScript(t, objID, q001, func(sc *script.Script, p *script.Player) { sc.MemoState(p, 6) }) {
		t.Fatal("MemoState of a quest the player has no state in returned")
	}
	if srv.RunScript(t, objID, q001, func(sc *script.Script, _ *script.Player) { sc.ItemStackable(424242) }) {
		t.Fatal("ItemStackable of an item with no template returned")
	}
}

// TestScriptFirstItemCount pins ItemContainer.getItemByItemId(id).getCount():
// the count of the first instance, which a player holding none aborts on.
func TestScriptFirstItemCount(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t, opsItemTemplates())
	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		sc.GiveItems(p, dropItem, 5)
		if got := p.FirstItemCount(dropItem); got != 5 {
			t.Errorf("FirstItemCount = %d, want 5", got)
		}
	})
	if srv.RunScript(t, objID, q001, func(_ *script.Script, p *script.Player) { p.FirstItemCount(soulshotNoGrade) }) {
		t.Fatal("FirstItemCount of an item the player does not hold returned")
	}
}

// TestScriptElapsedTicks pins Quest.getElapsedTicks: the seconds of server
// uptime past a tick taken from that same count.
func TestScriptElapsedTicks(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t)
	run(t, srv, objID, func(sc *script.Script, _ *script.Player) {
		now := sc.ElapsedTicks(0)
		if now < 0 {
			t.Fatalf("uptime ticks = %d, want at least 0", now)
		}
		if got := sc.ElapsedTicks(now - 900); got < 900 || got > 901 {
			t.Errorf("ElapsedTicks(now-900) = %d, want 900", got)
		}
	})
}

// TestScriptPartyLeaderReads pins AbstractGroup.getLeader,
// getLeaderObjectId and isLeader and Party.getMembersCount, read on a
// member: the leader first, both members counted.
func TestScriptPartyLeaderReads(t *testing.T) {
	t.Parallel()
	srv, leader, mate, _ := partyPair(t, "1", "1")
	for _, who := range []int32{leader, mate} {
		run(t, srv, who, func(_ *script.Script, p *script.Player) {
			l := p.PartyLeader()
			if l == nil || l.ObjectID() != leader {
				t.Errorf("%d: PartyLeader() = %v, want %d", who, l, leader)
			}
			if p.IsPartyLeader() != (who == leader) {
				t.Errorf("%d: IsPartyLeader() = %v", who, p.IsPartyLeader())
			}
			if got := len(p.PartyMembers()); got != 2 {
				t.Errorf("%d: %d party members, want 2", who, got)
			}
		})
	}
}

// TestScriptPartyLeaderAlone: a player in no party has no party leader,
// and asking whether it leads its party aborts.
func TestScriptPartyLeaderAlone(t *testing.T) {
	t.Parallel()
	solo, soloID := bootHelpers(t)
	run(t, solo, soloID, func(_ *script.Script, p *script.Player) {
		if p.PartyLeader() != nil {
			t.Error("a player in no party has a party leader")
		}
	})
	if solo.RunScript(t, soloID, q001, func(_ *script.Script, p *script.Player) { p.IsPartyLeader() }) {
		t.Fatal("IsPartyLeader of a player in no party returned")
	}
}

// academyOf is clanOf(t, 3) with Chief as the academy sponsor of Newbie.
func academyOf(t *testing.T) gameservertest.Option {
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
			`UPDATE chief SET obj_Id = 900001, account_name = 'chief', char_name = 'Chief', clanid = 7001, apprentice = (SELECT obj_Id FROM characters WHERE char_name = 'Newbie')`,
			`INSERT INTO characters SELECT * FROM chief`,
			`UPDATE characters SET clanid = 7001, sponsor = 900001 WHERE char_name = 'Newbie'`,
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) VALUES (7001, 'Chiefs', 3, 100, 900001)`,
			`DROP TEMPORARY TABLE chief`,
		} {
			if _, err := conn.ExecContext(ctx, q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
	})
}

// TestScriptAcademyMentors pins Player.getSponsor and getApprentice (the
// member ids) and Quest.getSponsor and getApprentice (the Q118 and Q123
// helpers): the mentor counts once it is a clan member in the world
// strictly within 1500 of the player.
func TestScriptAcademyMentors(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t, academyOf(t))
	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		if p.Sponsor() != 900001 || p.Apprentice() != 0 {
			t.Errorf("Sponsor %d, Apprentice %d; want 900001, 0", p.Sponsor(), p.Apprentice())
		}
		if sc.SponsorInRange(p) {
			t.Error("SponsorInRange with the sponsor out of the world = true")
		}
	})
	enterChief(t, srv)
	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		if !sc.SponsorInRange(p) {
			t.Error("SponsorInRange with the sponsor beside the player = false")
		}
	})
	run(t, srv, 900001, func(sc *script.Script, p *script.Player) {
		if p.Apprentice() != objID {
			t.Errorf("Chief's Apprentice() = %d, want %d", p.Apprentice(), objID)
		}
		if got := sc.ApprenticeInRange(p); got == nil || got.ObjectID() != objID {
			t.Errorf("ApprenticeInRange = %v, want %d", got, objID)
		}
	})
	x, y, z := srv.PlayerPosition(t, objID)
	run(t, srv, 900001, func(_ *script.Script, p *script.Player) { p.InstantTeleportTo(script.Loc{X: x + 1500, Y: y, Z: z}) })
	srv.AdvanceUntil(t, "the sponsor stands 1500 away", func() bool {
		cx, _, _ := srv.PlayerPosition(t, 900001)
		return cx == x+1500
	})
	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		if sc.SponsorInRange(p) {
			t.Error("SponsorInRange with the sponsor 1500 away = true")
		}
	})
}
