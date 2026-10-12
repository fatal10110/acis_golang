package npcs

import (
	"bytes"
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: the showChatWindow and getNpcTalkCond overrides of the castle
// NPCs. A castle NPC answers a member of a clan: under siege while its
// castle's siege runs (CastleMagician: while its battlefield is active),
// else as the owner when that clan owns the castle and holds the type's
// privilege, else as a stranger, as is a clanless talker.
//   - CastleChamberlain (CastleChamberlain.java:663-701): ActionFailed, then
//     chamberlain/no.htm, busy.htm or chamberlain.htm.
//   - CastleBlacksmith (CastleBlacksmith.java:51-99, CP_MANOR_ADMINISTRATION):
//     npcdefault.htm while AllowManor is off, else castleblacksmith-no.htm,
//     -busy.htm or castleblacksmith.htm with %npcname% and %castleid%; the
//     page alone.
//   - CastleMagician (CastleMagician.java:21-102): ActionFailed, then
//     magician-no.htm, magician-busy.htm or magician.htm.
//   - CastleWarehouseKeeper (CastleWarehouseKeeper.java:23-61): ActionFailed,
//     then castlewarehouse-no.htm, -busy.htm or castlewarehouse.htm with
//     %npcname%.
//   - MercenaryManagerNpc (MercenaryManagerNpc.java:78-124, CP_MERCENARIES):
//     mseller002.htm, mseller003.htm, or mseller001[_dawn|_dusk].htm by the
//     Seal of Strife owner; the page alone.
//   - CastleDoorman / Doorman (CastleDoorman.java:45-55, Doorman.java:61-69,
//     CP_ENTRY_EXIT_RIGHTS): doormen/<npcId>.htm, or <npcId>-no.htm for a
//     non-owner, then ActionFailed; a plain Doorman always owns.
//   - ManorManagerNpc (ManorManagerNpc.java:91-103): npcdefault.htm while
//     AllowManor is off, manager-lord.htm for the owning clan's leader, else
//     manager.htm, then ActionFailed.

const (
	castleChamberlainID = 35100
	castleBlacksmithID  = 35098
	castleMagicianID    = 35101
	castleWarehouseID   = 35099
	castleMercenaryID   = 35102
	castleManorID       = 35103
	castleDoormanID     = 35096
	frontierDoormanID   = 30596
	residenceRivalClan  = 0x10000078
	residenceRaidClan   = 0x10000079
	residenceFounderID  = 0x1000007a
	residenceRaiderID   = 0x1000007b
)

// residenceCastles is Gludio listing the castle NPCs.
func residenceCastles(t *testing.T) *castledata.Table {
	t.Helper()
	gludio, err := castledata.NewCastle(castledata.CastleAttrs{
		ID: taxCastleID, Alias: "gludio_castle", Name: "Gludio Castle", CircletID: 6838,
		Tax: residence.Tax{Rate: taxCastleTax},
		NPCs: []int{
			castleChamberlainID, castleBlacksmithID, castleMagicianID, castleWarehouseID,
			castleMercenaryID, castleManorID, castleDoormanID,
		},
	}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err := castledata.NewTable([]*castledata.Castle{gludio})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// residencePages are one page per file the castle NPCs read, each naming
// itself and the placeholders filled.
func residencePages() map[string]string {
	pages := map[string]string{}
	for _, file := range []string{
		"npcdefault.htm",
		"chamberlain/no.htm", "chamberlain/busy.htm", "chamberlain/chamberlain.htm",
		"castleblacksmith/castleblacksmith-no.htm", "castleblacksmith/castleblacksmith-busy.htm", "castleblacksmith/castleblacksmith.htm",
		"castlemagician/magician-no.htm", "castlemagician/magician-busy.htm", "castlemagician/magician.htm",
		"castlewarehouse/castlewarehouse-no.htm", "castlewarehouse/castlewarehouse-busy.htm", "castlewarehouse/castlewarehouse.htm",
		"mercmanager/mseller001.htm", "mercmanager/mseller001_dawn.htm", "mercmanager/mseller001_dusk.htm",
		"mercmanager/mseller002.htm", "mercmanager/mseller003.htm",
		"manormanager/manager.htm", "manormanager/manager-lord.htm",
		"doormen/35096.htm", "doormen/35096-no.htm", "doormen/30596.htm",
	} {
		pages[file] = residencePage(file)
	}
	return pages
}

func residencePage(file string) string {
	return "<html><body>" + file + " %objectId% %npcname% %castleid%</body></html>"
}

// residenceSetup is the world a castle NPC talk boots with. Talker leads
// Lords, which owns Gludio, unless rank makes it a member of that power
// grade holding privs under an offline founder, rival has another clan own
// Gludio, or clanless leaves Talker out of every clan. siege runs Gludio's
// siege, its battlefield around the players; manorOff turns the manor off.
type residenceSetup struct {
	rank, privs            int
	rival, clanless, siege bool
	manorOff               bool
	strife                 sevensigns.Cabal
}

func bootResidenceChat(t *testing.T, s residenceSetup) *folkWorld {
	t.Helper()
	opts := []gameservertest.Option{
		gameservertest.WithCastles(residenceCastles(t)),
		gameservertest.WithManor(network.ManorConfig{Allowed: !s.manorOff}),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedResidenceChat(t, db, s) }),
		gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) {
			ctx := context.Background()
			row, found, err := store.LoadStatus(ctx)
			if err != nil || !found {
				t.Fatalf("load seven signs status: found=%v err=%v", found, err)
			}
			row.SealOwners[2] = s.strife
			if err := store.SaveStatus(ctx, row); err != nil {
				t.Fatalf("seed seven signs status: %v", err)
			}
		}),
	}
	if s.siege {
		opts = append(opts, gameservertest.WithSieges(siege.DefaultConfig()), gameservertest.WithZones(gludioBattlefield(t)))
	}
	w := bootFolkWorld(t, residencePages(), opts...)
	if s.siege {
		gludio, ok := w.srv.Sieges.Get(taxCastleID)
		if !ok {
			t.Fatal("no Gludio siege")
		}
		gludio.Start()
		if !gludio.InProgress() {
			t.Fatal("Gludio siege not under way")
		}
		drainUntilQuiet(t, w.c)
	}
	return w
}

