package quest

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/newbiehelper"
	featuretutorial "github.com/fatal10110/acis_golang/internal/gameserver/script/feature/tutorial"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

const newbieHelper = "NewbieHelper"

// The newbie helper's NPCs and items.
const (
	carlID        = 30009 // Talking Island newbie helper
	roienID       = 30008 // Talking Island grand master guide
	cnacelotID    = 30598 // Talking Island newbie guide
	roxxyGateID   = 30006 // a gatekeeper the guides mark on the radar
	gremlinID     = 18342 // tutorial gremlin
	recommend01   = 1067
	beginnerShots = 5789
	blueGem       = 6353
	travelToken   = 8542
)

// newbieHelperPages are the datapack's newbie helper pages, keyed as the
// page cache looks them up, and the file each page's text belongs to.
func newbieHelperPages(t *testing.T) (pages map[string]string, files map[string]string) {
	t.Helper()
	dir := datapack.Path(t, "data", "html", "script", "feature", "NewbieHelper")
	pages, files = map[string]string{}, map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		pages["script/feature/NewbieHelper/"+rel] = string(data)
		page := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
		if !strings.HasSuffix(page, "\n") {
			page += "\n"
		}
		files[page] = rel
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return pages, files
}

// newbieWorld is a booted human fighter with a started
// tutorial quest, in the world beside the newbie helper NPCs.
type newbieWorld struct {
	*dialogWorld
	files map[string]string
	// messages renders system messages too, by id.
	messages bool
}

// bootNewbieHelper boots the character at level with the tutorial and the
// newbie helper registered, the helper's datapack pages, the shipped skills
// and items, buffs as the newbie buff table and roll as every script draw.
// The tutorial state is seeded STARTED and done with its enter-world steps.
// opts add to the boot.
func bootNewbieHelper(t *testing.T, level, roll int, buffs []modelskill.NewbieBuff, opts ...gameservertest.Option) *newbieWorld {
	t.Helper()
	datapack.Require(t)
	skills, shipped := shippedSeamData()
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{recommend01, beginnerShots, blueGem, travelToken} {
		tmpl, ok := shipped.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	pages, files := newbieHelperPages(t)
	pages["default/30008.htm"] = "<html><body>Roien's chat window</body></html>"
	table := modelskill.NewNewbieBuffTable(buffs)
	db := sqltest.SharedDB(t)
	w := bootSeamQuest(t, level, pages, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, tutorial, "<state>", val("STARTED")},
			journalRow{objID, tutorial, "onlyone", val("1")})
	}, append([]gameservertest.Option{
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), skills, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithScriptRand(func(int) int { return roll }),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{carlID: script.KindFolk, roienID: script.KindFolk, cnacelotID: script.KindFolk, gremlinID: script.KindHostile},
			[]script.Listing{{Path: "script.feature.NewbieHelper"}, {Path: "script.feature.Tutorial"}},
			script.Catalog{
				"script.feature.NewbieHelper": func() script.Script { return newbiehelper.New(table) },
				"script.feature.Tutorial":     featuretutorial.New,
			},
		),
	}, opts...)...)
	w.srv.ReadQueued(t, w.srv.Client)
	return &newbieWorld{dialogWorld: w, files: files}
}

// spawnGuide places the human civilian NPC npcID at offset from the
// player.
func (w *newbieWorld) spawnGuide(t *testing.T, npcID, dx, dy int) *npc.Folk {
	t.Helper()
	tmpl := gameservertest.FolkTemplate("Folk", npcID)
	tmpl.Race = npc.RaceHuman
	f := w.srv.SpawnFolkNPCAt(t, tmpl, location.Location{X: w.at.X + dx, Y: w.at.Y + dy, Z: w.at.Z})
	w.srv.ReadQueued(t, w.srv.Client)
	return f
}

// progression is the player's experience, skill points and level.
func (w *newbieWorld) progression(t *testing.T) player.Progression {
	t.Helper()
	var out player.Progression
	onCharacter(t, w.srv, w.player, func(c *player.Character) { out = c.ProgressionValues() })
	return out
}

// talk interacts with the NPC objectID and renders what it answers.
func (w *newbieWorld) talk(t *testing.T, objectID int32) []string {
	t.Helper()
	return w.render(t, w.interact(t, objectID))
}

// event sends the newbie helper event ev through the player's last quest
// NPC and renders what it answers.
func (w *newbieWorld) event(t *testing.T, ev string) []string {
	t.Helper()
	w.offerAll(t)
	w.srv.Client.Send(encodeBypass("Quest " + newbieHelper + " " + ev))
	return w.render(t, w.srv.ReadQueued(t, w.srv.Client))
}

