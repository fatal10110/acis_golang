package quest

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	quest001 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// The dialog goldens' NPCs and scripts.
const (
	darinID   = 30048
	roxxyID   = 30006
	baulroID  = 30033
	tallothID = 30141
	// judgeID stands for the golden's Black Judge: a civilian NPC one
	// script holds the first talk of.
	judgeID = 30981
	// guardID is a hostile guard a quest talks through.
	guardID = 30039
	// silentID is a civilian NPC whose first talk answers nothing.
	silentID = 30982
	// judgeGuardID is a guard one script holds the first talk of.
	judgeGuardID = 30040
	// silentGuardID is a starting village's guard, which an interact
	// leaves silent.
	silentGuardID = 30733

	q003      = "Q003_WillTheSealBeBroken"
	q006      = "Q006_StepIntoTheFuture"
	q023      = "Q023_LidiasHeart"
	q950      = "Q950_GuardQuest"
	necklace  = 906
	dummyBase = 900
)

// anyBypassPage admits every npc_ and Quest command: the goldens offer a
// bypass with a page sent silently before the client sends it.
const anyBypassPage = `<html><body><edit var="c"><a action="bypass -h npc_$c">Go</a>` +
	`<a action="bypass -h Quest $c">Quest</a></body></html>`

// goldenPage returns the page text of the first NpcHtmlMessage of the
// golden row id of table.
func goldenPage(t *testing.T, table, id string) string {
	t.Helper()
	for _, r := range scriptcontract.Lookup(t, table).Rows {
		if r.ID != id {
			continue
		}
		for _, line := range r.Lines {
			_, quoted, ok := strings.Cut(line, " html=")
			if !ok {
				continue
			}
			page, err := strconv.Unquote(quoted)
			if err != nil {
				t.Fatalf("golden %s row %s page %s: %v", table, id, quoted, err)
			}
			return page
		}
	}
	t.Fatalf("golden %s has no page in row %s", table, id)
	return ""
}

// contractPages are the pages the dialog goldens show: Q001's from its
// page files, the other scripts' answered as whole pages.
type contractPages struct {
	darinStart, darinStarted        string
	roxxyStart, roxxyStarted        string
	tallothStarted, noblesseRefusal string
}

func loadContractPages(t *testing.T) contractPages {
	t.Helper()
	return contractPages{
		darinStart:     goldenPage(t, "dialog.general_window", "one-quest-start"),
		darinStarted:   goldenPage(t, "dialog.general_window", "too-many-but-has-state"),
		roxxyStart:     goldenPage(t, "dialog.general_window", "one-quest-start-not-talk-first"),
		roxxyStarted:   goldenPage(t, "dialog.general_window", "started-start-quest-listed-once"),
		tallothStarted: goldenPage(t, "dialog.single_window", "unbound-quest-with-state"),
		// The refusal links back to the NPC, which the engine fills in.
		noblesseRefusal: strings.ReplaceAll(goldenPage(t, "dialog.single_window", "script-name-overweight"), "{roxxy}", "%objectId%"),
	}
}

