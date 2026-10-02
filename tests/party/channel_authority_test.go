package party

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Command channel authority (CommandChannel.checkAuthority): the leader of
// a level 5 clan holding Clan Imperium (skill 391, a clan skill from clan
// level 5, held from clan rank 4) or a Strategy Guide (item 8871). The
// system messages are 1575 COMMAND_CHANNEL_ONLY_BY_LEVEL_5_CLAN_LEADER_
// PARTY_LEADER, 1592 CANNOT_LONGER_SETUP_COMMAND_CHANNEL, 351
// NOT_ENOUGH_ITEMS and 302 S1_DISAPPEARED (SystemMessageId).
const (
	authorityClanID   = 268435456
	clanImperiumID    = 391
	strategyGuideID   = 8871
	msgOnlyLevel5     = 1575
	msgCannotSetup    = 1592
	msgNotEnoughItems = 351
	msgS1Disappeared  = 302
	msgFormed         = 1580
	msgDisbanded      = 1581
	msgJoined         = 1582
	msgDismissed      = 1583
	msgPartyDismissed = 1584
	msgConfirmFrom    = 1529
)

// Extended sub-opcodes of ExOpenMPCC (0x25), ExCloseMPCC (0x26),
// ExAskJoinMPCC (0x27) and ExMPCCPartyInfoUpdate (0x5a).
const (
	subOpenMPCC       = 0x25
	subCloseMPCC      = 0x26
	subAskJoinMPCC    = 0x27
	subPartyInfoMPCCU = 0x5a
)

var shippedSkills = struct {
	once  sync.Once
	defs  *modelskill.Table
	trees *modelskill.Trees
	err   error
}{}

// skillOptions boots the shipped skill definitions and trees, so a clan's
// skills reach its members as they enter the world.
func skillOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	skillsDir := datapack.Path(t, "data", "xml", "skills")
	treesDir := datapack.Path(t, "data", "xml", "skillstrees")
	shippedSkills.once.Do(func() {
		shippedSkills.defs, shippedSkills.err = gamexml.LoadSkillDefinitions(skillsDir, zerolog.Nop())
		if shippedSkills.err == nil {
			shippedSkills.trees, shippedSkills.err = gamexml.LoadSkillTrees(treesDir)
		}
	})
	if shippedSkills.err != nil {
		t.Fatalf("load shipped skills: %v", shippedSkills.err)
	}
	db := sqltest.SharedDB(t)
	return []gameservertest.Option{
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), shippedSkills.defs, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithSkillTrees(shippedSkills.trees),
	}
}

// strategyGuideTemplates is the shared item catalog plus the Strategy
// Guide.
func strategyGuideTemplates() gameservertest.Option {
	templates := append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: strategyGuideID, Name: "Strategy Guide", Kind: item.KindEtcItem, Duration: -1,
		Destroyable: true, EtcItem: &item.EtcItemDetail{},
	})
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// authorityClan stores a clan of level clanLevel the first seat, Leader,
// belongs to: led by Leader when leads is set, else by an offline member.
// skills are the clan skills it knows at level 1.
type authorityClan struct {
	level  int
	leads  bool
	skills []int
}

