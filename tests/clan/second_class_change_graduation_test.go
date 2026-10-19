package clan

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/secondclasschange"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The second class change feature (SecondClassChange) graduating an
// academy member: the graduation scenario's level 40 Warrior, holding the
// marks of the Gladiator, changes occupation at Grand Master Hannavalt
// instead of the class manager.

const (
	hannavaltID = 30109
	// The marks of the Gladiator and the transfer's reward.
	markOfChallenger = 2627
	markOfTrust      = 2734
	markOfDuelist    = 2762
	cGradeCoupon     = 8870
	smItemDisappears = 302
	smEarnedItems    = 53
)

// secondClassChangePages are Hannavalt's village master page and the
// feature's datapack pages.
func secondClassChangePages(t *testing.T) map[string]string {
	t.Helper()
	pages := map[string]string{}
	read := func(rel string) {
		data, err := os.ReadFile(datapack.Path(t, append([]string{"data", "html"}, strings.Split(rel, "/")...)...))
		if err != nil {
			t.Fatal(err)
		}
		pages[rel] = string(data)
	}
	read("villagemaster/" + strconv.Itoa(hannavaltID) + ".htm")
	dir := datapack.Path(t, "data", "html", "script", "feature", "SecondClassChange")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		read("script/feature/SecondClassChange/" + e.Name())
	}
	return pages
}

// TestSecondClassChangeGraduatesAcademyMember: the academy member takes
// the Gladiator at Hannavalt. The marks go and the coupons come, then the
// occupation change graduates it as the class manager's does: the clan
// earns the graduation points, the member is told and leaves the clan
// with the Academy Circlet, and the change is shown, before the
// congratulation page.
func TestSecondClassChangeGraduatesAcademyMember(t *testing.T) {
	datapack.Require(t)
	templates := append([]*item.Template(nil), graduationItems().All()...)
	for _, id := range []int32{markOfChallenger, markOfTrust, markOfDuelist, cGradeCoupon} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Founder", 10, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(secondClassChangePages(t)),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{hannavaltID: script.KindFolk},
			[]script.Listing{{Path: "script.feature.SecondClassChange"}},
			script.Catalog{"script.feature.SecondClassChange": secondclasschange.New}),
		gameservertest.WithClassTemplates(occupationTemplate(warriorClassID), occupationTemplate(gladiatorClassID)),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		seedGraduate(t, warriorClassID, 40),
		seedSubunitClan(t,
			academySeed,
			`UPDATE clan_data SET clan_level = 7 WHERE clan_id = `+itoa(subunitClanID),
			`UPDATE characters SET clanid = `+itoa(subunitClanID)+`, subpledge = -1, power_grade = 9, lvl_joined_academy = `+
				strconv.Itoa(graduateJoinedAt)+` WHERE obj_Id = `+itoa(graduateID),
		),
	)
	for _, id := range []int32{markOfChallenger, markOfTrust, markOfDuelist} {
		srv.GiveItem(t, graduateID, id, 1)
	}
	leader, graduate := srv.Client, srv.DialClient(t, "player2", 1)
	startInWorld(t, leader)
	startInWorld(t, graduate)
	drainFrames(t, leader)
	x, y, z := srv.PlayerPosition(t, graduateID)
	at := location.Location{X: x, Y: y, Z: z}
	hannavalt := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("VillageMasterFighter", hannavaltID), location.Location{X: x + 30, Y: y, Z: z})
	drainFrames(t, leader)
	drainFrames(t, graduate)
	for range 2 {
		graduate.Send(encodeAction(hannavalt.ObjectID(), at))
		drainFrames(t, graduate)
	}
	for _, cmd := range []string{
		"npc_" + strconv.Itoa(int(hannavalt.ObjectID())) + "_Quest SecondClassChange",
		"Quest SecondClassChange master_human_elf_fighter-30.htm",
	} {
		graduate.Send(encodeBypass(cmd))
		if got := only(drainFrames(t, graduate), serverpackets.OpcodeNpcHtmlMessage); len(got) != 1 {
			t.Fatalf("%s: pages = %d, want 1", cmd, len(got))
		}
	}
	graduate.Send(encodeBypass("Quest SecondClassChange GL"))
	frames := drainFrames(t, graduate)
	drainFrames(t, leader)

	// The marks and coupons, the fanfare, then the occupation change: the
	// graduation's notices and the circlet before its animation and
	// message, then the page.
	want := []int{
		smItemDisappears, smItemDisappears, smItemDisappears, smEarnedItems,
		smGraduatedFromAcademy, smAcademyMembershipEnded, smYouHaveWithdrawnFromClan, smYouPickedUpS1, smClassTransfer,
	}
	if got := messages(t, frames); !slices.Equal(got, want) {
		t.Fatalf("graduate's messages = %v, want %v", got, want)
	}
	order := only(frames, serverpackets.OpcodePlaySound, serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeNpcHtmlMessage)
	if !slices.Equal(order, []byte{serverpackets.OpcodePlaySound, serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeNpcHtmlMessage}) {
		t.Fatalf("graduate's sound, animation and page = %x", order)
	}

	g := onlineCharacter(t, srv, graduateID)
	if g.ClanID() != 0 || g.ClassID() != gladiatorClassID || g.BaseClassID() != gladiatorClassID {
		t.Fatalf("graduate clan %d class %d base class %d, want no clan and the Gladiator", g.ClanID(), g.ClassID(), g.BaseClassID())
	}
	inv := g.Inventory()
	if inv.ItemCount(academyCircletID, -1, true) != 1 || inv.ItemCount(cGradeCoupon, -1, true) != 15 {
		t.Fatalf("circlets %d coupons %d, want 1 and 15", inv.ItemCount(academyCircletID, -1, true), inv.ItemCount(cGradeCoupon, -1, true))
	}
	srv.FlushPersistence(t)
	if got := queryInt(t, &clanWorld{srv: srv}, `SELECT reputation_score FROM clan_data WHERE clan_id = ?`, subunitClanID); got != 20000+graduationPoints {
		t.Fatalf("stored reputation = %d, want %d", got, 20000+graduationPoints)
	}
}
