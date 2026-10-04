package items

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The castle world: "Newbie" leads castleClanID, a level 5 clan, beside an
// offline member wearing the Circlet of Gludio.
const (
	castleClanID     = 0x70000011
	offlineMemberID  = 0x70000012
	offlineCircletID = 0x70000013

	circletOfGludioID int32 = 6838 // castle="1" pledgeClass="2"
	lordsCrownID      int32 = 6841 // castle="-1" pledgeClass="-1" (the leader)
)

// castleWorld is a booted castle world: the leader in the world and a game
// master of access level 7 beside it, its target the leader.
type castleWorld struct {
	srv      *gameservertest.Server
	leader   *testsupport.ScriptedClient
	leaderID int32
	gm       *testsupport.ScriptedClient
}

// bootCastleWorld boots the castle world; castle is the castle the clan's
// row holds (0 for none) and seed runs once the clan is seeded.
func bootCastleWorld(t *testing.T, castle int, seed func(*sql.DB)) *castleWorld {
	t.Helper()
	datapack.Require(t)
	skills, shippedItems := shippedData()
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{circletOfGludioID, lordsCrownID} {
		tmpl, ok := shippedItems.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	adminData, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	page, err := os.ReadFile(datapack.Path(t, "data", "html", "admin", "castle.htm"))
	if err != nil {
		t.Fatalf("read castle page: %v", err)
	}
	db := sqltest.SharedDB(t)
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), skills, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCastles(castles),
		gameservertest.WithAdmin(adminData),
		gameservertest.WithHTMLPages(map[string]string{"admin/castle.htm": string(page)}),
		gameservertest.WithReuseDelays(3*time.Second, 0),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, s := range []struct {
				q    string
				args []any
			}{
				{"UPDATE characters SET clanid = ? WHERE char_name = 'Newbie'", []any{castleClanID}},
				{`INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id)
					SELECT ?, 'Lords', 5, ?, obj_Id FROM characters WHERE char_name = 'Newbie'`, []any{castleClanID, castle}},
				{"INSERT INTO characters (account_name, obj_Id, char_name, level, clanid, power_grade) VALUES ('away', ?, 'Away', 40, ?, 2)", []any{offlineMemberID, castleClanID}},
				{"INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) VALUES (?, ?, ?, 1, 'PAPERDOLL', 14)", []any{offlineMemberID, offlineCircletID, circletOfGludioID}},
			} {
				if _, err := db.ExecContext(context.Background(), s.q, s.args...); err != nil {
					t.Fatalf("seed castle clan: %v", err)
				}
			}
			if seed != nil {
				seed(db)
			}
		}),
	)
	w := &castleWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t)}
	gmID := srv.SeedCharacterFor(t, "gm", "Admin", 10, 0).ID
	if _, err := db.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE obj_Id = ?", gmID); err != nil {
		t.Fatalf("set access level: %v", err)
	}
	return w
}

// enter brings the leader, then the game master, into the world and has
// the game master select the leader.
func (w *castleWorld) enter(t *testing.T) {
	t.Helper()
	enterAsClanMember(w.leader)
	drainUntilQuiet(t, w.leader)
	w.gm = w.srv.DialClient(t, "gm", 1)
	w.gm.Send(encodeRequestGameStart(0))
	w.gm.Send(encodeEnterWorld())
	drainUntilQuiet(t, w.gm)
	w.gm.Send(encodeAction(w.leaderID, 0, 0, 0, false))
	drainUntilQuiet(t, w.gm)
	drainUntilQuiet(t, w.leader)
}

// castleCommand sends //castle args from the game master and returns its
// frames and the leader's.
func (w *castleWorld) castleCommand(t *testing.T, args string) (gm, leader [][]byte) {
	t.Helper()
	cmd := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	cmd.WriteString("castle " + args)
	w.gm.Send(cmd.Bytes())
	return collectFrames(w.gm), collectFrames(w.leader)
}

// collectFrames reads until the client stays quiet for 300ms.
func collectFrames(c *testsupport.ScriptedClient) [][]byte {
	var out [][]byte
	for range 100 {
		f := c.ReadWithTimeout(300 * time.Millisecond)
		if f == nil {
			break
		}
		out = append(out, f)
	}
	return out
}

// gmAnswer decodes the game master's frames: the texts it was told, the
// page it was shown ("" without one), and whether it got ActionFailed. The
// leader's look broadcast to it is skipped.
func gmAnswer(t *testing.T, frames [][]byte) (texts []string, page string, failed bool) {
	t.Helper()
	for _, f := range frames {
		r := wire.NewReader(f[1:])
		switch f[0] {
		case serverpackets.OpcodeSystemMessage:
			id := r.ReadInt32()
			if n := r.ReadInt32(); id == serverpackets.SystemMessageS1 && n == 1 {
				r.ReadInt32()
				texts = append(texts, r.ReadString())
			} else {
				texts = append(texts, "sm"+strconv.Itoa(int(id)))
			}
		case serverpackets.OpcodeNpcHtmlMessage:
			r.ReadInt32()
			page = r.ReadString()
		case serverpackets.OpcodeActionFailed:
			failed = true
		case serverpackets.OpcodeCharInfo, serverpackets.OpcodeRelationChanged:
			// The selected leader's new look, after an unequip.
		default:
			t.Fatalf("game master got opcode %#x among %x", f[0], opcodesOf(frames))
		}
	}
	return texts, page, failed
}

