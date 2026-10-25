package quest

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	opsQuest = "Q953_OwnQueueOps"
	// opsSkillID is a fixture self-target TELEPORT skill: its cast moves
	// the caster to opsSkillDest.
	opsSkillID = 9302
)

// opsSkillDest is where opsSkillID moves its caster.
var opsSkillDest = location.Location{X: 1000, Y: 2000, Z: -3000}

// nearSkillDest reports whether x, y lies within 20 of opsSkillDest, where
// the skill's teleport lands.
func nearSkillDest(x, y int) bool {
	return abs(x-opsSkillDest.X) <= 20 && abs(y-opsSkillDest.Y) <= 20
}

// opsNames names the frames an instant teleport, a skill call and a dialog
// answer send; any other frame is left out.
var opsNames = map[byte]string{
	serverpackets.OpcodeActionFailed:       "ActionFailed",
	serverpackets.OpcodeTargetUnselected:   "TargetUnselected",
	serverpackets.OpcodeTeleportToLocation: "TeleportToLocation",
	serverpackets.OpcodeMagicSkillUse:      "MagicSkillUse",
	serverpackets.OpcodeNpcHtmlMessage:     "NpcHtmlMessage",
}

func opsFrames(frames [][]byte) []string {
	var out []string
	for _, f := range frames {
		if name, ok := opsNames[f[0]]; ok {
			out = append(out, name)
		}
	}
	return out
}

// opsWorld is a player beside an NPC whose quest jumps it (instant
// teleport) or makes it cast a self-teleport skill, from a quest event on
// the player's own queue and from an engine timer reading the player out of
// a cell.
type opsWorld struct {
	*dialogWorld
	// dest is where every jump lands.
	dest location.Location
	// moved records, per hook, whether the player stood where the operation
	// sends it right after the call returned.
	moved map[string]*atomic.Bool
	fired *atomic.Int32
}

func bootOps(t *testing.T) *opsWorld {
	t.Helper()
	w := &opsWorld{moved: map[string]*atomic.Bool{"jump": {}, "cast": {}, "stored jump": {}, "stored cast": {}}, fired: &atomic.Int32{}}
	held := &script.PlayerCell{}
	ref := modelskill.Ref{ID: opsSkillID, Level: 1}
	at := func(p *script.Player, want location.Location) bool {
		l := script.LocOf(p)
		return location.Location{X: l.X, Y: l.Y, Z: l.Z} == want
	}
	// The skill's teleport lands within 20 of its coordinates.
	nearSkillDest := func(p *script.Player) bool {
		l := script.LocOf(p)
		return nearSkillDest(l.X, l.Y)
	}
	run := func(s *script.Script, hook string, p *script.Player) {
		switch hook {
		case "jump", "stored jump":
			p.InstantTeleportTo(script.Loc{X: w.dest.X, Y: w.dest.Y, Z: w.dest.Z})
			w.moved[hook].Store(at(p, w.dest))
		case "cast", "stored cast":
			s.CallSkill(p, p, ref)
			w.moved[hook].Store(nearSkillDest(p))
		}
	}
	catalog := script.Catalog{
		"quest." + opsQuest: func() script.Script {
			return script.Script{
				Title: "Own Queue Ops", QuestID: 953,
				Bind: script.Bindings{script.EventQuestStart: {teleporterID}, script.EventTalked: {teleporterID}},
				Hooks: script.Hooks{
					OnEvent: func(s *script.Script, e script.Event) string {
						switch e.Name {
						case "jump", "cast":
							run(s, e.Name, e.Player)
							return "<html><body>" + e.Name + "</body></html>"
						case "stored jump", "stored cast":
							held.Set(e.Player)
							s.StartTimer(e.Name, nil, nil, 100*time.Millisecond)
						}
						return ""
					},
					OnTimer: func(s *script.Script, e script.Timer) string {
						defer w.fired.Add(1)
						run(s, e.Name, held.Get())
						return ""
					},
				},
			}
		},
	}
	def := modelskill.Definition{
		ID: opsSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "TELEPORT", HitTime: 500, TeleCoords: &opsSkillDest,
	}
	db := sqltest.SharedDB(t)
	table := modelskill.NewTable([]modelskill.Definition{def})
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Caster", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithHTMLPages(map[string]string{"test/any.htm": anyBypassPage}),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{teleporterID: script.KindFolk}, []script.Listing{{Path: "quest." + opsQuest}}, catalog),
	)
	objID := srv.SoleObjectID(t)
	enterWorld(t, srv)
	x, y, z := srv.PlayerPosition(t, objID)
	w.dialogWorld = &dialogWorld{srv: srv, player: objID, at: location.Location{X: x, Y: y, Z: z}, roles: map[int32]string{}, npcs: map[string]int32{}}
	w.dest = location.Location{X: x + 50, Y: y, Z: z}
	w.spawnFolk(t, "teleporter", teleporterID, 20, 0, 0)
	w.interact(t, w.npcs["teleporter"])
	w.offerAll(t)
	return w
}