func (a authorityClan) option(t *testing.T) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		exec := func(query string, args ...any) {
			if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
				t.Fatalf("%s: %v", query, err)
			}
		}
		exec(`UPDATE characters SET clanid = ?, power_grade = 0 WHERE char_name = 'Leader'`, authorityClanID)
		leader := `SELECT ?, 'Strategists', ?, 1000, obj_Id FROM characters WHERE char_name = 'Leader'`
		if !a.leads {
			exec(`INSERT INTO characters (account_name, obj_Id, char_name, level, clanid, power_grade) VALUES ('founder', 900001, 'Founder', 40, ?, 0)`, authorityClanID)
			leader = `SELECT ?, 'Strategists', ?, 1000, obj_Id FROM characters WHERE char_name = 'Founder'`
		}
		exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) `+leader, authorityClanID, a.level)
		for _, id := range a.skills {
			exec(`INSERT INTO clan_skills (clan_id, skill_id, skill_level) VALUES (?,?,1)`, authorityClanID, id)
		}
	})
}

// bootAuthority boots one client per seat as bootGroup does, Leader being
// the first, with Leader's clan stored and guides Strategy Guides (each
// its own item, the guide not stacking) in Leader's inventory before it
// enters the world. It returns the first guide's object id (0 without one).
func bootAuthority(t *testing.T, seats []seat, clan authorityClan, guides int) (*group, int32) {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter(seats[0].name, seats[0].level, 0),
		gameservertest.WithWantChars(1),
		strategyGuideTemplates(),
		clan.option(t),
	}, skillOptions(t)...)
	srv := gameservertest.Boot(t, opts...)
	leaderID := srv.SoleObjectID(t)
	var guideID int32
	for i := range guides {
		id := srv.GiveItem(t, leaderID, strategyGuideID, 1)
		if i == 0 {
			guideID = id
		}
	}
	g := &group{srv: srv, players: []player{{c: srv.Client, id: leaderID, name: seats[0].name}}}
	for i, s := range seats[1:] {
		account := "member" + string(rune('a'+i))
		id := srv.SeedCharacterFor(t, account, s.name, s.level, 0).ID
		g.players = append(g.players, player{c: srv.DialClient(t, account, 1), id: id, name: s.name})
	}
	for _, p := range g.players {
		startInWorld(t, p.c)
	}
	g.quiet(t)
	return g, guideID
}

// extendedSub is an extended frame's sub-opcode, -1 for any other frame.
func extendedSub(frame []byte) int {
	if frame[0] != serverpackets.OpcodeExtended || len(frame) < 3 {
		return -1
	}
	return int(wire.NewReader(frame[1:]).ReadUint16())
}

// event names a frame the channel flow sends: a system message's id, an
// extended frame's sub-opcode plus 0x10000, an inventory update as
// evInventoryUpdate; any other frame is dropped.
func events(t *testing.T, frames [][]byte) []int {
	t.Helper()
	var out []int
	for _, f := range frames {
		switch {
		case f[0] == serverpackets.OpcodeSystemMessage:
			out = append(out, int(wire.NewReader(f[1:]).ReadInt32()))
		case f[0] == serverpackets.OpcodeInventoryUpdate:
			out = append(out, evInventoryUpdate)
		case extendedSub(f) >= 0:
			out = append(out, ext(extendedSub(f)))
		}
	}
	return out
}

const evInventoryUpdate = -2

func ext(sub int) int { return 0x10000 + sub }

func assertEvents(t *testing.T, got []int, want []int, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s events = %v, want %v", what, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s events = %v, want %v", what, got, want)
		}
	}
}

// askAndAccept has from invite name's party into the channel: the target
// party's leader to is asked by name and accepts. It returns every
// client's frames after the answer.
func (g *group) askAndAccept(t *testing.T, from, to int) [][][]byte {
	t.Helper()
	g.players[from].c.Send(encodeExtendedName(clientpackets.OpcodeRequestExAskJoinMPCC, g.players[to].name))
	assertSystemMessageText(t, skipPositions(g.players[to].c), msgConfirmFrom, g.players[from].name)
	ask := skipPositions(g.players[to].c)
	if extendedSub(ask) != subAskJoinMPCC {
		t.Fatalf("invitation frame = %x, want ExAskJoinMPCC", ask)
	}
	r := wire.NewReader(ask[3:])
	if name := r.ReadString(); name != g.players[from].name {
		t.Fatalf("ExAskJoinMPCC requester = %q, want %q", name, g.players[from].name)
	}
	g.players[to].c.Send(encodeExtendedInt(clientpackets.OpcodeRequestExAcceptJoinMPCC, 1))
	out := make([][][]byte, len(g.players))
	for i, p := range g.players {
		out[i] = drainFrames(t, p.c)
	}
	return out
}

// assertPartyInfoUpdate decodes the one ExMPCCPartyInfoUpdate in frames.
func assertPartyInfoUpdate(t *testing.T, frames [][]byte, name string, leaderID, count, mode int32) {
	t.Helper()
	for _, f := range frames {
		if extendedSub(f) != subPartyInfoMPCCU {
			continue
		}
		r := wire.NewReader(f[3:])
		gotName, gotID, gotCount, gotMode := r.ReadString(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		if gotName != name || gotID != leaderID || gotCount != count || gotMode != mode || r.Remaining() != 0 {
			t.Fatalf("ExMPCCPartyInfoUpdate = %q %d %d %d (%d left), want %q %d %d %d", gotName, gotID, gotCount, gotMode, r.Remaining(), name, leaderID, count, mode)
		}
		return
	}
	t.Fatal("no ExMPCCPartyInfoUpdate")
}

// TestChannelFormsWithClanImperium runs a command channel end to end under
// a level 5 clan leader holding Clan Imperium: formation, a third party
// joining, its dismissal, and the leading party dispersing, which
// disbands the channel. Nothing is taken from the leader.
func TestChannelFormsWithClanImperium(t *testing.T) {
	g, _ := bootAuthority(t, []seat{{"Leader", 40}, {"Member", 30}, {"Other", 40}, {"Fourth", 50}, {"Fifth", 30}, {"Sixth", 30}},
		authorityClan{level: 5, leads: true, skills: []int{clanImperiumID}}, 0)
	g.invite(t, 0, 1, 0)
	g.invite(t, 2, 3, 0)
	g.invite(t, 4, 5, 0)

	formed := g.askAndAccept(t, 0, 2)
	for i := range 2 {
		assertEvents(t, events(t, formed[i]), []int{msgFormed, ext(subOpenMPCC)}, g.players[i].name+" formation")
	}
	for i := 2; i < 4; i++ {
		assertEvents(t, events(t, formed[i]), []int{msgJoined, ext(subOpenMPCC)}, g.players[i].name+" formation")
	}
	assertEvents(t, events(t, formed[4]), nil, "Fifth at formation")

	joined := g.askAndAccept(t, 0, 4)
	for i := range 4 {
		assertEvents(t, events(t, joined[i]), []int{ext(subPartyInfoMPCCU)}, g.players[i].name+" third party")
		assertPartyInfoUpdate(t, joined[i], "Fifth", g.players[4].id, 2, 1)
	}
	for i := 4; i < 6; i++ {
		assertEvents(t, events(t, joined[i]), []int{msgJoined, ext(subOpenMPCC)}, g.players[i].name+" third party")
	}

	// Naming any member of a party dismisses its whole party.
	g.players[0].c.Send(encodeExtendedName(clientpackets.OpcodeRequestExOustFromMPCC, "Sixth"))
	for i, p := range g.players {
		frames := drainFrames(t, p.c)
		if i < 4 {
			assertEvents(t, events(t, frames), []int{ext(subPartyInfoMPCCU), msgPartyDismissed}, p.name+" dismissal")
			assertPartyInfoUpdate(t, frames, "Fifth", g.players[4].id, 2, 0)
			continue
		}
		assertEvents(t, events(t, frames), []int{ext(subCloseMPCC), msgDismissed}, p.name+" dismissal")
	}

	// The leading party disperses as its last member leaves: the channel
	// disbands.
	g.players[1].c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	for _, p := range g.players[2:4] {
		got := events(t, drainFrames(t, p.c))
		assertEvents(t, got, []int{ext(subCloseMPCC), msgDisbanded}, p.name+" disbanding")
	}
	g.quiet(t)
	g.players[2].c.Send(encodeExtendedName(clientpackets.OpcodeRequestExOustFromMPCC, "Leader"))
	assertStaticSystemMessage(t, skipPositions(g.players[2].c), serverpackets.SystemMessageInvalidTarget)
	g.players[2].c.Send(encodeExtendedName(clientpackets.OpcodeRequestExOustFromMPCC, "Fourth"))
	assertStaticSystemMessage(t, skipPositions(g.players[2].c), serverpackets.SystemMessageNotAuthorizedToDoThat)
}

// TestChannelFormsWithStrategyGuide forms a channel under a level 5 clan
// leader without Clan Imperium: the guide is only looked at on the
// invitation and destroyed, with its notice and inventory update, ahead of
// the formation.
func TestChannelFormsWithStrategyGuide(t *testing.T) {
	g, guideID := bootAuthority(t, []seat{{"Leader", 40}, {"Member", 30}, {"Other", 40}, {"Fourth", 50}},
		authorityClan{level: 5, leads: true}, 1)
	g.invite(t, 0, 1, 0)
	g.invite(t, 2, 3, 0)

	formed := g.askAndAccept(t, 0, 2)
	assertEvents(t, events(t, formed[0]), []int{msgS1Disappeared, msgFormed, ext(subOpenMPCC)}, "Leader formation")
	for _, f := range formed[0] {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != msgS1Disappeared {
			continue
		}
		if n, typ, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); n != 1 || typ != serverpackets.SystemMessageParamItemName || id != strategyGuideID {
			t.Fatalf("S1_DISAPPEARED params = %d %d %d, want the Strategy Guide", n, typ, id)
		}
	}
	assertEvents(t, events(t, formed[1]), []int{msgFormed, ext(subOpenMPCC)}, "Member formation")
	for i := 2; i < 4; i++ {
		assertEvents(t, events(t, formed[i]), []int{msgJoined, ext(subOpenMPCC)}, g.players[i].name+" formation")
	}
	// The guide's removal reaches the leader with the next inventory
	// update.
	g.srv.InventoryUpdates.Tick()
	assertEvents(t, events(t, drainFrames(t, g.players[0].c)), []int{evInventoryUpdate}, "Leader inventory update")
	if inv := g.srv.PlayerInventory(t, g.players[0].id); inv.ItemByObjectID(guideID) != nil {
		t.Fatal("Strategy Guide still held after the formation")
	}
	g.srv.FlushItems(t)
	var held int
	if err := g.srv.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM items WHERE object_id = ?`, guideID).Scan(&held); err != nil {
		t.Fatal(err)
	}
	if held != 0 {
		t.Fatalf("Strategy Guide rows after the formation = %d, want 0", held)
	}
}

