package admin

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Fixture ids of the //info scenarios.
const (
	infoGateID     = 24190001
	infoGrocerID   = 30001
	infoWolfID     = 20120
	infoPetID      = 12077
	infoCollarID   = int32(9600)
	infoSummonSkil = 2046
	// infoServitorID is the servitor infoServitorSkill summons.
	infoServitorID    = 14001
	infoServitorSkill = 1129
)

// infoPage sends //cmd and returns the one page it opens.
func infoPage(t *testing.T, gm *testsupport.ScriptedClient, cmd string) string {
	t.Helper()
	frames := exchange(t, gm, encodeBuildCmd(cmd))
	if len(frames) != 1 {
		t.Fatalf("//%s frames = %x, want one NpcHtmlMessage", cmd, testsupport.FrameOpcodes(frames))
	}
	return htmlBody(t, frames[0])
}

// assertInfoPage requires //cmd to open want.
func assertInfoPage(t *testing.T, gm *testsupport.ScriptedClient, cmd, want string) {
	t.Helper()
	got := infoPage(t, gm, cmd)
	if got == want {
		return
	}
	at := 0
	for at < min(len(got), len(want)) && got[at] == want[at] {
		at++
	}
	t.Fatalf("//%s page differs at byte %d:\n got  %q\n want %q", cmd, at, got[max(at-80, 0):min(at+80, len(got))], want[max(at-80, 0):min(at+80, len(want))])
}

// assertActionFailed requires //cmd to answer only ActionFailed.
func assertActionFailed(t *testing.T, gm *testsupport.ScriptedClient, cmd string) {
	t.Helper()
	frames := exchange(t, gm, encodeBuildCmd(cmd))
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("//%s frames = %x, want ActionFailed", cmd, testsupport.FrameOpcodes(frames))
	}
}

// shippedPage is the datapack admin page name with each placeholder of
// pairs replaced by the value after it.
func shippedPage(t *testing.T, name string, pairs ...string) string {
	t.Helper()
	// The page cache reads a page the way the reference HtmCache does:
	// line by line, each line ended by "\n".
	page := strings.ReplaceAll(shippedAdminPages(t, name)["admin/"+name], "\r\n", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		page = strings.ReplaceAll(page, pairs[i], pairs[i+1])
	}
	return page
}

// infoCastles is one castle, Gludio, whose gates list the info gate and
// whose NPCs list the grocer.
func infoCastles(t *testing.T) *castledata.Table {
	t.Helper()
	c, err := castledata.NewCastle(castledata.CastleAttrs{
		ID: 1, Alias: "gludio", Name: "Gludio", Gates: []string{"info_gate"}, NPCs: []int{infoGrocerID},
	}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err := castledata.NewTable([]*castledata.Castle{c})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// seedStrifeOwner seeds the Seal of Strife as owned by cabal.
func seedStrifeOwner(t *testing.T, cabal sevensigns.Cabal) gameservertest.Option {
	return gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) {
		row, found, err := store.LoadStatus(t.Context())
		if err != nil || !found {
			t.Fatalf("load status row: found=%v err=%v", found, err)
		}
		row.SealOwners = [3]sevensigns.Cabal{sevensigns.NoCabal, sevensigns.NoCabal, cabal}
		if err := store.SaveStatus(t.Context(), row); err != nil {
			t.Fatalf("seed status row: %v", err)
		}
	})
}