// TestQuestEventInstantTeleportLandsBeforeThePage: a quest event runs on
// its player's own queue, so its instant teleport of that player runs
// inside the call, as the reference's Player.instantTeleportTo does: the
// hook reads the new position right after the call, and the jump goes out
// before the page the hook answers.
func TestQuestEventInstantTeleportLandsBeforeThePage(t *testing.T) {
	t.Parallel()
	w := bootOps(t)
	w.srv.Client.Send(encodeBypass("Quest " + opsQuest + " jump"))
	got := opsFrames(w.srv.ReadQueued(t, w.srv.Client))
	want := []string{"TeleportToLocation", "NpcHtmlMessage", "ActionFailed"}
	if !slices.Equal(got, want) {
		t.Fatalf("event jump frames:\n got %q\nwant %q", got, want)
	}
	if !w.moved["jump"].Load() {
		t.Fatal("the event hook read the old position after InstantTeleportTo")
	}
}

// TestQuestEventCallSkillCastsBeforeThePage: a quest event's skill call
// with its own player as caster casts inside the call, as the reference's
// Creature.callSkill does: the cast and the skill's teleport go out before
// the page, and the hook reads the position the skill moved the player to.
func TestQuestEventCallSkillCastsBeforeThePage(t *testing.T) {
	t.Parallel()
	w := bootOps(t)
	w.srv.Client.Send(encodeBypass("Quest " + opsQuest + " cast"))
	got := opsFrames(w.srv.ReadQueued(t, w.srv.Client))
	want := append(append([]string{"MagicSkillUse"}, abortAndTeleport[:len(abortAndTeleport)-1]...), "NpcHtmlMessage", "ActionFailed")
	if !slices.Equal(got, want) {
		t.Fatalf("event cast frames:\n got %q\nwant %q", got, want)
	}
	if !w.moved["cast"].Load() {
		t.Fatal("the event hook read the old position after CallSkill")
	}
}