// dialogScripts registers the goldens' scripts in scripts.xml order: the
// real Q001, and stand-ins for Q003, Q006, the Noblesse teleporter and the
// Black Judge that answer what the reference scripts answer on the
// goldens' paths, plus 25 bindless quests the seeds start. Darin, Roxxy,
// Baulro, Talloth and the judge are civilian templates, the guard a
// hostile one.
func dialogScripts(t *testing.T, pages contractPages) gameservertest.Option {
	t.Helper()
	answer := func(name string, created, started string) func(*script.Script, script.Talk) string {
		return func(s *script.Script, e script.Talk) string {
			st := s.QuestState(e.Player, name)
			switch {
			case st == nil:
				return script.NoQuestMsg()
			case st.Status() == questlog.StatusCreated && created != "":
				return created
			case st.Status() == questlog.StatusStarted && started != "":
				return started
			}
			return script.NoQuestMsg()
		}
	}
	catalog := script.Catalog{
		"quest.Q001_LettersOfLove": quest001.New,
		"quest.Q003_WillTheSealBeBroken": func() script.Script {
			return script.Script{
				Title: "Will the Seal be Broken?", QuestID: 3,
				Bind:  script.Bindings{script.EventQuestStart: {tallothID}, script.EventTalked: {tallothID}},
				Hooks: script.Hooks{OnTalk: answer(q003, "", pages.tallothStarted)},
			}
		},
		"quest.Q006_StepIntoTheFuture": func() script.Script {
			return script.Script{
				Title: "Step into the Future", QuestID: 6,
				Bind:  script.Bindings{script.EventQuestStart: {roxxyID}, script.EventTalked: {roxxyID, baulroID}},
				Hooks: script.Hooks{OnTalk: answer(q006, pages.roxxyStart, pages.roxxyStarted)},
			}
		},
		"script.teleport.NoblesseTeleporter": func() script.Script {
			return script.Script{
				Dir:   "teleport",
				Bind:  script.Bindings{script.EventTalked: {roxxyID}},
				Hooks: script.Hooks{OnTalk: func(*script.Script, script.Talk) string { return pages.noblesseRefusal }},
			}
		},
		"script.feature.BlackJudge": func() script.Script {
			return script.Script{
				Dir:  "feature",
				Bind: script.Bindings{script.EventFirstTalk: {judgeID, judgeGuardID}},
				Hooks: script.Hooks{OnFirstTalk: func(*script.Script, script.FirstTalk) string {
					return "<html><body>Judge %objectId%</body></html>"
				}},
			}
		},
		"script.feature.Silent": func() script.Script {
			return script.Script{
				Dir:   "feature",
				Bind:  script.Bindings{script.EventFirstTalk: {silentID}},
				Hooks: script.Hooks{OnFirstTalk: func(*script.Script, script.FirstTalk) string { return "" }},
			}
		},
		// The guard's quest answers its window with a page naming the
		// guard, and each event with the event's name.
		"quest.Q950_GuardQuest": func() script.Script {
			return script.Script{
				Title: "Guard Quest", QuestID: 950,
				Bind: script.Bindings{script.EventQuestStart: {guardID}, script.EventTalked: {guardID}},
				Hooks: script.Hooks{
					OnTalk:  func(*script.Script, script.Talk) string { return "<html><body>guard %objectId%</body></html>" },
					OnEvent: func(_ *script.Script, e script.Event) string { return e.Name },
				},
			}
		},
		"quest.Q023_LidiasHeart": func() script.Script { return script.Script{Title: "Lidia's Heart", QuestID: 23} },
	}
	list := []script.Listing{
		{Path: "quest.Q001_LettersOfLove"},
		{Path: "quest.Q003_WillTheSealBeBroken"},
		{Path: "quest.Q006_StepIntoTheFuture"},
		{Path: "quest.Q023_LidiasHeart"},
		{Path: "script.teleport.NoblesseTeleporter"},
		{Path: "script.feature.BlackJudge"},
		{Path: "script.feature.Silent"},
		{Path: "quest.Q950_GuardQuest"},
	}
	for i := range 25 {
		path := fmt.Sprintf("quest.Q%d_Seed", dummyBase+i)
		id := int32(dummyBase + i)
		catalog[path] = func() script.Script { return script.Script{QuestID: id} }
		list = append(list, script.Listing{Path: path})
	}
	kinds := map[int32]script.NPCKind{
		darinID: script.KindFolk, roxxyID: script.KindFolk, baulroID: script.KindFolk,
		tallothID: script.KindFolk, judgeID: script.KindFolk, silentID: script.KindFolk, guardID: script.KindHostile,
		judgeGuardID: script.KindHostile, silentGuardID: script.KindHostile,
	}
	return gameservertest.WithNPCScripts(kinds, list, catalog)
}