// TestAdminInfoDoorPage pins AdminInfo.java's door page: the template's
// names, ids and opening rules, the residence whose gates list it, HP over
// a maximum the upgrade ratio (1 without door upgrades) multiplies, the
// defences raised by a fifth while Dawn owns the Seal of Strife, and where
// it stands.
func TestAdminInfoDoorPage(t *testing.T) {
	t.Parallel()
	gate := &door.Template{
		ID: infoGateID, Name: "info_gate", Kind: door.KindDoor, Level: 3,
		Position: location.Location{X: 100, Y: spawnY, Z: spawnZ},
		Coordinates: []location.Point{
			{X: 92, Y: spawnY - 8}, {X: 108, Y: spawnY - 8}, {X: 108, Y: spawnY + 8}, {X: 92, Y: spawnY + 8},
		},
		HP: 500, PDef: 101, MDef: 55, Height: 32, TriggeredID: 24190002,
		OpenKind: door.OpenTime, OpenTime: 300, RandomTime: 60, CloseTime: 120,
	}
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithDoors(gate),
		gameservertest.WithResidences(nil, infoCastles(t)),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "doorinfo.htm")),
		seedStrifeOwner(t, sevensigns.Dawn),
	)
	gm := srv.Client
	enterWorld(t, gm)
	d, ok := srv.WorldObjects.Door(infoGateID)
	if !ok {
		t.Fatal("info gate not spawned")
	}
	exchange(t, gm, encodeAction(d.ObjectID()))

	assertInfoPage(t, gm, "info", shippedPage(t, "doorinfo.htm",
		"%name%", "info_gate",
		"%objid%", strconv.Itoa(int(d.ObjectID())),
		"%doorid%", "24190001",
		"%doortype%", "DOOR",
		"%doorlvl%", "3",
		"%residence%", "Gludio",
		"%opentype%", "TIME",
		"%initial%", "Closed",
		"%ot%", "300",
		"%ct%", "120",
		"%rt%", "60",
		"%controlid%", "24190002",
		"%hp%", "500",
		"%hpmax%", "500",
		"%hpratio%", "1",
		// (int) (101 * 1.2) and (int) (55 * 1.2).
		"%pdef%", "121",
		"%mdef%", "66",
		"%spawn%", "100, 20, 30, 0",
		"%height%", "32.0",
	))
}

// TestAdminInfoStaticObjectPage pins the static object page: where it
// stands, its object and static ids, and its class.
func TestAdminInfoStaticObjectPage(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, "staticinfo.htm")))
	gm := srv.Client
	enterWorld(t, gm)
	at := location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ}
	sign, err := staticobject.NewObject(srv.NewObjectID(), &staticobject.Template{ID: 24190020, Location: at, Type: staticobject.ArenaSignType})
	if err != nil {
		t.Fatal(err)
	}
	srv.State.Spawn(sign, at.X, at.Y, at.Z, 0)
	drain(t, gm)
	exchange(t, gm, encodeAction(sign.ObjectID()))

	assertInfoPage(t, gm, "info", shippedPage(t, "staticinfo.htm",
		"%x%", "50", "%y%", "20", "%z%", "30",
		"%objid%", strconv.Itoa(int(sign.ObjectID())),
		"%staticid%", "24190020",
		"%class%", "StaticObject",
	))
}

// infoPetTemplate is a pet whose level 10 row holds 3000 food.
func infoPetTemplate() *npc.Template {
	row := npc.PetLevelStats{MaxExp: 500, MaxHP: 400, MaxMP: 80, PAtk: 100, PDef: 90, MAtk: 20, MDef: 40, MaxMeal: 3000, MealInNormal: 5, MealInBattle: 10, SSCount: 1}
	return &npc.Template{
		ID: infoPetID, Name: "Wolf", Level: 10, Type: "Pet",
		STR: 40, CON: 43, DEX: 30, INT: 22, WIT: 20, MEN: 20,
		BaseAttackRange: 40, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		AIParams: commons.NewStatSet(),
		Pet:      &npc.PetData{Food1: 2515, AutoFeedLimit: 0.55, HungryLimit: 0.3, UnsummonLimit: 0.1, Levels: map[int]npc.PetLevelStats{10: row, 11: row}},
	}
}

// TestAdminInfoPetPage pins the summon page on an unnamed pet: "N/A" for
// its name, its owner as an admin_debug link, its class and intention, HP
// and MP as current over maximum, and the pet-only inventory link, food
// over the level's food and load over the weight limit.
func TestAdminInfoPetPage(t *testing.T) {
	t.Parallel()
	collars, err := item.NewSummonItemTable([]item.SummonItem{{ItemID: infoCollarID, NPCID: infoPetID, SummonType: 1}})
	if err != nil {
		t.Fatal(err)
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: infoSummonSkil, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON_CREATURE", StaticHitTime: true, StaticReuse: true,
	}}), gamesql.NewCharacterSkillStore(db))
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{infoPetTemplate()})),
		gameservertest.WithSummonItems(collars),
		gameservertest.WithSkills(skills),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "petinfo.htm")),
	)
	collar := srv.GiveItem(t, gmID, infoCollarID, 1)
	gm := srv.Client
	enterWorld(t, gm)

	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(collar)
	w.WriteInt32(0)
	gm.Send(w.Bytes())
	var pet *summon.Actor
	srv.AdvanceUntil(t, "pet in the world", func() bool {
		obj, ok := srv.State.Summon(gmID)
		if ok {
			pet, ok = obj.(*summon.Actor)
		}
		return ok
	})
	drain(t, gm)
	exchange(t, gm, encodeAction(pet.ObjectID()))

	assertInfoPage(t, gm, "info", shippedPage(t, "petinfo.htm",
		"%name%", "N/A",
		"%level%", "10",
		"%exp%", "500",
		"%owner%", ` <a action="bypass -h admin_debug Admin">Admin</a>`,
		"%class%", "Pet",
		"%ai%", "FOLLOW",
		"%hp%", fmt.Sprintf("%d/%d", int(pet.HP()), int(pet.MaxHPValue())),
		"%mp%", fmt.Sprintf("%d/%d", int(pet.MPValue()), int(pet.MaxMPValue())),
		"%karma%", "0",
		"%undead%", "no",
		"%inv%", ` <a action="bypass admin_summon inventory">view</a>`,
		"%food%", fmt.Sprintf("%d/3000", pet.Fed()),
		"%load%", fmt.Sprintf("0/%d", pet.WeightLimit()),
	))
}