// heldGuides counts the Strategy Guides in the leader's live inventory.
func heldGuides(t *testing.T, g *group) int {
	t.Helper()
	return g.srv.PlayerInventory(t, g.players[0].id).ItemCount(strategyGuideID, -1, false)
}

// TestChannelGuidePaidOnlyOnFormation: a guide-only leader holding two
// guides pays one to form the channel; inviting a further party needs a
// guide in hand but adding that party to the standing channel takes
// nothing. A leader whose last guide went on the formation can invite no
// one more.
func TestChannelGuidePaidOnlyOnFormation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		guides int
	}{
		{"second guide in hand", 2},
		{"last guide spent", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := bootAuthority(t, []seat{{"Leader", 40}, {"Member", 30}, {"Other", 40}, {"Fourth", 50}, {"Fifth", 30}, {"Sixth", 30}},
				authorityClan{level: 5, leads: true}, tc.guides)
			g.invite(t, 0, 1, 0)
			g.invite(t, 2, 3, 0)
			g.invite(t, 4, 5, 0)

			formed := g.askAndAccept(t, 0, 2)
			assertEvents(t, events(t, formed[0]), []int{msgS1Disappeared, msgFormed, ext(subOpenMPCC)}, "Leader formation")
			g.srv.InventoryUpdates.Tick()
			assertEvents(t, events(t, drainFrames(t, g.players[0].c)), []int{evInventoryUpdate}, "Leader formation inventory update")
			if n := heldGuides(t, g); n != tc.guides-1 {
				t.Fatalf("guides after the formation = %d, want %d", n, tc.guides-1)
			}
			g.quiet(t)

			if tc.guides == 1 {
				g.players[0].c.Send(encodeExtendedName(clientpackets.OpcodeRequestExAskJoinMPCC, "Fifth"))
				assertStaticSystemMessage(t, skipPositions(g.players[0].c), msgCannotSetup)
				for _, p := range g.players[1:] {
					assertSilent(t, p.c, p.name+" after a refused invitation")
				}
				return
			}

			joined := g.askAndAccept(t, 0, 4)
			assertEvents(t, events(t, joined[0]), []int{ext(subPartyInfoMPCCU)}, "Leader third party")
			for i := 4; i < 6; i++ {
				assertEvents(t, events(t, joined[i]), []int{msgJoined, ext(subOpenMPCC)}, g.players[i].name+" third party")
			}
			g.srv.InventoryUpdates.Tick()
			assertSilent(t, g.players[0].c, "Leader inventory after the third party joins")
			if n := heldGuides(t, g); n != 1 {
				t.Fatalf("guides after the third party joins = %d, want 1", n)
			}
		})
	}
}

