package admin

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// Skill fixtures of the //info skill page.
const (
	dataBiteSkill = 4001
	dataHowlSkill = 4002
)

// dataTemplateParams are the template AI parameters of dataGrocer, in the
// order its <ai> block declares them.
var dataTemplateParams = []string{
	"AttackRange", "IsAggressive", "MoveAroundSocial", "MoveAroundSocial1", "MoveAroundSocial2", "Skill02_Check_Dist",
	"Skill02_Dist_Min", "SoulShot", "SoulShotRate", "SpiritShot", "SpiritShotRate",
}

// dataGrocer is a merchant holding a skill of two types, given out of
// type order, and dataTemplateParams.
func dataGrocer() *npc.Template {
	params := make(npc.AIParams, len(dataTemplateParams))
	for i, k := range dataTemplateParams {
		params[k] = "t" + strconv.Itoa(i)
	}
	return npc.NewTemplate(npc.Template{
		ID: grocerID, TemplateID: grocerID, Type: "Merchant", Name: "Grocer Lector", Level: 20,
		HPMax: 500, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true, CorpseTime: 7,
		AIParams: params, AIParamKeys: dataTemplateParams,
	}, npc.TemplateSkills{ByType: map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeDDMagic: {ID: dataBiteSkill, Level: 1},
		npc.SkillTypeBuff:    {ID: dataHowlSkill, Level: 2},
	}})
}

// dataWolf is a monster with no skill, no AI parameter beyond the one
// that makes it lead the privates its spawn declares, and no buylist.
func dataWolf() *npc.Template {
	return &npc.Template{
		ID: wolfID, TemplateID: wolfID, Type: "Monster", Name: "Wolf", Level: 3,
		HPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true, CanBeAttacked: true, CorpseTime: 7,
		CollisionRadius: 8, CollisionHeight: 20,
	}
}

// dataTree is a Christmas tree.
func dataTree() *npc.Template {
	return &npc.Template{
		ID: treeID, TemplateID: treeID, Type: "ChristmasTree", Name: "Christmas Tree", Level: 1,
		STR: 40, DEX: 30, CON: 43, INT: 21, WIT: 20, MEN: 82,
		HPMax: 300, MPMax: 100, PAtk: 10, MAtk: 12, PDef: 30, MDef: 40, AtkSpd: 253, CritRate: 4,
		RunSpeed: 0, WalkSpeed: 0, CorpseTime: 7,
	}
}

// dataSpawnParams are the AI parameters the grocer's spawn declares, in
// that order.
var dataSpawnParams = []string{
	"AttackRange", "IsAggressive", "MoveAroundSocial", "MoveAroundSocial1", "MoveAroundSocial2", "Skill02_Probablity",
	"Skill02_Target", "Skill03_Target", "Skill03_Type", "SoulShot", "SoulShotRate", "SpiritShot", "SpiritShotRate",
}

// dataMakers are four makers: a wolf at a fixed point leading a private,
// two wolves at one of two points, a wolf that never respawns, and a
// grocer anywhere in the territory whose spawn declares dataSpawnParams.
func dataMakers(t *testing.T) *spawn.Table {
	t.Helper()
	var sets strings.Builder
	for i, k := range dataSpawnParams {
		sets.WriteString(`<set name="` + k + `" val="s` + strconv.Itoa(i) + `"/>`)
	}
	body := `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="den" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="wolf_den" territory="den" maximumNpcs="1">
		<npc id="20120" total="1" pos="200;300;30;0" respawn="90sec" respawnRand="30sec"><privates><private id="20120" weight="1" respawn="60sec"/></privates></npc>
	</npcmaker>
	<npcmaker name="wolf_pack" territory="den" maximumNpcs="2">
		<npc id="20120" total="2" pos="100;100;30;0;50%;120;140;30;0;50%" respawn="1hour"/>
	</npcmaker>
	<npcmaker name="wolf_lair" territory="den" maximumNpcs="1">
		<npc id="20120" total="1" pos="300;300;30;0" respawn="no" respawnRand="30sec"/>
	</npcmaker>
	<npcmaker name="grocer_row" territory="den" maximumNpcs="1">
		<npc id="30001" total="1" respawn="0"><ai>` + sets.String() + `</ai></npc>
	</npcmaker>
</list>`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.xml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

// bootDataAdmin boots an admin in the world with the data fixtures: the
// skill definitions, the grocer's buylists, Gludio listing the grocer, and
// the maker population of dataMakers.
func bootDataAdmin(t *testing.T) (*gameservertest.Server, int32) {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: dataBiteSkill, Level: 1, Name: "Wolf Bite", SkillType: "PDAM"},
		{ID: dataHowlSkill, Level: 2, Name: "Howl", SkillType: "NOTDONE"},
	}), gamesql.NewCharacterSkillStore(db))
	wolf := dataWolf()
	wolf.AIParams = npc.AIParams{"Party_Type": "2"}
	wolf.AIParamKeys = []string{"Party_Type"}
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithSkills(skills),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{dataGrocer(), wolf, dataTree()})),
		gameservertest.WithNpcSpawns(dataMakers(t)),
		// In load order 18, 5, 3; the hash table of four lists lists them
		// by slot: 18 (2), 3 (3), 5 (5).
		gameservertest.WithBuyLists(
			buylist.NewList(18, grocerID, nil), buylist.NewList(5, grocerID, nil),
			buylist.NewList(7, 30002, nil), buylist.NewList(3, grocerID, nil),
		),
		gameservertest.WithCastles(infoCastles(t)),
		gameservertest.WithResidences(nil, infoCastles(t)),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "npcinfo/default.htm", "npcinfo/spawn.htm", "npcinfo/stat.htm", "npcinfo/general-2.htm")),
	)
	enterWorld(t, srv.Client)
	return srv, gmID
}