// after advances the clock by d and renders what the player was sent.
func (w *newbieWorld) after(t *testing.T, d time.Duration) []string {
	t.Helper()
	w.srv.Advance(t, d)
	return w.render(t, w.srv.ReadQueued(t, w.srv.Client))
}

// render writes the frames a newbie helper flow is judged by, one line
// each, in order: pages (by datapack file), ActionFailed, sounds, tutorial
// question marks, radar controls, item drops and skill casts. Other frames
// are left out.
func (w *newbieWorld) render(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		d := frameReader{b: f[1:]}
		switch f[0] {
		case serverpackets.OpcodeNpcHtmlMessage:
			d.int32()
			html := d.str()
			if file, ok := w.files[html]; ok {
				out = append(out, "page "+file)
			} else {
				out = append(out, "page "+html)
			}
		case serverpackets.OpcodeActionFailed:
			out = append(out, "ActionFailed")
		case serverpackets.OpcodeSystemMessage:
			if w.messages {
				out = append(out, fmt.Sprintf("message %d", d.int32()))
			}
		case serverpackets.OpcodePlaySound:
			d.int32()
			out = append(out, "sound "+d.str())
		case serverpackets.OpcodeTutorialShowQuestionMark:
			out = append(out, fmt.Sprintf("question mark %d", d.int32()))
		case serverpackets.OpcodeRadarControl:
			out = append(out, fmt.Sprintf("radar %d %d %d,%d,%d", d.int32(), d.int32(), d.int32(), d.int32(), d.int32()))
		case serverpackets.OpcodeDropItem:
			d.int32()
			d.int32()
			id := d.int32()
			d.int32()
			d.int32()
			d.int32()
			d.int32()
			out = append(out, fmt.Sprintf("drop %d x%d", id, d.int32()))
		case serverpackets.OpcodeMagicSkillUse:
			caster, target := d.int32(), d.int32()
			id, level := d.int32(), d.int32()
			who := func(obj int32) string {
				if obj == w.player {
					return "player"
				}
				return fmt.Sprint(obj)
			}
			out = append(out, fmt.Sprintf("cast %d-%d %s->%s", id, level, who(caster), who(target)))
		}
	}
	return out
}

// frameReader reads a server packet's little-endian fields.
type frameReader struct{ b []byte }

func (r *frameReader) int32() int32 {
	v := int32(binary.LittleEndian.Uint32(r.b))
	r.b = r.b[4:]
	return v
}

func (r *frameReader) str() string {
	var units []uint16
	for {
		u := binary.LittleEndian.Uint16(r.b)
		r.b = r.b[2:]
		if u == 0 {
			return string(utf16.Decode(units))
		}
		units = append(units, u)
	}
}

func expectLines(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if len(want) == 0 {
		want = nil
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s:\n got %q\nwant %q", what, got, want)
	}
}

func expectVars(t *testing.T, w *newbieWorld, quest string, want map[string]string) {
	t.Helper()
	if got := w.srv.QuestVars(t, w.player, quest); !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", quest, got, want)
	}
}

func expectCount(t *testing.T, w *newbieWorld, itemID int32, want int) {
	t.Helper()
	if got := w.srv.PlayerItemCount(t, w.player, itemID); got != want {
		t.Fatalf("item %d count = %d, want %d", itemID, got, want)
	}
}