// TestChannelGuideGoneByTheAnswer: a guide held at the invitation but
// destroyed before the answer leaves nothing to pay with; the leader is
// told so twice over and no channel forms.
func TestChannelGuideGoneByTheAnswer(t *testing.T) {
	g, guideID := bootAuthority(t, []seat{{"Leader", 40}, {"Member", 30}, {"Other", 40}, {"Fourth", 50}},
		authorityClan{level: 5, leads: true}, 1)
	g.invite(t, 0, 1, 0)
	g.invite(t, 2, 3, 0)

	g.players[0].c.Send(encodeExtendedName(clientpackets.OpcodeRequestExAskJoinMPCC, "Other"))
	assertSystemMessageText(t, skipPositions(g.players[2].c), msgConfirmFrom, "Leader")
	g.quiet(t)
	// The leader's pending request keeps it from destroying the guide
	// itself, so it goes server-side, on the leader's queue.
	done := make(chan struct{})
	if !g.srv.PlayerQueue(t, g.players[0].id).Post(func() {
		defer close(done)
		g.srv.PlayerInventory(t, g.players[0].id).DestroyByObjectID(guideID, 1)
	}) {
		t.Fatal("leader queue closed")
	}
	<-done
	g.srv.InventoryUpdates.Tick()
	g.quiet(t)

	g.players[2].c.Send(encodeExtendedInt(clientpackets.OpcodeRequestExAcceptJoinMPCC, 1))
	assertEvents(t, events(t, drainFrames(t, g.players[0].c)), []int{msgNotEnoughItems, msgCannotSetup}, "Leader")
	for _, p := range g.players[1:] {
		assertSilent(t, p.c, p.name+" after a refused formation")
	}
}