// dataSpawned returns the NPCs of template id that maker spawned.
func dataSpawned(t *testing.T, srv *gameservertest.Server, id int, maker string) []placed {
	t.Helper()
	var out []placed
	for _, obj := range npcsOf(srv, id) {
		if rec, ok := srv.NpcSpawns.SpawnOf(obj.ObjectID()); ok && rec.Maker == maker {
			out = append(out, obj.(placed))
		}
	}
	return out
}

// paramRows is one row per key of keys, its value named prefix plus the
// key's place in declared.
func paramRows(keys, declared []string, prefix string) string {
	var b strings.Builder
	for _, k := range keys {
		for i, d := range declared {
			if d == k {
				b.WriteString(`<font color="LEVEL">[` + k + `]</font> ` + prefix + strconv.Itoa(i) + "<br1>")
			}
		}
	}
	return b.String()
}

// TestAdminInfoNpcSkillShopAndParamPages pins AdminInfo.java's skill, shop
// and AI parameter pages (sendSkillInfos, sendShopInfos, sendAiInfos pages
// 1 and 2): each typed skill under its type in type order, one not done
// greyed; the castle tax rate, then the NPC's buylists in the order the
// buylist hash table holds them; and the template's and the spawn's AI
// parameters in the order their memos list them, with the empty texts of
// an NPC holding none and of one no spawn placed.
func TestAdminInfoNpcSkillShopAndParamPages(t *testing.T) {
	t.Parallel()
	srv, _ := bootDataAdmin(t)
	gm := srv.Client
	gludio, ok := srv.Castles.Get(1)
	if !ok {
		t.Fatal("no Gludio")
	}
	gludio.SetCurrentTaxPercent(10, false)

	grocers := dataSpawned(t, srv, grocerID, "grocer_row")
	if len(grocers) != 1 {
		t.Fatalf("grocer_row grocers = %d, want 1", len(grocers))
	}
	grocer := grocers[0].(*npc.Folk)
	exchange(t, gm, encodeAction(grocer.ObjectID()))

	assertInfoPage(t, gm, "info skill", defaultPage(t,
		`<br><font color="LEVEL">BUFF</font><br1><font color="777777">Howl</font> [4002-2]<br1>`+
			`<br><font color="LEVEL">DD_MAGIC</font><br1>Wolf Bite [4001-1]<br1>`))
	assertInfoPage(t, gm, "info shop", defaultPage(t, `Tax rate: 10%<br><table width="100%">`+
		`<tr><td><a action="bypass -h admin_buy 18 1">Buylist id: 18</a></td></tr>`+
		`<tr><td><a action="bypass -h admin_buy 3 1">Buylist id: 3</a></td></tr>`+
		`<tr><td><a action="bypass -h admin_buy 5 1">Buylist id: 5</a></td></tr></table>`))

	// The order of each memo, as the reference hash table gives it: the
	// template's is a copy of the memo it was loaded into, sized for its
	// eleven keys up front; the spawn's resized at its twelfth key.
	templateOrder := []string{
		"MoveAroundSocial2", "MoveAroundSocial1", "SoulShotRate", "AttackRange", "Skill02_Dist_Min", "SpiritShotRate",
		"Skill02_Check_Dist", "SpiritShot", "MoveAroundSocial", "IsAggressive", "SoulShot",
	}
	spawnOrder := []string{
		"MoveAroundSocial2", "SoulShotRate", "MoveAroundSocial1", "Skill03_Type", "SpiritShotRate", "AttackRange", "SpiritShot",
		"MoveAroundSocial", "Skill02_Probablity", "IsAggressive", "Skill03_Target", "SoulShot", "Skill02_Target",
	}
	templateTabs := strings.Replace(aiTabs(1), "[Template]", "[Template]</a>", 1)
	assertInfoPage(t, gm, "info ai 1", defaultPage(t, templateTabs+paramRows(templateOrder, dataTemplateParams, "t")))
	assertInfoPage(t, gm, "info ai 2", defaultPage(t, aiTabs(2)+paramRows(spawnOrder, dataSpawnParams, "s")))

	// A wolf: no skill, no buylist, no template parameter beyond its
	// party type, and a spawn declaring none.
	wolves := dataSpawned(t, srv, wolfID, "wolf_pack")
	if len(wolves) != 2 {
		t.Fatalf("wolf_pack wolves = %d, want 2", len(wolves))
	}
	exchange(t, gm, encodeAction(wolves[0].(*npc.Hostile).ObjectID()))
	assertInfoPage(t, gm, "info skill", defaultPage(t, "This NPC doesn't hold any skill."))
	assertInfoPage(t, gm, "info shop", defaultPage(t, "This NPC doesn't hold any buyList."))
	assertInfoPage(t, gm, "info ai 1", defaultPage(t, templateTabs+`<font color="LEVEL">[Party_Type]</font> 2<br1>`))
	assertInfoPage(t, gm, "info ai 2", defaultPage(t, aiTabs(2)+"This NPC Spawn doesn't hold any memos."))

	// A grocer no spawn placed has no spawn to read.
	loose := srv.SpawnFolkNPCAt(t, dataGrocer(), location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
	drain(t, gm)
	exchange(t, gm, encodeAction(loose.ObjectID()))
	assertInfoPage(t, gm, "info ai 2", defaultPage(t, aiTabs(2)+"This NPC doesn't have any Spawn."))
}

// dataSpawnPage is npcinfo/spawn.htm filled with pairs.
func dataSpawnPage(t *testing.T, pairs ...string) string {
	t.Helper()
	return shippedPage(t, "npcinfo/spawn.htm", pairs...)
}

// noSpawnFields are the spawn fields of an NPC no spawn placed.
func noSpawnFields() []string {
	none := "<font color=FF0000>--</font>"
	var out []string
	for _, key := range []string{"%spawn%", "%spawndesc%", "%spawninfo%", "%loc2d%", "%loc3d%", "%resp%", "%rand_resp%", "%privates%"} {
		out = append(out, key, none)
	}
	return out
}

// whereabouts are the %loc% and %dist% fields of an NPC standing at p,
// seen by a GM at gm.
func whereabouts(p, gm placed) []string {
	x, y, z := p.Position()
	gx, gy, gz := gm.Position()
	dx, dy, dz := float64(x-gx), float64(y-gy), float64(z-gz)
	return []string{
		"%loc%", strconv.Itoa(x) + " " + strconv.Itoa(y) + " " + strconv.Itoa(z),
		"%dist%", strconv.Itoa(int(math.Sqrt(dx*dx + dy*dy + dz*dz))),
	}
}

var spawnInfoLoc = regexp.MustCompile(`loc: (-?\d+), (-?\d+), (-?\d+), (-?\d+)`)

// TestAdminInfoNpcSpawnPage pins AdminInfo.java's spawn page
// (sendSpawnInfos): a maker's NPC by its maker, linked, and its declared
// positions, a delay that never respawns clamping its random part to
// -1s; a master with its privates and one of them, by its master
// and a point drawn around it; a //spawn NPC and a tree by their point and
// the respawn delay //spawn declared; and an NPC no spawn placed, every
// spawn field struck out.
func TestAdminInfoNpcSpawnPage(t *testing.T) {
	t.Parallel()
	srv, gmID := bootDataAdmin(t)
	gm := srv.Client
	admin := liveCombatant(t, srv, gmID)

	// The master of wolf_den, at its fixed point, and its private.
	var master, private *npc.Hostile
	for _, w := range dataSpawned(t, srv, wolfID, "wolf_den") {
		master = w.(*npc.Hostile)
	}
	for _, obj := range npcsOf(srv, wolfID) {
		if rec, ok := srv.NpcSpawns.SpawnOf(obj.ObjectID()); ok && rec.MasterID != 0 {
			private = obj.(*npc.Hostile)
		}
	}
	if master == nil || private == nil {
		t.Fatalf("wolf_den master %v, private %v; want both", master, private)
	}
	exchange(t, gm, encodeAction(master.ObjectID()))
	assertInfoPage(t, gm, "info spawn", dataSpawnPage(t, append(whereabouts(master, admin),
		"%corpse%", " 7s",
		"%spawn%", "MultiSpawn [id=20120]",
		"%spawndesc%", `<a action="bypass -h admin_maker wolf_den">NpcMaker: wolf_den</a>`,
		"%spawninfo%", "loc: fixed 200, 300, 30",
		"%loc2d%", "0", "%loc3d%", "0",
		"%resp%", " 1m 30s", "%rand_resp%", " 30s", "%privates%", "true",
		"%minion%", "I'm a master, holding 1 crappy minions.",
	)...))

	// The private: its spawn point is drawn around the master each time,
	// facing the way the master faces.
	exchange(t, gm, encodeAction(private.ObjectID()))
	page := infoPage(t, gm, "info spawn")
	m := spawnInfoLoc.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("private spawn page has no spawn point: %q", page)
	}
	px, _ := strconv.Atoi(m[1])
	py, _ := strconv.Atoi(m[2])
	mx, my, _ := master.Position()
	if d := math.Hypot(float64(px-mx), float64(py-my)); d > 100+8+8 || m[4] != strconv.Itoa(master.Heading()) {
		t.Fatalf("private spawn point %s is %.0f from the master facing %d; want within 116, facing the master's way", m[0], d, master.Heading())
	}
	if want := dataSpawnPage(t, append(whereabouts(private, admin),
		"%corpse%", " 7s",
		"%spawn%", "Spawn [id=20120]",
		"%spawndesc%", "Master: Wolf [objId="+strconv.Itoa(int(master.ObjectID()))+"]",
		"%spawninfo%", m[0],
		"%loc2d%", "0", "%loc3d%", "0",
		"%resp%", " 1m", "%rand_resp%", " 0s", "%privates%", "false",
		"%minion%", "I'm a crappy minion, my master <font color=LEVEL>Wolf</font> holds 1 minions.",
	)...); page != want {
		t.Fatalf("private spawn page =\n %q\nwant\n %q", page, want)
	}

	// A wolf of wolf_pack: one of two points, no privates.
	pack := dataSpawned(t, srv, wolfID, "wolf_pack")[0]
	exchange(t, gm, encodeAction(pack.(*npc.Hostile).ObjectID()))
	assertInfoPage(t, gm, "info spawn", dataSpawnPage(t, append(whereabouts(pack, admin),
		"%corpse%", " 7s",
		"%spawn%", "MultiSpawn [id=20120]",
		"%spawndesc%", `<a action="bypass -h admin_maker wolf_pack">NpcMaker: wolf_pack</a>`,
		"%spawninfo%", "loc: fixed random 1 of 2",
		"%loc2d%", "0", "%loc3d%", "0",
		"%resp%", "1h", "%rand_resp%", " 0s", "%privates%", "false",
		"%minion%", "I'm a regular NPC, as the most random BR/RU leaker.",
	)...))

	// The wolf of wolf_lair never respawns: its delay floors at zero, its
	// random part is clamped against the declared -1s.
	lair := dataSpawned(t, srv, wolfID, "wolf_lair")[0]
	exchange(t, gm, encodeAction(lair.(*npc.Hostile).ObjectID()))
	assertInfoPage(t, gm, "info spawn", dataSpawnPage(t, append(whereabouts(lair, admin),
		"%corpse%", " 7s",
		"%spawn%", "MultiSpawn [id=20120]",
		"%spawndesc%", `<a action="bypass -h admin_maker wolf_lair">NpcMaker: wolf_lair</a>`,
		"%spawninfo%", "loc: fixed 300, 300, 30",
		"%loc2d%", "0", "%loc3d%", "0",
		"%resp%", " 0s", "%rand_resp%", " -1s", "%privates%", "false",
		"%minion%", "I'm a regular NPC, as the most random BR/RU leaker.",
	)...))

	// The grocer of grocer_row, anywhere in its territory.
	grocer := dataSpawned(t, srv, grocerID, "grocer_row")[0]
	exchange(t, gm, encodeAction(grocer.(*npc.Folk).ObjectID()))
	assertInfoPage(t, gm, "info spawn", dataSpawnPage(t, append(whereabouts(grocer, admin),
		"%corpse%", " 7s",
		"%spawn%", "MultiSpawn [id=30001]",
		"%spawndesc%", `<a action="bypass -h admin_maker grocer_row">NpcMaker: grocer_row</a>`,
		"%spawninfo%", "loc: anywhere",
		"%loc2d%", "0", "%loc3d%", "0",
		"%resp%", " 0s", "%rand_resp%", " 0s", "%privates%", "false",
		"%minion%", "I'm a regular NPC, as the most random BR/RU leaker.",
	)...))

	// //spawn places a wolf and a tree on the GM, declaring 90 and the
	// default 60 seconds.
	exchange(t, gm, encodeBuildCmd("spawn 20120 90"))
	exchange(t, gm, encodeBuildCmd("spawn 13006"))
	srv.Settle(t)
	drain(t, gm)
	var fixed *npc.Hostile
	for _, obj := range npcsOf(srv, wolfID) {
		if rec, ok := srv.NpcSpawns.SpawnOf(obj.ObjectID()); ok && rec.Fixed {
			fixed = obj.(*npc.Hostile)
		}
	}
	trees := npcsOf(srv, treeID)
	if fixed == nil || len(trees) != 1 {
		t.Fatalf("//spawn placed wolf %v, %d trees; want one of each", fixed, len(trees))
	}
	point := func(p placed) string {
		x, y, z := p.Position()
		return strconv.Itoa(x) + ", " + strconv.Itoa(y) + ", " + strconv.Itoa(z) + ", " + strconv.Itoa(p.Heading())
	}
	exchange(t, gm, encodeAction(fixed.ObjectID()))
	assertInfoPage(t, gm, "info spawn", dataSpawnPage(t, append(whereabouts(fixed, admin),
		"%corpse%", " 7s",
		"%spawn%", "Spawn [id=20120]",
		"%spawndesc%", "Location: "+point(fixed),
		"%spawninfo%", "loc: "+point(fixed),
		"%loc2d%", "0", "%loc3d%", "0",
		"%resp%", " 1m 30s", "%rand_resp%", " 0s", "%privates%", "false",
		"%minion%", "I'm a regular NPC, as the most random BR/RU leaker.",
	)...))
	assertInfoPage(t, gm, "info ai 2", defaultPage(t, aiTabs(2)+"This NPC Spawn doesn't hold any memos."))
	tree := trees[0].(*npc.Decoration)
	retarget(t, srv, gm, gmID, tree)
	assertInfoPage(t, gm, "info spawn", dataSpawnPage(t, append(whereabouts(tree, admin),
		"%corpse%", " 7s",
		"%spawn%", "Spawn [id=13006]",
		"%spawndesc%", "Location: "+point(tree),
		"%spawninfo%", "loc: "+point(tree),
		"%loc2d%", "0", "%loc3d%", "0",
		"%resp%", " 1m", "%rand_resp%", " 0s", "%privates%", "false",
		"%minion%", "I'm a regular NPC, as the most random BR/RU leaker.",
	)...))
	assertInfoPage(t, gm, "info ai 2", defaultPage(t, aiTabs(2)+"This NPC Spawn doesn't hold any memos."))

	// A grocer no spawn placed.
	loose := srv.SpawnFolkNPCAt(t, dataGrocer(), location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
	drain(t, gm)
	exchange(t, gm, encodeAction(loose.ObjectID()))
	assertInfoPage(t, gm, "info spawn", dataSpawnPage(t, append(append(whereabouts(loose, admin), "%corpse%", " 7s"), append(noSpawnFields(),
		"%minion%", "I'm a regular NPC, as the most random BR/RU leaker.")...)...))
}

// TestAdminInfoNpcFolkDesiresAndTreePages pins AdminInfo.java's desire and
// stat pages beyond attackable NPCs: a civilian NPC lists the desires its
// AI holds, a tree, which runs no AI, holds none and shows its stats.
func TestAdminInfoNpcFolkDesiresAndTreePages(t *testing.T) {
	t.Parallel()
	srv, gmID := bootDataAdmin(t)
	gm := srv.Client
	refresh := `<button value="Refresh" action="bypass -h admin_info desire" width=65 height=19 back="L2UI_ch3.smallbutton2_over" fore="L2UI_ch3.smallbutton2">`

	grocer := dataSpawned(t, srv, grocerID, "grocer_row")[0].(*npc.Folk)
	exchange(t, gm, encodeAction(grocer.ObjectID()))
	grocer.Desires().Clear()
	assertInfoPage(t, gm, "info desire", defaultPage(t, "This NPC's Desires are empty.<br>"+refresh))
	grocer.Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionSocial, Weight: 50})
	grocer.Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionNothing, Weight: 2.5})
	assertInfoPage(t, gm, "info desire", defaultPage(t, refresh+
		`<br><table width="280"><tr><td><font color="LEVEL">Type</font></td><td><font color="LEVEL">Weight</font></td></tr>`+
		`<tr><td>SOCIAL</td><td>50.0</td></tr><tr><td>NOTHING</td><td>2.5</td></tr>`+
		`</table><img src="L2UI.SquareGray" width=280 height=1>`))

	exchange(t, gm, encodeBuildCmd("spawn 13006"))
	srv.Settle(t)
	drain(t, gm)
	tree := npcsOf(srv, treeID)[0].(*npc.Decoration)
	retarget(t, srv, gm, gmID, tree)
	assertInfoPage(t, gm, "info desire", defaultPage(t, "This NPC's Desires are empty.<br>"+refresh))
	s := tree.Status()
	itoa := strconv.Itoa
	assertInfoPage(t, gm, "info stat", shippedPage(t, "npcinfo/stat.htm",
		"%hp%", itoa(int(s.MaxHPValue())), "%hpmax%", itoa(int(s.MaxHPValue())),
		"%mp%", itoa(int(s.MaxMPValue())), "%mpmax%", itoa(int(s.MaxMPValue())),
		"%patk%", itoa(int(s.PAtk())), "%matk%", itoa(int(s.MAtk())),
		"%pdef%", itoa(int(s.PDef())), "%mdef%", itoa(int(s.MDef())),
		"%accu%", itoa(int(s.CalcStat(stat.AccuracyCombat, 0))), "%evas%", itoa(s.Evasion()),
		"%crit%", itoa(int(s.CalcStat(stat.CriticalRate, 4))), "%rspd%", "0",
		"%aspd%", itoa(s.AttackSpeed()), "%cspd%", itoa(s.MagicAttackSpeed()),
		"%str%", "40", "%dex%", "30", "%con%", "43", "%int%", "21", "%wit%", "20", "%men%", "82",
		"%ele_fire%", "1.0", "%ele_water%", "1.0", "%ele_wind%", "1.0", "%ele_earth%", "1.0", "%ele_holy%", "1.0", "%ele_dark%", "1.0",
	))
	if s.MaxHPValue() <= 300 {
		t.Fatalf("tree max HP %v; want the template's 300 raised by its CON", s.MaxHPValue())
	}
}