// gludioBattlefield is Gludio's siege zone, around the players.
func gludioBattlefield(t *testing.T) *zone.Index {
	t.Helper()
	form, err := zone.NewCuboid(-2000, 2000, -2000, 2000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	set := commons.NewStatSet()
	set.Set("castleId", strconv.Itoa(taxCastleID))
	field, err := zone.NewSiege(1, form, set)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(field)
	return zones
}

func seedResidenceChat(t *testing.T, db *sql.DB, s residenceSetup) {
	t.Helper()
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatalf("seed castle NPC talk: %s: %v", q, err)
		}
	}
	if s.clanless {
		exec("INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle) VALUES (?, 'Rivals', 5, ?)", residenceRivalClan, taxCastleID)
		return
	}
	castleOfLords, castleOfRivals := taxCastleID, 0
	if s.rival {
		castleOfLords, castleOfRivals = 0, taxCastleID
	}
	if s.rank == 0 {
		exec("UPDATE characters SET clanid = ?, power_grade = 0 WHERE char_name = 'Talker'", taxClanID)
		exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id)
			SELECT ?, ?, 5, ?, obj_Id FROM characters WHERE char_name = 'Talker'`, taxClanID, taxClanName, castleOfLords)
	} else {
		exec("UPDATE characters SET clanid = ?, power_grade = ? WHERE char_name = 'Talker'", taxClanID, s.rank)
		exec(`INSERT INTO characters (account_name, obj_Id, char_name, level, clanid, power_grade) VALUES ('founder', ?, 'Founder', 40, ?, 0)`,
			residenceFounderID, taxClanID)
		exec("INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id) VALUES (?, ?, 5, ?, ?)",
			taxClanID, taxClanName, castleOfLords, residenceFounderID)
		exec("INSERT INTO clan_privs (clan_id, ranking, privs) VALUES (?, ?, ?)", taxClanID, s.rank, s.privs)
	}
	exec("INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle) VALUES (?, 'Rivals', 5, ?)", residenceRivalClan, castleOfRivals)
	if s.siege {
		exec("INSERT INTO characters (account_name, obj_Id, char_name, level, clanid) VALUES ('raider', ?, 'Raider', 40, ?)", residenceRaiderID, residenceRaidClan)
		exec("INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (?, 'Raiders', 5, ?)", residenceRaidClan, residenceRaiderID)
		exec("INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, 'ATTACKER')", taxCastleID, residenceRaidClan)
	}
}

// residenceNPC is one castle NPC type, the release it answers with, and
// whether its page names the NPC and the castle.
type residenceNPC struct {
	kind  string
	id    int
	shape []byte
}

var (
	releasedPage = []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeNpcHtmlMessage}
	pageReleased = []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
	pageAlone    = []byte{serverpackets.OpcodeNpcHtmlMessage}

	chamberlainNPC = residenceNPC{"CastleChamberlain", castleChamberlainID, releasedPage}
	blacksmithNPC  = residenceNPC{"CastleBlacksmith", castleBlacksmithID, pageAlone}
	magicianNPC    = residenceNPC{"CastleMagician", castleMagicianID, releasedPage}
	warehouseNPC   = residenceNPC{"CastleWarehouseKeeper", castleWarehouseID, releasedPage}
	mercenaryNPC   = residenceNPC{"MercenaryManagerNpc", castleMercenaryID, pageAlone}
	manorNPC       = residenceNPC{"ManorManagerNpc", castleManorID, pageReleased}
	castleDoorNPC  = residenceNPC{"CastleDoorman", castleDoormanID, pageReleased}
	frontierNPC    = residenceNPC{"Doorman", frontierDoormanID, pageReleased}
)

// talkResidence spawns n, talks to it and checks the chat window: the
// interact's release, approach and animation, then shape, with file's page
// filled as wantFilled says the type fills it.
func (w *folkWorld) talkResidence(t *testing.T, n residenceNPC, file string, dx int) {
	t.Helper()
	tmpl := folkTemplate(n.kind, n.id)
	tmpl.Name = "Keeper"
	f := w.spawnFolk(t, tmpl, dx)
	w.selectFolk(t, f)
	frames := w.talk(t, f, false)
	want := append([]byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeSocialAction}, n.shape...)
	if got := interactOrder(frames); !bytes.Equal(got, want) {
		t.Fatalf("%s talk = % x, want % x", n.kind, got, want)
	}
	if got, want := pageIn(t, frames), wantResidencePage(n, file, f); got != want {
		t.Fatalf("%s page = %q, want %q", n.kind, got, want)
	}
}

// wantResidencePage is file's page as n's type fills it at f.
func wantResidencePage(n residenceNPC, file string, f *npc.Folk) string {
	page := residencePage(file)
	switch {
	case n.kind == "CastleBlacksmith" && file == "npcdefault.htm":
		page = fill(page, "%npcname%", "Keeper")
	case n.kind == "CastleBlacksmith":
		page = fill(page, "%npcname%", "Keeper", "%castleid%", strconv.Itoa(taxCastleID))
	case n.kind == "CastleWarehouseKeeper":
		page = fill(page, "%npcname%", "Keeper")
	}
	return wantChatPage(page, f)
}

// TestCastleNPCChatWindowsForTheLord pins each castle NPC's page for the
// leader of the clan owning its castle, which holds every privilege, the
// Seal of Strife owned by the Dawn; a plain doorman opens its page to
// everyone.
func TestCastleNPCChatWindowsForTheLord(t *testing.T) {
	t.Parallel()
	w := bootResidenceChat(t, residenceSetup{strife: sevensigns.Dawn})
	for i, c := range []struct {
		n    residenceNPC
		file string
	}{
		{chamberlainNPC, "chamberlain/chamberlain.htm"},
		{blacksmithNPC, "castleblacksmith/castleblacksmith.htm"},
		{magicianNPC, "castlemagician/magician.htm"},
		{warehouseNPC, "castlewarehouse/castlewarehouse.htm"},
		{mercenaryNPC, "mercmanager/mseller001_dawn.htm"},
		{manorNPC, "manormanager/manager-lord.htm"},
		{castleDoorNPC, "doormen/35096.htm"},
		{frontierNPC, "doormen/30596.htm"},
	} {
		w.talkResidence(t, c.n, c.file, 30+10*i)
	}
}

// TestCastleNPCChatWindowsForAMember pins the pages for a member of the
// owning clan holding no privilege, the Seal of Strife owned by the Dusk:
// the chamberlain, the magician and the warehouse keeper welcome any
// member; the blacksmith, the mercenary manager and the doorman want their
// privilege; the manor manager's lord page is the leader's.
func TestCastleNPCChatWindowsForAMember(t *testing.T) {
	t.Parallel()
	w := bootResidenceChat(t, residenceSetup{rank: 5, strife: sevensigns.Dusk})
	for i, c := range []struct {
		n    residenceNPC
		file string
	}{
		{chamberlainNPC, "chamberlain/chamberlain.htm"},
		{blacksmithNPC, "castleblacksmith/castleblacksmith-no.htm"},
		{magicianNPC, "castlemagician/magician.htm"},
		{warehouseNPC, "castlewarehouse/castlewarehouse.htm"},
		{mercenaryNPC, "mercmanager/mseller002.htm"},
		{manorNPC, "manormanager/manager.htm"},
		{castleDoorNPC, "doormen/35096-no.htm"},
	} {
		w.talkResidence(t, c.n, c.file, 30+10*i)
	}
}

// TestCastleNPCChatWindowsForPrivilegedMembers pins that a member holding
// the manor, mercenary and entry privileges is the owner to the
// blacksmith, the mercenary manager and the doorman; no cabal owning the
// Seal of Strife shows the plain mercenary page.
func TestCastleNPCChatWindowsForPrivilegedMembers(t *testing.T) {
	t.Parallel()
	w := bootResidenceChat(t, residenceSetup{rank: 5, privs: 65536 | 2097152 | 32768})
	for i, c := range []struct {
		n    residenceNPC
		file string
	}{
		{blacksmithNPC, "castleblacksmith/castleblacksmith.htm"},
		{mercenaryNPC, "mercmanager/mseller001.htm"},
		{castleDoorNPC, "doormen/35096.htm"},
	} {
		w.talkResidence(t, c.n, c.file, 30+10*i)
	}
}

// TestCastleNPCChatWindowsForStrangers pins the refusal pages for the
// leader of a clan not owning the castle, and for a clanless talker, whom
// no castle NPC answers as more than a stranger.
func TestCastleNPCChatWindowsForStrangers(t *testing.T) {
	t.Parallel()
	for _, s := range []residenceSetup{{rival: true}, {clanless: true}} {
		t.Run(map[bool]string{true: "clanless", false: "rival clan"}[s.clanless], func(t *testing.T) {
			t.Parallel()
			w := bootResidenceChat(t, s)
			for i, c := range []struct {
				n    residenceNPC
				file string
			}{
				{chamberlainNPC, "chamberlain/no.htm"},
				{blacksmithNPC, "castleblacksmith/castleblacksmith-no.htm"},
				{magicianNPC, "castlemagician/magician-no.htm"},
				{warehouseNPC, "castlewarehouse/castlewarehouse-no.htm"},
				{mercenaryNPC, "mercmanager/mseller002.htm"},
				{manorNPC, "manormanager/manager.htm"},
				{castleDoorNPC, "doormen/35096-no.htm"},
				{frontierNPC, "doormen/30596.htm"},
			} {
				w.talkResidence(t, c.n, c.file, 30+10*i)
			}
		})
	}
}

// TestCastleNPCChatWindowsUnderSiege pins the busy pages while the castle's
// siege runs and its battlefield is active, even for its lord; the doorman
// and the manor manager do not look at the siege.
func TestCastleNPCChatWindowsUnderSiege(t *testing.T) {
	t.Parallel()
	w := bootResidenceChat(t, residenceSetup{siege: true})
	for i, c := range []struct {
		n    residenceNPC
		file string
	}{
		{chamberlainNPC, "chamberlain/busy.htm"},
		{blacksmithNPC, "castleblacksmith/castleblacksmith-busy.htm"},
		{magicianNPC, "castlemagician/magician-busy.htm"},
		{warehouseNPC, "castlewarehouse/castlewarehouse-busy.htm"},
		{mercenaryNPC, "mercmanager/mseller003.htm"},
		{manorNPC, "manormanager/manager-lord.htm"},
		{castleDoorNPC, "doormen/35096.htm"},
	} {
		w.talkResidence(t, c.n, c.file, 30+10*i)
	}
}

// TestCastleNPCChatWindowsWithoutTheManor pins the generic page the
// blacksmith and the manor manager show while the manor is off.
func TestCastleNPCChatWindowsWithoutTheManor(t *testing.T) {
	t.Parallel()
	w := bootResidenceChat(t, residenceSetup{manorOff: true})
	w.talkResidence(t, blacksmithNPC, "npcdefault.htm", 30)
	w.talkResidence(t, manorNPC, "npcdefault.htm", 40)
}

// TestCastleNPCChatWindowsForAMercenaryHirer pins that a member allowed to
// hire mercenaries, the Seal of Strife owned by the Dusk, is shown the
// Dusk's mercenary page.
func TestCastleNPCChatWindowsForAMercenaryHirer(t *testing.T) {
	t.Parallel()
	w := bootResidenceChat(t, residenceSetup{rank: 5, privs: 2097152, strife: sevensigns.Dusk})
	w.talkResidence(t, mercenaryNPC, "mercmanager/mseller001_dusk.htm", 30)
}

// TestCastleBlacksmithOfNoCastle pins that a castle blacksmith listed by no
// castle, the manor on, sends nothing after the interact.
func TestCastleBlacksmithOfNoCastle(t *testing.T) {
	t.Parallel()
	w := bootResidenceChat(t, residenceSetup{})
	tmpl := folkTemplate("CastleBlacksmith", unlistedResidentID)
	tmpl.Name = "Keeper"
	f := w.spawnFolk(t, tmpl, 30)
	w.selectFolk(t, f)
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeSocialAction}
	if got := interactOrder(w.talk(t, f, false)); !bytes.Equal(got, want) {
		t.Fatalf("talk = % x, want % x", got, want)
	}
}