// TestChannelAuthorityRefusals pins the invitation's authority refusals:
// a level 5 clan leader with neither the skill nor a guide is told it can
// no longer set one up; a member who does not lead the clan, or the leader
// of a level 4 clan even holding a guide, is told only a level 5 clan
// leader may. Nobody is asked.
func TestChannelAuthorityRefusals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		clan   authorityClan
		guides int
		want   int
	}{
		{"no skill, no guide", authorityClan{level: 5, leads: true}, 0, msgCannotSetup},
		{"not the clan leader", authorityClan{level: 5, skills: []int{clanImperiumID}}, 1, msgOnlyLevel5},
		{"level 4 clan", authorityClan{level: 4, leads: true}, 1, msgOnlyLevel5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := bootAuthority(t, []seat{{"Leader", 40}, {"Member", 30}, {"Other", 40}, {"Fourth", 50}}, tc.clan, tc.guides)
			g.invite(t, 0, 1, 0)
			g.invite(t, 2, 3, 0)
			g.players[0].c.Send(encodeExtendedName(clientpackets.OpcodeRequestExAskJoinMPCC, "Other"))
			assertStaticSystemMessage(t, skipPositions(g.players[0].c), tc.want)
			for _, p := range g.players[1:] {
				assertSilent(t, p.c, p.name+" after a refused invitation")
			}
		})
	}
}