// TestAdminInfoNpcResidenceIsTheTemplates pins //info 2 on NPCs whose
// maker names no residence: the castle listing the template, else none.
func TestAdminInfoNpcResidenceIsTheTemplates(t *testing.T) {
	t.Parallel()
	srv, _ := bootDataAdmin(t)
	gm := srv.Client
	exchange(t, gm, encodeAction(dataSpawned(t, srv, grocerID, "grocer_row")[0].(*npc.Folk).ObjectID()))
	assertInfoPage(t, gm, "info 2", shippedPage(t, "npcinfo/general-2.htm",
		"%isUndying%", "false", "%canBeAttacked%", "false", "%isNoSleepMode%", "false",
		"%aggroRange%", "0", "%canMove%", "true", "%isSeedable%", "false", "%residence%", "Gludio"))
	exchange(t, gm, encodeAction(dataSpawned(t, srv, wolfID, "wolf_pack")[0].(*npc.Hostile).ObjectID()))
	assertInfoPage(t, gm, "info 2", shippedPage(t, "npcinfo/general-2.htm",
		"%isUndying%", "false", "%canBeAttacked%", "true", "%isNoSleepMode%", "false",
		"%aggroRange%", "0", "%canMove%", "true", "%isSeedable%", "false", "%residence%", "none"))
}