// infoServitorTemplate is an undead servitor, the necromancer's Reanimated
// Man.
func infoServitorTemplate() *npc.Template {
	return &npc.Template{
		ID: infoServitorID, TemplateID: infoServitorID, Type: "Servitor", Name: "Reanimated Man", Level: 20, Race: npc.RaceUndead,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, BaseAttackRange: 40,
		CollisionRadius: 8, CollisionHeight: 20, CorpseTime: 7, AIParams: commons.NewStatSet(),
	}
}

// TestAdminInfoServitorPage pins the summon page on a servitor: its
// template's name, its class, "yes" for an undead one, and no inventory,
// food or load.
func TestAdminInfoServitorPage(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: infoServitorSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON", NpcID: infoServitorID, SummonTotalLifeTime: 1_200_000, StaticHitTime: true, StaticReuse: true,
	}}), gamesql.NewCharacterSkillStore(db))
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{infoServitorTemplate()})),
		gameservertest.WithSkills(skills),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "petinfo.htm")),
	)
	if _, err := srv.DB.ExecContext(t.Context(), "INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index) VALUES (?, ?, 1, 0)", gmID, infoServitorSkill); err != nil {
		t.Fatalf("seed servitor summon skill: %v", err)
	}
	gm := srv.Client
	enterWorld(t, gm)

	gm.Send(encodeMagicSkillUse(infoServitorSkill))
	var servitor *summon.Actor
	srv.AdvanceUntil(t, "servitor in the world", func() bool {
		obj, ok := srv.State.Summon(gmID)
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	drain(t, gm)
	exchange(t, gm, encodeAction(servitor.ObjectID()))

	assertInfoPage(t, gm, "info", shippedPage(t, "petinfo.htm",
		"%name%", "Reanimated Man",
		"%level%", "20",
		"%exp%", "0",
		"%owner%", ` <a action="bypass -h admin_debug Admin">Admin</a>`,
		"%class%", "Servitor",
		"%ai%", "FOLLOW",
		"%hp%", fmt.Sprintf("%d/%d", int(servitor.HP()), int(servitor.MaxHPValue())),
		"%mp%", fmt.Sprintf("%d/%d", int(servitor.MPValue()), int(servitor.MaxMPValue())),
		"%karma%", "0",
		"%undead%", "yes",
		"%inv%", "none",
		"%food%", "N/A",
		"%load%", "N/A",
	))
}

// TestAdminInfoOtherSelection pins //info on a selection with no info page
// of its own, a ground item: an HTML window with no page.
func TestAdminInfoOtherSelection(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	tmpl, ok := gameservertest.ItemTemplates().Get(item.AdenaID)
	if !ok {
		t.Fatal("no adena template")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: srv.NewObjectID(), TemplateID: item.AdenaID, Count: 10, ManaLeft: -1}, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	srv.GroundItems.Drop(ground, task.DropOptions{X: spawnX + 40, Y: spawnY, Z: spawnZ})
	drain(t, gm)
	// No client packet selects a ground item; a click on one picks it up.
	onLearnerQueue(t, srv, gmID, func(pc *player.Character) { pc.StoreTarget(ground) })

	if page := infoPage(t, gm, "info"); page != "" {
		t.Fatalf("//info on a ground item page = %q, want none", page)
	}
}

// infoGrocer is a merchant with every general-page field set apart.
func infoGrocer() *npc.Template {
	return &npc.Template{
		ID: infoGrocerID, TemplateID: 30002, Type: "Merchant", Name: "Grocer Lector", Title: "Grocer", Alias: "lector",
		UsingServerSideName: true, Level: 25, CollisionRadius: 8, CollisionHeight: 23.5, HitTimeFactor: 0.5,
		RightHand: 1, LeftHand: 2, RewardExp: 1234.5, RewardSp: 12,
		BaseAttackRange: 40, BaseDamageRange: []int{0, 0, 80, 120}, BaseRandomDamage: 30, Race: npc.RaceHumanoid,
		STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 20, MEN: 82,
		HPMax: 2444, MPMax: 1300, PAtk: 500, MAtk: 300, PDef: 400, MDef: 350, AtkSpd: 253, CritRate: 4,
		RunSpeed: 120, WalkSpeed: 60, Undying: true, CanMove: true, AIParams: commons.NewStatSet(),
	}
}

// Drop fixture items.
const (
	infoItemLong   = 1001
	infoItemRecipe = 1002
)

// infoWolf is a monster of three drop categories: a seven-item drop one,
// a currency one and a spoil one.
func infoWolf() *npc.Template {
	drops := []item.Drop{
		{ItemID: 1003, Min: 1, Max: 1, Chance: 90},
		{ItemID: 1004, Min: 1, Max: 2, Chance: 0.5},
		{ItemID: 1005, Min: 3, Max: 3, Chance: 50},
		{ItemID: 1006, Min: 1, Max: 1, Chance: 10},
		{ItemID: infoItemLong, Min: 1, Max: 1, Chance: 3.0625},
		{ItemID: 1007, Min: 1, Max: 1, Chance: 6},
		{ItemID: 1008, Min: 1, Max: 1, Chance: 81},
	}
	return &npc.Template{
		ID: infoWolfID, TemplateID: infoWolfID, Type: "Monster", Name: "Wolf", Level: 3,
		BaseDamageRange: []int{0, 0, 40, 40}, Race: npc.RaceBeast,
		Clans: []string{"wolf_clan", "beast_clan"}, ClanRange: 300, IgnoredIDs: []int{20121},
		HPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		CanBeAttacked: true, CanMove: true, AIParams: commons.NewStatSet(),
		Drops: []item.DropCategory{
			{Kind: item.DropNormal, Chance: 70, Drops: drops},
			{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: item.AdenaID, Min: 1000, Max: 1000, Chance: 100}}},
			{Kind: item.DropSpoil, Chance: 100, Drops: []item.Drop{{ItemID: infoItemRecipe, Min: 1, Max: 3, Chance: 25.5}}},
		},
	}
}