// pledgeCastle returns the castle id of the PledgeShowInfoUpdate among
// frames and its index.
func pledgeCastle(t *testing.T, frames [][]byte) (castle int32, index int) {
	t.Helper()
	for i, f := range frames {
		if f[0] != serverpackets.OpcodePledgeShowInfoUpdate {
			continue
		}
		r := wire.NewReader(f[1:])
		if clanID := r.ReadInt32(); clanID != castleClanID {
			t.Fatalf("PledgeShowInfoUpdate clan = %#x, want %#x", clanID, castleClanID)
		}
		r.ReadInt32()
		r.ReadInt32()
		return r.ReadInt32(), i
	}
	t.Fatalf("no PledgeShowInfoUpdate among opcodes %x", opcodesOf(frames))
	return 0, 0
}

func queryCastleInt(t *testing.T, srv *gameservertest.Server, query string, args ...any) int64 {
	t.Helper()
	srv.FlushPersistence(t)
	var v int64
	if err := srv.DB.QueryRowContext(context.Background(), query, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return v
}

// TestAdminCastleSetAndRemoveOwner drives //castle set and //castle remove
// on Gludio Castle: the clan's members in the world see the clan header
// carry the castle, hear the siege victory music, and may then wear the
// Lord's Crown; taking the castle back refreshes the header to no castle,
// takes the crown off the online leader and returns the offline member's
// circlet to its inventory, and resets the castle's finances. Both the
// clan_data owner column and the castle row are stored.
func TestAdminCastleSetAndRemoveOwner(t *testing.T) {
	t.Parallel()
	w := bootCastleWorld(t, 0, func(db *sql.DB) {
		if _, err := db.ExecContext(context.Background(), `UPDATE castle SET currentTaxPercent=10, nextTaxPercent=20, treasury=5000 WHERE id=1`); err != nil {
			t.Fatalf("seed castle row: %v", err)
		}
	})
	crown := w.srv.GiveItem(t, w.leaderID, lordsCrownID, 1)
	w.enter(t)

	gm, leader := w.castleCommand(t, "set gludio_castle")
	castle, i := pledgeCastle(t, leader)
	if castle != 1 || i+1 >= len(leader) || leader[i+1][0] != serverpackets.OpcodePlaySound {
		t.Fatalf("leader after set: castle %d, opcodes %x; want castle 1 then PlaySound", castle, opcodesOf(leader))
	}
	sound := wire.NewReader(leader[i+1][1:])
	if typ, file := sound.ReadInt32(), sound.ReadString(); typ != 1 || file != "Siege_Victory" {
		t.Fatalf("PlaySound = %d %q, want 1 \"Siege_Victory\"", typ, file)
	}
	texts, page, _ := gmAnswer(t, gm)
	if len(texts) != 0 || !strings.Contains(page, "Gludio Castle") {
		t.Fatalf("gm after set: texts %q, page %q", texts, page)
	}
	if got := queryCastleInt(t, w.srv, "SELECT hasCastle FROM clan_data WHERE clan_id = ?", castleClanID); got != 1 {
		t.Fatalf("hasCastle after set = %d, want 1", got)
	}

	w.leader.Send(encodeUseItem(crown, false))
	drainUntilQuiet(t, w.leader)
	drainUntilQuiet(t, w.gm) // the leader's new look
	w.srv.FlushItems(t)
	if inst := mustFindItem(t, w.srv, w.leaderID, crown); inst.Location != item.LocationPaperdoll {
		t.Fatalf("crown after equip = %v, want paperdoll", inst.Location)
	}

	gm, leader = w.castleCommand(t, "set gludio_castle")
	if texts, _, _ := gmAnswer(t, gm); len(texts) != 1 || texts[0] != "Newbie's clan already owns a castle." {
		t.Fatalf("second set told %q", texts)
	}
	if len(leader) != 0 {
		t.Fatalf("leader got %x on a refused set", opcodesOf(leader))
	}

	gm, leader = w.castleCommand(t, "remove gludio_castle")
	castle, i = pledgeCastle(t, leader)
	if castle != 0 {
		t.Fatalf("PledgeShowInfoUpdate castle after remove = %d, want 0", castle)
	}
	if i != 0 {
		t.Fatalf("leader after remove: opcodes %x; want the clan header before the crown comes off", opcodesOf(leader))
	}
	if _, page, _ := gmAnswer(t, gm); !strings.Contains(page, "Current/next: 15% / 15%") || !strings.Contains(page, "Treasure: 0<") {
		t.Fatalf("castle page after remove: %q", page)
	}
	w.srv.FlushItems(t)
	if inst := mustFindItem(t, w.srv, w.leaderID, crown); inst.Location != item.LocationInventory {
		t.Fatalf("crown after remove = %v, want inventory", inst.Location)
	}
	var loc string
	w.srv.FlushPersistence(t)
	if err := w.srv.DB.QueryRowContext(context.Background(), "SELECT loc FROM items WHERE object_id = ?", offlineCircletID).Scan(&loc); err != nil || loc != "INVENTORY" {
		t.Fatalf("offline member's circlet loc = %q (%v), want INVENTORY", loc, err)
	}
	if got := queryCastleInt(t, w.srv, "SELECT hasCastle FROM clan_data WHERE clan_id = ?", castleClanID); got != 0 {
		t.Fatalf("hasCastle after remove = %d, want 0", got)
	}
	for col, want := range map[string]int64{"treasury": 0, "currentTaxPercent": 15, "nextTaxPercent": 15} {
		if got := queryCastleInt(t, w.srv, "SELECT "+col+" FROM castle WHERE id = 1"); got != want {
			t.Fatalf("castle %s after remove = %d, want %d", col, got, want)
		}
	}

	gm, _ = w.castleCommand(t, "remove gludio_castle")
	if texts, _, _ := gmAnswer(t, gm); len(texts) != 1 || texts[0] != "This castle does not have an owner." {
		t.Fatalf("remove of a free castle told %q", texts)
	}
}

// TestAdminCastlePageRestoresStoredCastle boots a clan owning Gludio
// Castle from its stored rows: the //castle page shows the stored tax
// rates, certificates and money, digits grouped, its artifact and control
// towers as teleport links; //castle tax then closes the tax period
// (revenue and income into the treasury, the next rate in force) and
// //castle certificates puts the certificates back to 300, both stored.
func TestAdminCastlePageRestoresStoredCastle(t *testing.T) {
	t.Parallel()
	w := bootCastleWorld(t, 1, func(db *sql.DB) {
		if _, err := db.ExecContext(context.Background(), `UPDATE castle SET currentTaxPercent=10, nextTaxPercent=20,
			treasury=1234567, taxRevenue=1000, seedIncome=500, certificates=120 WHERE id=1`); err != nil {
			t.Fatalf("seed castle row: %v", err)
		}
	})
	w.enter(t)

	gm, _ := w.castleCommand(t, "gludio_castle")
	_, page, _ := gmAnswer(t, gm)
	for _, want := range []string{
		"Left cert.: 120 (",
		"Parent: 5<",
		"Alive Life Towers : 3<",
		"Default: 15%<",
		"Current/next: 10% / 20%<",
		"TaxSysget: 40%<",
		"Tax revenue: 1,000<",
		"Tribute: 25%<",
		"Seed income: 500<",
		"Treasure: 1,234,567<",
		`<a action="bypass -h admin_teleport -18113 107972 -2480 16384">[1]</a>&nbsp;&nbsp;`,
		`<a action="bypass -h admin_teleport -18325 112811 -2377 0">[1]</a>&nbsp;&nbsp;<a action="bypass -h admin_teleport -18048 107098 -2378 0">[2]</a>`,
		"bypass -h admin_castle tax gludio_castle",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("castle page lacks %q:\n%s", want, page)
		}
	}

	gm, _ = w.castleCommand(t, "tax gludio_castle")
	texts, page, _ := gmAnswer(t, gm)
	if len(texts) != 1 || texts[0] != "Gludio Castle's taxes have been updated." {
		t.Fatalf("tax told %q", texts)
	}
	if !strings.Contains(page, "Treasure: 1,236,067<") || !strings.Contains(page, "Current/next: 20% / 20%<") || !strings.Contains(page, "Tax revenue: 0<") {
		t.Fatalf("castle page after tax: %q", page)
	}
	for col, want := range map[string]int64{"treasury": 1236067, "taxRevenue": 0, "seedIncome": 0, "currentTaxPercent": 20, "nextTaxPercent": 20} {
		if got := queryCastleInt(t, w.srv, "SELECT "+col+" FROM castle WHERE id = 1"); got != want {
			t.Fatalf("castle %s after tax = %d, want %d", col, got, want)
		}
	}

	gm, _ = w.castleCommand(t, "certificates gludio_castle")
	if texts, page, _ := gmAnswer(t, gm); len(texts) != 1 || texts[0] != "Gludio Castle's castle certificates are reset." || !strings.Contains(page, "Left cert.: 300 (") {
		t.Fatalf("certificates told %q, page %q", texts, page)
	}
	if got := queryCastleInt(t, w.srv, "SELECT certificates FROM castle WHERE id = 1"); got != 300 {
		t.Fatalf("certificates after reset = %d, want 300", got)
	}

	gm, _ = w.castleCommand(t, "bogus gludio_castle")
	if texts, page, _ := gmAnswer(t, gm); len(texts) != 1 || texts[0] != "Usage: //castle [set|remove|certificates|tax castleName]." || page == "" {
		t.Fatalf("unknown action told %q, page %q", texts, page)
	}
	gm, _ = w.castleCommand(t, "set nowhere_castle")
	if texts, page, failed := gmAnswer(t, gm); len(texts) != 0 || page != "" || !failed {
		t.Fatalf("unknown castle: texts %q, page %q, ActionFailed %v", texts, page, failed)
	}
}