// TestNewbieHelperRecommendationShotsAndRadar walks a human fighter
// through the Talking Island newbie helper and grand master: the guide
// opens its own chat window before the helper is met; the helper's first
// talk starts the helper and arms the 30 s voice; the tutorial gremlin's
// death three seconds later shows question mark 3 and, under the 25 %
// roll, drops a Blue Gemstone for the player; with the gemstone the helper
// takes it, gives the recommendation and the beginner soulshots with their
// voice, and its timer plays the class voice. The guide then takes the
// recommendation, marks itself on the radar, rewards 50 SP and the shots
// once, and its 60 s timer shows question mark 7; the helper then has
// nothing more to say.
func TestNewbieHelperRecommendationShotsAndRadar(t *testing.T) {
	t.Parallel()
	w := bootNewbieHelper(t, 1, 0, nil)
	carl := w.spawnGuide(t, carlID, 20, 0)
	roien := w.spawnGuide(t, roienID, 0, 20)

	expectLines(t, "guide before the helper", w.talk(t, roien.ObjectID()),
		"ActionFailed", "page <html><body>Roien's chat window</body></html>\n", "ActionFailed", "ActionFailed")
	expectVars(t, w, newbieHelper, map[string]string{"<state>": "CREATED"})

	expectLines(t, "helper, first talk", w.talk(t, carl.ObjectID()), "ActionFailed", "page newbiehelper_fig_01.htm", "ActionFailed")
	expectVars(t, w, newbieHelper, map[string]string{"<state>": "STARTED", "step": "1"})
	expectVars(t, w, tutorial, map[string]string{"<state>": "STARTED", "onlyone": "1", "Ex": "0"})
	expectLines(t, "before 30 s", w.after(t, 29*time.Second))
	expectLines(t, "helper timer", w.after(t, time.Second), "sound tutorial_voice_009a")
	expectVars(t, w, tutorial, map[string]string{"<state>": "STARTED", "onlyone": "1", "Ex": "1"})

	gremlinTmpl := timakArcher()
	gremlinTmpl.ID, gremlinTmpl.TemplateID, gremlinTmpl.Name = gremlinID, gremlinID, "Gremlin"
	gremlin := w.srv.SpawnHostileNPCTemplateAt(t, gremlinTmpl, location.Location{X: w.at.X - 20, Y: w.at.Y, Z: w.at.Z})
	w.srv.ReadQueued(t, w.srv.Client)
	killer := playerCombatant(t, w.srv, w.player)
	onNPCQueue(t, gremlin, func() { gremlin.Kill(killer) })
	w.srv.ReadQueued(t, w.srv.Client)
	expectLines(t, "gremlin dying", w.after(t, 3*time.Second),
		"question mark 3", "sound tutorial_voice_011", fmt.Sprintf("drop %d x1", blueGem), "sound "+script.SoundTutorial)
	expectVars(t, w, tutorial, map[string]string{"<state>": "STARTED", "onlyone": "1", "Ex": "2"})

	// The gemstone is reserved for the player, who picks it up; the
	// tutorial hears the pickup.
	var gem item.GroundSnapshot
	for _, g := range w.srv.GroundItems.Snapshots(nil) {
		if g.TemplateID == blueGem {
			gem = g
		}
	}
	if gem.ObjectID == 0 || gem.OwnerID != w.player {
		t.Fatalf("gemstone on the ground = %+v, want one reserved for %d", gem, w.player)
	}
	w.srv.Client.Send(encodeTutorialAction(gem.ObjectID, w.at))
	w.srv.AdvanceUntil(t, "gemstone picked up", func() bool { return w.srv.PlayerItemCount(t, w.player, blueGem) == 1 })
	w.srv.Advance(t, time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	expectVars(t, w, tutorial, map[string]string{"<state>": "STARTED", "onlyone": "1", "Ex": "2", "Gemstone": "1"})
	w.at.X, w.at.Y, w.at.Z = w.srv.PlayerPosition(t, w.player)
	// The helper is still the player's target: one click talks.
	w.srv.Client.Send(encodeTutorialAction(carl.ObjectID(), w.at))
	expectLines(t, "helper with the gemstone", w.render(t, w.srv.ReadQueued(t, w.srv.Client)), "ActionFailed", "sound tutorial_voice_026", "page 30009-03.htm", "ActionFailed")
	expectCount(t, w, blueGem, 0)
	expectCount(t, w, recommend01, 1)
	expectCount(t, w, beginnerShots, 200)
	expectVars(t, w, newbieHelper, map[string]string{"<state>": "STARTED", "step": "2"})
	expectVars(t, w, tutorial, map[string]string{"<state>": "STARTED", "onlyone": "1", "Ex": "3", "ucMemo": "3", "Gemstone": "1"})
	expectLines(t, "helper timer again", w.after(t, 30*time.Second), "sound tutorial_voice_010a")
	expectVars(t, w, tutorial, map[string]string{"<state>": "STARTED", "onlyone": "1", "Ex": "4", "ucMemo": "3", "Gemstone": "1"})

	expectLines(t, "guide with the recommendation", w.talk(t, roien.ObjectID()), "ActionFailed", "page 30008-02.htm", "ActionFailed")
	before := w.progression(t)
	expectLines(t, "recommendation handed in", w.event(t, "30008-03.htm"),
		"radar 2 2 -84058,243239,-3730", "radar 0 1 -84058,243239,-3730", "sound tutorial_voice_026", "page 30008-03.htm", "ActionFailed")
	if after := w.progression(t); after.SP-before.SP != 50 || after.Exp != before.Exp {
		t.Fatalf("gained %d exp and %d sp, want 0 and 50", after.Exp-before.Exp, after.SP-before.SP)
	}
	expectCount(t, w, recommend01, 0)
	expectCount(t, w, beginnerShots, 400)
	expectVars(t, w, newbieHelper, map[string]string{"<state>": "STARTED", "onlyone": "1"})

	expectLines(t, "recommendation again", w.event(t, "30008-03.htm"),
		"radar 2 2 -84058,243239,-3730", "radar 0 1 -84058,243239,-3730", "page 30008-03.htm", "ActionFailed")
	expectCount(t, w, beginnerShots, 400)

	expectLines(t, "grand master timer", w.after(t, 60*time.Second),
		"question mark 7", "sound "+script.SoundTutorial, "sound tutorial_voice_025")
	expectLines(t, "helper after the guide", w.talk(t, carl.ObjectID()), "ActionFailed", "page newbiehelper_03.htm", "ActionFailed")
}

// TestNewbieGuideShotsTokensAdviceAndRadar talks to a Talking Island
// newbie guide: its first talk gives the beginner shots and twelve travel
// tokens once and completes the helper; it answers advice by the player's
// level, marks a village NPC on the radar, and opens the travel token
// teleport list. A NewbieToken<id> event finds no place in the reference's
// table, so the token is kept and the player stays.
func TestNewbieGuideShotsTokensAdviceAndRadar(t *testing.T) {
	t.Parallel()
	w := bootNewbieHelper(t, 1, 0, nil)
	guide := w.spawnGuide(t, cnacelotID, 20, 0)
	w.spawnGuide(t, roxxyGateID, 300, 300)

	expectLines(t, "guide, first talk", w.talk(t, guide.ObjectID()), "ActionFailed", "sound tutorial_voice_026", "page 30598.htm", "ActionFailed")
	expectCount(t, w, beginnerShots, 200)
	expectCount(t, w, travelToken, 12)
	expectVars(t, w, newbieHelper, map[string]string{"<state>": "COMPLETED"})
	expectLines(t, "guide, second talk", w.talk(t, guide.ObjectID()), "ActionFailed", "page 30598.htm", "ActionFailed")
	expectCount(t, w, beginnerShots, 200)
	expectCount(t, w, travelToken, 12)

	expectLines(t, "advice", w.event(t, "AskAdvice"), "page human/guide_human_cnacelot_f05.htm", "ActionFailed")
	expectLines(t, "location", w.event(t, fmt.Sprintf("NpcLocationInfo %d", roxxyGateID)),
		fmt.Sprintf("radar 2 2 %d,%d,%d", w.at.X+300, w.at.Y+300, w.at.Z),
		fmt.Sprintf("radar 0 1 %d,%d,%d", w.at.X+300, w.at.Y+300, w.at.Z),
		"page newbie_guide_move_to_loc.htm", "ActionFailed")
	expectLines(t, "location of an NPC not listed", w.event(t, fmt.Sprintf("NpcLocationInfo %d", carlID)))

	x, y, z := w.srv.PlayerPosition(t, w.player)
	expectLines(t, "token to a guide", w.event(t, fmt.Sprintf("NewbieToken%d", cnacelotID)))
	expectCount(t, w, travelToken, 12)
	if nx, ny, nz := w.srv.PlayerPosition(t, w.player); nx != x || ny != y || nz != z {
		t.Fatalf("player moved to %d,%d,%d", nx, ny, nz)
	}
}

// TestNewbieGuideBuffsAndBlessing: a guide's support magic tells a player
// below the lowest buff level so; at level 8 it casts each valid buff a
// second after the one before, the player casting on itself and feeling
// its effect; the blessing is cast a second after it is asked for (its
// effect does not start, so it is not felt).
func TestNewbieGuideBuffsAndBlessing(t *testing.T) {
	t.Parallel()
	buffs := []modelskill.NewbieBuff{
		modelskill.NewNewbieBuff(4322, 1, 8, 24, false),
		modelskill.NewNewbieBuff(4323, 1, 8, 23, false),
		modelskill.NewNewbieBuff(4328, 1, 8, 22, true),
	}
	w := bootNewbieHelper(t, 7, 0, buffs)
	guide := w.spawnGuide(t, cnacelotID, 20, 0)
	w.talk(t, guide.ObjectID())
	w.messages = true

	expectLines(t, "support below the lowest level", w.event(t, "SupportMagic"), "page guide_for_newbie002.htm", "ActionFailed")

	w.srv.AddPlayerLevel(t, w.player, 1)
	w.srv.ReadQueued(t, w.srv.Client)
	expectLines(t, "first buff", w.event(t, "SupportMagic"), "cast 4322-1 player->player", "message 110")
	expectLines(t, "second buff", w.after(t, time.Second), "cast 4323-1 player->player", "message 110")
	expectLines(t, "no third buff", w.after(t, time.Second))

	expectLines(t, "blessing asked", w.event(t, "GiveBlessing"))
	expectLines(t, "blessing", w.after(t, time.Second), "cast 5182-1 player->player")
}