// dialogItemTemplates is the shared catalog plus Q001's quest items and
// its reward necklace.
func dialogItemTemplates() gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range append(slices.Clone(q001Items), necklace) {
		templates = append(templates, &item.Template{
			ID: id, Name: "Quest Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// dialogWorld is a booted server with its character, level 20, in the
// world.
type dialogWorld struct {
	srv    *gameservertest.Server
	player int32
	at     location.Location
	// roles names the NPCs spawned for the goldens' renders.
	roles map[int32]string
	npcs  map[string]int32
}

// bootDialog boots the character with the dialog scripts and the pages,
// runs before (seeds that must precede the selection) and enters the
// world.
func bootDialog(t *testing.T, pages map[string]string, before func(srv *gameservertest.Server, objID int32), opts ...gameservertest.Option) *dialogWorld {
	t.Helper()
	all := map[string]string{"test/any.htm": anyBypassPage}
	for k, v := range pages {
		all[k] = v
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Talker", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(all),
		dialogScripts(t, loadContractPages(t)),
		dialogItemTemplates(),
	}, opts...)...)
	objID := srv.SoleObjectID(t)
	if before != nil {
		before(srv, objID)
	}
	enterWorld(t, srv)
	x, y, z := srv.PlayerPosition(t, objID)
	return &dialogWorld{srv: srv, player: objID, at: location.Location{X: x, Y: y, Z: z}, roles: map[int32]string{}, npcs: map[string]int32{}}
}

// spawnFolk places the civilian NPC npcID at offset from the player under
// role.
func (w *dialogWorld) spawnFolk(t *testing.T, role string, npcID int, dx, dy, dz int) *npc.Folk {
	t.Helper()
	f := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", npcID), location.Location{X: w.at.X + dx, Y: w.at.Y + dy, Z: w.at.Z + dz})
	w.roles[f.ObjectID()], w.npcs[role] = role, f.ObjectID()
	w.srv.ReadQueued(t, w.srv.Client)
	return f
}

// spawnGoldenNPCs places the goldens' NPCs beside the player, Darin dx
// east of it.
func (w *dialogWorld) spawnGoldenNPCs(t *testing.T, dx int) map[string]*npc.Folk {
	t.Helper()
	return map[string]*npc.Folk{
		"darin":   w.spawnFolk(t, "darin", darinID, dx, 0, 0),
		"roxxy":   w.spawnFolk(t, "roxxy", roxxyID, 0, 10, 0),
		"baulro":  w.spawnFolk(t, "baulro", baulroID, 0, 20, 0),
		"talloth": w.spawnFolk(t, "talloth", tallothID, 0, 30, 0),
	}
}

// expand replaces each {role} of s with that NPC's object id.
func (w *dialogWorld) expand(s string) string {
	for role, id := range w.npcs {
		s = strings.ReplaceAll(s, "{"+role+"}", strconv.Itoa(int(id)))
	}
	return s
}

// offerAll opens the page that admits every npc_ and Quest command, and
// discards it.
func (w *dialogWorld) offerAll(t *testing.T) {
	t.Helper()
	w.srv.Client.Send(encodeLinkHTML("test/any.htm"))
	w.srv.ReadQueued(t, w.srv.Client)
}

// bypass sends command and renders what it is answered with, deferred
// updates left out.
func (w *dialogWorld) bypass(t *testing.T, command string) []string {
	t.Helper()
	w.srv.Client.Send(encodeBypass(command))
	return w.lines(t)
}

// lines renders every frame queued since the last read as the goldens
// write packets, deferred updates left out.
func (w *dialogWorld) lines(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, f := range w.srv.ReadQueued(t, w.srv.Client) {
		if deferredUpdate(f) {
			continue
		}
		line, err := scriptcontract.Packet(f, w.roles)
		if err != nil {
			t.Fatalf("frame %x: %v", f, err)
		}
		out = append(out, line)
	}
	return out
}

// interact selects the NPC objectID and talks to it, returning the frames
// of the talk.
func (w *dialogWorld) interact(t *testing.T, objectID int32) [][]byte {
	t.Helper()
	w.srv.Client.Send(encodeTutorialAction(objectID, w.at))
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Client.Send(encodeTutorialAction(objectID, w.at))
	return w.srv.ReadQueued(t, w.srv.Client)
}

// onCharacter runs fn on the player's queue.
func (w *dialogWorld) onCharacter(t *testing.T, fn func(c *player.Character)) {
	t.Helper()
	w.srv.RunQuest(t, w.player, q001, func(_ *script.Quests, c *player.Character, _ *script.Script) { fn(c) })
}

// lastRole is the role of the player's last quest NPC, "0" when it has
// none.
func (w *dialogWorld) lastRole(t *testing.T) string {
	t.Helper()
	var last int32
	w.onCharacter(t, func(c *player.Character) { last = c.LastQuestNPC() })
	if last == 0 {
		return "0"
	}
	if role, ok := w.roles[last]; ok {
		return role
	}
	return strconv.Itoa(int(last))
}

// states lists the player's states in the goldens' quests as name:STATUS,
// sorted; "-" for none.
func (w *dialogWorld) states(t *testing.T) string {
	t.Helper()
	var out []string
	w.onCharacter(t, func(c *player.Character) {
		for _, name := range []string{q001, q003, q006} {
			if st := c.Quests().State(name); st != nil {
				out = append(out, name+":"+st.Status().String())
			}
		}
	})
	if len(out) == 0 {
		return "-"
	}
	slices.Sort(out)
	return strings.Join(out, ",")
}

// seedJournal seeds the golden's quest states, before the selection loads
// them: name:STATE entries (a started quest at condition 1), started24,
// started25 and completed25 (that many bindless quests), and overweight
// (a load past the third weight band; the suite boots with the shipped
// weight limit multiplier of 1).
func seedJournal(t *testing.T, srv *gameservertest.Server, objID int32, seeds []string) {
	t.Helper()
	state := func(name, status string) {
		insertJournal(t, srv.DB, journalRow{objID, name, "<state>", val(status)})
		if status == "STARTED" {
			insertJournal(t, srv.DB, journalRow{objID, name, "<cond>", val("1")})
		}
	}
	for _, seed := range seeds {
		switch seed {
		case "started24", "started25", "completed25":
			n, status := 25, "STARTED"
			if seed == "started24" {
				n = 24
			}
			if seed == "completed25" {
				status = "COMPLETED"
			}
			for i := range n {
				state(fmt.Sprintf("Q%d_Seed", dummyBase+i), status)
			}
		case "overweight":
			srv.GiveItem(t, objID, 9500, 100_000)
		default:
			name, status, ok := strings.Cut(seed, ":")
			if !ok {
				t.Fatalf("unknown seed %q", seed)
			}
			state(name, status)
		}
	}
}

func encodeBypass(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	w.WriteString(command)
	return w.Bytes()
}

func encodeLinkHTML(link string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestLinkHtml)
	w.WriteString(link)
	return w.Bytes()
}