// TestStoredHandleOpsPostFromAnotherQueue: a handle the event hook stored
// in a cell, used by a hook on another queue (an engine timer), still asks
// the jump and the cast of the player's own queue: that hook reads the old
// position, and the player moves once its queue runs the operation.
func TestStoredHandleOpsPostFromAnotherQueue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		hook   string
		landed func(w *opsWorld, x, y, z int) bool
		want   []string
	}{
		{"stored jump", func(w *opsWorld, x, y, z int) bool { return location.Location{X: x, Y: y, Z: z} == w.dest }, []string{"TeleportToLocation"}},
		{"stored cast", func(_ *opsWorld, x, y, _ int) bool { return nearSkillDest(x, y) }, append([]string{"MagicSkillUse"}, abortAndTeleport[:len(abortAndTeleport)-1]...)},
	} {
		t.Run(tc.hook, func(t *testing.T) {
			t.Parallel()
			w := bootOps(t)
			w.srv.Client.Send(encodeBypass("Quest " + opsQuest + " " + tc.hook))
			w.srv.ReadQueued(t, w.srv.Client)
			w.srv.AdvanceUntil(t, "the engine timer fires", func() bool { return w.fired.Load() > 0 })
			if w.moved[tc.hook].Load() {
				t.Fatal("an operation asked from another queue ran inside the call")
			}
			w.srv.AdvanceUntil(t, "the posted operation lands", func() bool {
				x, y, z := w.srv.PlayerPosition(t, w.player)
				return tc.landed(w, x, y, z)
			})
			if got := opsFrames(w.srv.ReadQueued(t, w.srv.Client)); !slices.Equal(got, tc.want) {
				t.Fatalf("posted frames:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

// tutorialJumps registers a tutorial quest whose event hook jumps its
// player 50 along x for every event in jumpOn, records whether it read the
// new position right after the call, and then shows question mark 7.
func tutorialJumps(jumpOn ...string) (gameservertest.Option, func(name string) (moved, heard bool)) {
	var mu sync.Mutex
	moved := map[string]bool{}
	hook := func(_ *script.Script, e script.Event) string {
		if !slices.Contains(jumpOn, e.Name) {
			return ""
		}
		p := e.Player
		from := script.LocOf(p)
		p.InstantTeleportTo(script.Loc{X: from.X + 50, Y: from.Y, Z: from.Z})
		to := script.LocOf(p)
		mu.Lock()
		moved[e.Name] = to.X == from.X+50 && to.Y == from.Y
		mu.Unlock()
		p.ShowQuestionMark(7)
		return ""
	}
	path := "script.feature." + tutorial
	opt := gameservertest.WithScripts([]script.Listing{{Path: path}}, script.Catalog{
		path: func() script.Script {
			return script.Script{QuestID: -1, Dir: "feature", Hooks: script.Hooks{OnEvent: hook}}
		},
	})
	return opt, func(name string) (bool, bool) {
		mu.Lock()
		defer mu.Unlock()
		m, ok := moved[name]
		return m, ok
	}
}

// tutorialJumpFrames names the jump and the tutorial's question mark among
// frames; any other frame is left out.
func tutorialJumpFrames(frames [][]byte) []string {
	var out []string
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeTeleportToLocation:
			out = append(out, "TeleportToLocation")
		case serverpackets.OpcodeTutorialShowQuestionMark:
			out = append(out, "TutorialShowQuestionMark")
		}
	}
	return out
}

// TestTutorialEventsOfThePlayersOwnActionsRunOnItsQueue: the tutorial
// events a player's own entry, tutorial request, pickup and sit request
// raise run on its own queue, so the tutorial hook's jump of that player
// runs inside the call, as in the reference: the hook reads the new
// position, and the jump goes out before the question mark the hook shows
// next.
func TestTutorialEventsOfThePlayersOwnActionsRunOnItsQueue(t *testing.T) {
	t.Parallel()
	opt, moved := tutorialJumps("UC", "go", "CE57", "CE8388608")
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		opt,
	)
	objID := srv.SoleObjectID(t)
	insertJournal(t, srv.DB, journalRow{objID, tutorial, questlog.KeyState, val("STARTED")})
	c := srv.Client
	inOrder := []string{"TeleportToLocation", "TutorialShowQuestionMark"}
	expect := func(what, name string, frames [][]byte) {
		t.Helper()
		if got := tutorialJumpFrames(frames); !slices.Equal(got, inOrder) {
			t.Fatalf("%s frames:\n got %q\nwant %q", what, got, inOrder)
		}
		if m, heard := moved(name); !heard || !m {
			t.Fatalf("%s: tutorial heard %s %v, read the new position %v; want both", what, name, heard, m)
		}
	}

	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	expect("entry", "UC", srv.ReadQueued(t, c))

	c.Send(encodeTutorialString(clientpackets.OpcodeRequestTutorialPassCmdToServer, "go"))
	expect("tutorial request", "go", srv.ReadQueued(t, c))

	var x, y, z int
	onCharacter(t, srv, objID, func(ch *player.Character) { x, y, z = ch.Position() })
	srv.SeedGroundItem(t, 0, item.AdenaID, 10, x, y, z)
	srv.ReadQueued(t, c)
	snaps := srv.GroundItems.Snapshots(nil)
	if len(snaps) != 1 {
		t.Fatalf("ground items = %d, want 1", len(snaps))
	}
	c.Send(encodeTutorialAction(snaps[0].ObjectID, location.Location{X: x, Y: y, Z: z}))
	expect("adena pickup", "CE57", srv.ReadQueued(t, c))

	// The pickup holds the player for 200 ms.
	srv.Advance(t, 250*time.Millisecond)
	c.Send(encodeTutorialSit())
	expect("sit request", "CE8388608", srv.ReadQueued(t, c))
}