// infoItems is the shared item catalog plus the drop fixture items.
func infoItems() gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for id, name := range map[int32]string{
		infoItemLong:   "Sealed Tallum Tunic of the Ancient Kingdom Masterwork",
		infoItemRecipe: "Recipe: Soulshot (D)",
		1003:           "Wolf Pelt", 1004: "Wolf Fang", 1005: "Animal Bone", 1006: "Thread", 1007: "Coal", 1008: "Stem",
	} {
		templates = append(templates, &item.Template{ID: id, Name: name, Kind: item.KindEtcItem, Duration: -1, EtcItem: &item.EtcItemDetail{}})
	}
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// pageBar is the page bar Pagination.generatePages writes for page of
// total, action with %page% standing for each linked page.
func pageBar(action string, page, total int) string {
	link := func(p int) string { return strings.ReplaceAll(action, "%page%", strconv.Itoa(p)) }
	var b strings.Builder
	b.WriteString(`<table width=280 bgcolor=000000><tr><td FIXWIDTH=22 align=center><img height=2><button action="` + link(1) + `" back=L2UI_CH3.prev1_down fore=L2UI_CH3.prev1 width=16 height=16></td>`)
	for i := page - 5; i < page-1; i++ {
		cell := ""
		if i >= 0 {
			cell = fmt.Sprintf(`<a action="%s">%02d</a>`, link(i+1), i+1)
		}
		b.WriteString("<td FIXWIDTH=26 align=center>" + cell + "</td>")
	}
	fmt.Fprintf(&b, "<td FIXWIDTH=26 align=center><font color=LEVEL>%02d</font></td>", page)
	for i := page; i < page+4; i++ {
		cell := ""
		if i < total {
			cell = fmt.Sprintf(`<a action="%s">%02d</a>`, link(i+1), i+1)
		}
		b.WriteString("<td FIXWIDTH=26 align=center>" + cell + "</td>")
	}
	b.WriteString(`<td FIXWIDTH=22 align=center><img height=2><button action="` + link(total) + `" back=L2UI_CH3.next1_down fore=L2UI_CH3.next1 width=16 height=16></td></tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
	return b.String()
}

// dropRow is the row-th drop row naming name at percent rate in color.
func dropRow(row int, name, color, percent, amount string) string {
	open := "<table width=280><tr>"
	if row%2 == 0 {
		open = "<table width=280 bgcolor=000000><tr>"
	}
	return open + "<td width=34 height=40><img src=icon.noimage width=32 height=32></td><td width=246>&nbsp;" + name +
		"<br1><table width=240><tr><td width=80><font color=B09878>Rate:</font> <font color=" + color + ">" + percent +
		"%</font></td><td width=160><font color=B09878>Amount: </font>" + amount +
		"</td></tr></table></td></tr></table><img src=L2UI.SquareGray width=280 height=1>"
}

// defaultPage is npcinfo/default.htm showing content.
func defaultPage(t *testing.T, content string) string {
	t.Helper()
	return shippedPage(t, "npcinfo/default.htm", "%content%", content)
}

var infoNpcPages = []string{
	"npcinfo/default.htm", "npcinfo/general-0.htm", "npcinfo/general-1.htm", "npcinfo/general-2.htm", "npcinfo/stat.htm",
}

// TestAdminInfoNpcGeneralPages pins //info, //info <page> and their
// fallbacks on a merchant: page 0 by default, for any page number other
// than 1 or 2 and for a word that is no sub-command; a page number past
// the int range opens nothing.
func TestAdminInfoNpcGeneralPages(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithResidences(nil, infoCastles(t)),
		gameservertest.WithHTMLPages(shippedAdminPages(t, infoNpcPages...)),
	)
	gm := srv.Client
	enterWorld(t, gm)
	grocer := srv.SpawnFolkNPCAt(t, infoGrocer(), location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
	drain(t, gm)
	exchange(t, gm, encodeAction(grocer.ObjectID()))

	page0 := shippedPage(t, "npcinfo/general-0.htm",
		"%objectId%", strconv.Itoa(int(grocer.ObjectID())),
		"%npcId%", "30001", "%idTemplate%", "30002",
		"%name%", "Grocer Lector", "%title%", "Grocer", "%alias%", "lector",
		"%usingServerSideName%", "true", "%usingServerSideTitle%", "false",
		"%type%", "Merchant", "%level%", "25",
		"%radius%", "8.0", "%height%", "23.5", "%hitTimeFactor%", "0.5",
		"%rHand%", "1", "%lHand%", "2",
	)
	for _, cmd := range []string{"info", "info 0", "info 3", "info 007", "info nonsense"} {
		assertInfoPage(t, gm, cmd, page0)
	}
	if !strings.Contains(page0, "admin_list_spawns %id%") {
		t.Fatal("general-0.htm lost its unfilled %id% placeholder")
	}
	page1 := shippedPage(t, "npcinfo/general-1.htm",
		"%exp%", "1234.5", "%sp%", "12.0",
		"%baseAttackRange%", "40", "%baseDamageRange%", "[0, 0, 80, 120]", "%baseRandomDamage%", "30",
		"%race%", "HUMANOID", "%clan%", "none", "%clanRange%", "0", "%ignoredIds%", "none",
	)
	assertInfoPage(t, gm, "info 1", page1)
	assertInfoPage(t, gm, "info 01", page1)
	assertInfoPage(t, gm, "info 2", shippedPage(t, "npcinfo/general-2.htm",
		"%isUndying%", "true", "%canBeAttacked%", "false", "%isNoSleepMode%", "false",
		"%aggroRange%", "0", "%canMove%", "true", "%isSeedable%", "false", "%residence%", "Gludio",
	))

	if frames := exchange(t, gm, encodeBuildCmd("info 99999999999")); len(frames) != 0 {
		t.Fatalf("//info past the int range frames = %x, want none", testsupport.FrameOpcodes(frames))
	}

	// A civilian builds no aggro and shows its stats.
	assertInfoPage(t, gm, "info aggro", defaultPage(t, `This NPC can't build aggro towards targets.<br><button value="Refresh" action="bypass -h admin_info aggro" width=65 height=19 back="L2UI_ch3.smallbutton2_over" fore="L2UI_ch3.smallbutton2">`))
	assertInfoPage(t, gm, "info stat", shippedPage(t, "npcinfo/stat.htm",
		"%hp%", strconv.Itoa(int(grocer.HP())), "%hpmax%", strconv.Itoa(grocer.MaxHP()),
		"%mp%", strconv.Itoa(int(grocer.MPValue())), "%mpmax%", strconv.Itoa(int(grocer.MaxMPValue())),
		"%patk%", strconv.Itoa(int(grocer.PAtk())), "%matk%", strconv.Itoa(int(grocer.MAtk())),
		"%pdef%", strconv.Itoa(int(grocer.PDef())), "%mdef%", strconv.Itoa(int(grocer.MDef())),
		"%accu%", strconv.Itoa(int(grocer.CalcStat(stat.AccuracyCombat, 0))), "%evas%", strconv.Itoa(grocer.Evasion()),
		"%crit%", strconv.Itoa(int(grocer.CalcStat(stat.CriticalRate, 4))), "%rspd%", strconv.Itoa(int(grocer.MoveSpeed())),
		"%aspd%", strconv.Itoa(grocer.AttackSpeed()), "%cspd%", strconv.Itoa(grocer.MagicAttackSpeed()),
		"%str%", "40", "%dex%", "30", "%con%", "43", "%int%", "21", "%wit%", "20", "%men%", "82",
		"%ele_fire%", "1.0", "%ele_water%", "1.0", "%ele_wind%", "1.0", "%ele_earth%", "1.0", "%ele_holy%", "1.0", "%ele_dark%", "1.0",
	))
	// A civilian holds no drop: an empty list never pages, whatever page
	// is asked.
	for _, cmd := range []string{"info drop", "info drop 0", "info drop 0 0"} {
		assertInfoPage(t, gm, cmd, defaultPage(t, "This NPC doesn't hold any drops."))
	}
	assertInfoPage(t, gm, "info spoil", defaultPage(t, "This NPC doesn't hold any spoils."))

	// The pages that read state Go does not keep yet release the client.
	for _, cmd := range []string{"info ai", "info ai 1", "info ai x", "info desire", "info script", "info shop", "info skill", "info spawn"} {
		assertActionFailed(t, gm, cmd)
	}
}

// TestAdminInfoNpcDropPages pins //info drop|spoil [page [subpage]] on a
// monster: one category to a page, its drops most likely first six to a
// sub-page, each named (recipes as "R: ", long names trimmed), coloured by
// chance and shown with its amount; a page or sub-page that does not read
// as an int, or one below 1, opens the first drop page instead, even from
// //info spoil.
func TestAdminInfoNpcDropPages(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel,
		infoItems(),
		gameservertest.WithHTMLPages(shippedAdminPages(t, infoNpcPages...)),
		// The rates a default configuration gives.
		gameservertest.WithNpcDropRates(item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}),
	)
	gm := srv.Client
	enterWorld(t, gm)
	wolf := srv.SpawnHostileNPCTemplateAt(t, infoWolf(), location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
	drain(t, gm)
	exchange(t, gm, encodeAction(wolf.ObjectID()))

	assertInfoPage(t, gm, "info 1", shippedPage(t, "npcinfo/general-1.htm",
		"%exp%", "0.0", "%sp%", "0.0",
		"%baseAttackRange%", "0", "%baseDamageRange%", "[0, 0, 40, 40]", "%baseRandomDamage%", "0",
		"%race%", "BEAST", "%clan%", "[wolf_clan, beast_clan]", "%clanRange%", "300", "%ignoredIds%", "[20121]",
	))

	header := "<br></center>Category: DROP - Rate: 70% - Iterations: x1<center>"
	firstDrops := header +
		dropRow(0, "Wolf Pelt", "90EE90", "90", "1") +
		dropRow(1, "Stem", "90EE90", "81", "1") +
		dropRow(2, "Animal Bone", "BDB76B", "50", "3") +
		dropRow(3, "Thread", "BDB76B", "10", "1") +
		dropRow(4, "Coal", "BDB76B", "6", "1") +
		dropRow(5, "Sealed Tallum Tunic of the Ancient Kingdom...", "F08080", "3.062", "1") +
		pageBar("bypass admin_info drop 1 %page%", 1, 2) +
		pageBar("bypass admin_info drop %page% 1", 1, 2)
	for _, cmd := range []string{
		"info drop", "info drop 1", "info drop 1 1",
		"info drop 0", "info drop 1 0", "info drop x", "info drop 1 -2", "info spoil 0", "info spoil 1 y",
	} {
		assertInfoPage(t, gm, cmd, defaultPage(t, firstDrops))
	}
	assertInfoPage(t, gm, "info drop 1 2", defaultPage(t, header+
		dropRow(0, "Wolf Fang", "F08080", ".5", "1 - 2")+
		strings.Repeat("<img height=41>", 5)+
		pageBar("bypass admin_info drop 1 %page%", 2, 2)+
		pageBar("bypass admin_info drop %page% 1", 1, 2)))
	// A page past the last shows the last; its links keep the page asked.
	currency := defaultPage(t, "<br></center>Category: CURRENCY - Rate: 100% - Iterations: x1<center>"+
		dropRow(0, "Adena", "90EE90", "100", "1000")+
		strings.Repeat("<img height=41>", 5)+
		pageBar("bypass admin_info drop 9 %page%", 1, 1)+
		pageBar("bypass admin_info drop %page% 1", 2, 2))
	assertInfoPage(t, gm, "info drop 9", currency)
	assertInfoPage(t, gm, "info spoil", defaultPage(t, "<br></center>Category: SPOIL - Rate: 100% - Iterations: x1<center>"+
		dropRow(0, "R: Soulshot (D)", "BDB76B", "25.5", "1 - 3")+
		strings.Repeat("<img height=41>", 5)+
		pageBar("bypass admin_info spoil 1 %page%", 1, 1)+
		pageBar("bypass admin_info spoil %page% 1", 1, 1)))

	// Aggro: empty, then the attackers most hated first with their damage.
	assertInfoPage(t, gm, "info aggro", defaultPage(t, `This NPC's AggroList is empty.<br><button value="Refresh" action="bypass -h admin_info aggro" width=65 height=19 back="L2UI_ch3.smallbutton2_over" fore="L2UI_ch3.smallbutton2">`))
	_, playerID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	wolf.AddDamageHate(liveCombatant(t, srv, gmID), 120.5, 300)
	wolf.AddDamageHate(liveCombatant(t, srv, playerID), 10, 500)
	assertInfoPage(t, gm, "info aggro", defaultPage(t, `<button value="Refresh" action="bypass -h admin_info aggro" width=65 height=19 back="L2UI_ch3.smallbutton2_over" fore="L2UI_ch3.smallbutton2">`+
		`<br><table width="280"><tr><td><font color="LEVEL">Attacker</font></td><td><font color="LEVEL">Damage</font></td><td><font color="LEVEL">Hate</font></td></tr>`+
		`<tr><td>Player</td><td>10.0</td><td>500.0</td></tr><tr><td>Admin</td><td>120.5</td><td>300.0</td></tr>`+
		`</table><img src="L2UI.SquareGray" width=280 height=1>`))

	// Desire: none queued yet (the AI task does not run here), then each
	// queued desire with its weight, in the order queued.
	refresh := `<button value="Refresh" action="bypass -h admin_info desire" width=65 height=19 back="L2UI_ch3.smallbutton2_over" fore="L2UI_ch3.smallbutton2">`
	assertInfoPage(t, gm, "info desire", defaultPage(t, "This NPC's Desires are empty.<br>"+refresh))
	wolf.AI().Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionMoveRoute, RouteName: "den", Weight: 12.5})
	wolf.AI().Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionNothing, Weight: 3})
	assertInfoPage(t, gm, "info desire", defaultPage(t, refresh+
		`<br><table width="280"><tr><td><font color="LEVEL">Type</font></td><td><font color="LEVEL">Weight</font></td></tr>`+
		`<tr><td>MOVE_ROUTE</td><td>12.5</td></tr><tr><td>NOTHING</td><td>3.0</td></tr>`+
		`</table><img src="L2UI.SquareGray" width=280 height=1>`))
}

// liveCombatant returns the online player id as the world holds it.
func liveCombatant(t *testing.T, srv *gameservertest.Server, id int32) attackable.Combatant {
	t.Helper()
	obj, ok := srv.State.Object(id)
	if !ok {
		t.Fatalf("player %d not in the world", id)
	}
	c, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("player %d is no combatant", id)
	}
	return c
}
