package combat

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// clanRole is a role of the fan-out goldens: its template id, its clan
// (none for the party roles), whether it is a civilian NPC, and the caller
// ids it ignores.
type clanRole struct {
	id      int32
	clan    string
	folk    bool
	ignores []int
}

// The goldens' roles: the party M, m1, m2; the caller C and its
// neighbours; far stands 450 from C, out of its clan range.
var clanRoles = map[string]clanRole{
	"M":        {id: 20030},
	"m1":       {id: 20031},
	"m2":       {id: 20040},
	"C":        {id: 20041, clan: "probe_clan"},
	"same":     {id: 20042, clan: "probe_clan"},
	"dead":     {id: 20043, clan: "probe_clan"},
	"other":    {id: 20044, clan: "other_clan"},
	"ignoring": {id: 20045, clan: "probe_clan", ignores: []int{20041}},
	"far":      {id: 20046, clan: "probe_clan"},
	"folk":     {id: 30048, clan: "probe_clan", folk: true},
	// seen is a clan member of the line-of-sight scene.
	"seen": {id: 20047, clan: "probe_clan"},
}

const (
	// probeClanRange is C's clan range in the goldens.
	probeClanRange = 300
	// probeRadius is each clan role's collision radius: C's plus same's is
	// the golden's 24.
	probeRadius = 12
	// farOffset is how far from C the far role stands.
	farOffset = 450
)

// clanBehavior is a behavior bound to ids that records, besides the
// attacked and party-attacked hooks, the clan and death hooks.
func (l *hookLog) clanBehavior(ids ...int32) func() script.Script {
	return func() script.Script {
		s := l.behavior(ids...)()
		s.Hooks = s.Hooks.With(l.clanHooks())
		s.Hooks.OnPartyDied = func(_ *script.Script, e script.PartyDied) {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.lines = append(l.lines, fmt.Sprintf("PARTY_DIED caller=%s called=%s", l.nameOf(e.Caller), l.nameOf(e.Called)))
		}
		return s
	}
}

// folkBehavior is a behavior bound to civilian ids that records the
// attacked, clan-attacked and clan-died hooks: a civilian NPC has no party.
func (l *hookLog) folkBehavior(ids ...int32) func() script.Script {
	return func() script.Script {
		base := l.behavior(ids...)()
		hooks := l.clanHooks()
		hooks.OnAttacked = base.OnAttacked
		return script.Script{Behavior: true, NPCs: ids, Hooks: hooks}
	}
}

func (l *hookLog) clanHooks() script.Hooks {
	return script.Hooks{
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			l.mu.Lock()
			defer l.mu.Unlock()
			sk := "none"
			if e.Skill != (skill.Ref{}) {
				sk = strconv.Itoa(int(e.Skill.ID))
			}
			l.lines = append(l.lines, fmt.Sprintf("CLAN_ATTACKED caller=%s called=%s attacker=%s damage=%d skill=%s", l.nameOf(e.Caller), l.nameOf(e.Called), l.nameOf(e.Attacker), e.Damage, sk))
		},
		OnClanDied: func(_ *script.Script, e script.ClanDied) {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.lines = append(l.lines, fmt.Sprintf("CLAN_DIED caller=%s called=%s killer=%s", l.nameOf(e.Caller), l.nameOf(e.Called), l.nameOf(e.Killer)))
		},
	}
}

func (l *hookLog) trackFolk(role string, f *npc.Folk) {
	l.mu.Lock()
	l.names[f.ObjectID()] = role
	l.folk[f.ObjectID()] = f
	l.mu.Unlock()
}

// bootClanHooks boots one character with the recording behaviors bound to
// every role id, and to extra hostile and civilian ids.
func bootClanHooks(t *testing.T, log *hookLog, extraHostile, extraFolk []int32, opts ...gameservertest.Option) *gameservertest.Server {
	t.Helper()
	kinds := map[int32]script.NPCKind{}
	hostile, folk := slices.Clone(extraHostile), slices.Clone(extraFolk)
	for _, r := range clanRoles {
		if r.folk {
			folk = append(folk, r.id)
		} else {
			hostile = append(hostile, r.id)
		}
	}
	for _, id := range hostile {
		kinds[id] = script.KindHostile
	}
	for _, id := range folk {
		kinds[id] = script.KindFolk
	}
	return gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCScripts(kinds,
			[]script.Listing{{Path: "ai.Recorder"}, {Path: "ai.FolkRecorder"}},
			script.Catalog{"ai.Recorder": log.clanBehavior(hostile...), "ai.FolkRecorder": log.folkBehavior(folk...)}),
	}, opts...)...)
}

// clanTemplate is the template of a role: a monster, or a civilian NPC
// that dies, of the role's clan with the goldens' clan range.
func clanTemplate(r clanRole) *npc.Template {
	t := roleTemplate(r.id, nil)
	if r.folk {
		t = gameservertest.FolkTemplate("Folk", int(r.id))
		t.Undying = false
	}
	if r.clan != "" {
		t.Clans = []string{r.clan}
		t.ClanRange = probeClanRange
		t.IgnoredIDs = r.ignores
		t.CollisionRadius = probeRadius
	}
	return t
}

// clanScene is the NPCs of one golden row by role.
type clanScene struct {
	hostile map[string]*npc.Hostile
	folk    map[string]*npc.Folk
}

// spawnRoles spawns the roles of a golden row, C's neighbours side by side
// near it and far farOffset away, and applies its setup.
func spawnRoles(t *testing.T, srv *gameservertest.Server, log *hookLog, r scriptcontract.Row) clanScene {
	t.Helper()
	sc := clanScene{hostile: map[string]*npc.Hostile{}, folk: map[string]*npc.Folk{}}
	ranges := map[string]int{}
	for _, s := range r.List(t, "setup") {
		if role, ok := strings.CutSuffix(s, ":range0"); ok {
			ranges[role] = 0
		}
	}
	for i, role := range r.List(t, "npcs") {
		spec, ok := clanRoles[role]
		if !ok {
			t.Fatalf("unknown role %q", role)
		}
		tmpl := clanTemplate(spec)
		if v, ok := ranges[role]; ok {
			tmpl.ClanRange = v
		}
		at := location.Location{X: hostileX + 20*i, Y: hostileY, Z: hostileZ}
		if role == "far" {
			at.X = hostileX + farOffset
		}
		if spec.folk {
			f := srv.SpawnFolkNPCAt(t, tmpl, at)
			sc.folk[role] = f
			log.trackFolk(role, f)
			continue
		}
		h := srv.SpawnHostileNPCTemplateAt(t, tmpl, at)
		sc.hostile[role] = h
		log.track(role, h)
	}
	for _, s := range r.List(t, "setup") {
		switch {
		case s == "-" || strings.HasSuffix(s, ":range0"):
		case s == "party":
			for role, h := range sc.hostile {
				if role != "M" && sc.hostile["M"] != nil {
					h.SetMaster(sc.hostile["M"])
					sc.hostile["M"].AddMinion(h)
				}
			}
		case strings.HasSuffix(s, ":dead"):
			h := sc.hostile[strings.TrimSuffix(s, ":dead")]
			if h == nil {
				t.Fatalf("unknown setup %q", s)
			}
			h.MarkDead()
		default:
			t.Fatalf("unknown setup %q", s)
		}
	}
	return sc
}

// remove takes the scene's NPCs out of the world, so the next row's
// clan scans do not find them.
func (sc clanScene) remove(srv *gameservertest.Server) {
	for _, h := range sc.hostile {
		h.Decay(srv.State, nil)
	}
	for _, f := range sc.folk {
		f.Decay(srv.State, nil)
	}
}

// checkMasters checks the row's after list of master links.
func checkMasters(t *testing.T, sc clanScene, r scriptcontract.Row) {
	t.Helper()
	for _, a := range r.List(t, "after") {
		if a == "-" {
			continue
		}
		minion, master, _ := strings.Cut(a, ".master=")
		if h := sc.hostile[minion]; h == nil || h.Master() != sc.hostile[master] {
			t.Fatalf("after: %s.master is not %s", minion, master)
		}
	}
}

// replayRows runs the rows of table that keep, with srv booted once. act
// applies the row's source to its target from the player p; keepLine
// selects the hook lines the table pins.
func replayRows(t *testing.T, table string, keep func(scriptcontract.Row) bool, keepLine func(string) bool, act func(target *npc.Hostile, p attackable.Combatant)) {
	t.Helper()
	log := newHookLog()
	srv := bootClanHooks(t, log, nil, nil)
	startInWorld(t, srv.Client)
	p := liveCombatant(t, srv)
	log.name(p.ObjectID(), "p")

	scriptcontract.Run(t, table, func(t *testing.T, r scriptcontract.Row) {
		if !keep(r) {
			t.Skip("not a row of this replay")
		}
		sc := spawnRoles(t, srv, log, r)
		defer sc.remove(srv)
		target := sc.hostile[r.Str(t, "target")]
		onAttackerQueue(t, srv, p.ObjectID(), func() { act(target, p) })

		got, _ := log.take()
		got = slices.DeleteFunc(got, func(line string) bool { return !keepLine(line) })
		if !slices.Equal(got, r.Lines) {
			t.Fatalf("hook calls =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(r.Lines, "\n  "))
		}
		checkMasters(t, sc, r)
	})
}

func isClanRow(r scriptcontract.Row) bool { return strings.HasPrefix(r.ID, "clan") }

func everyRow(scriptcontract.Row) bool { return true }

func everyLine(string) bool { return true }

func deathLine(line string) bool { return strings.Contains(line, "_DIED ") }

// lethal is more damage than any role's HP.
const lethal = 1_000_000

// TestClanFanOutFromAHit replays the clan rows of fanout.hit: the NPC calls
// itself, then every live NPC of its clan in range, civilian ones included,
// that does not ignore it; with no clan range it calls nobody.
func TestClanFanOutFromAHit(t *testing.T) {
	t.Parallel()
	replayRows(t, "fanout.hit", isClanRow, everyLine, func(target *npc.Hostile, p attackable.Combatant) {
		target.TakeDamage(10, p)
	})
}

// TestClanFanOutFromAnAggressionEffect replays the clan row of
// fanout.aggression: the NPC calls itself only.
func TestClanFanOutFromAnAggressionEffect(t *testing.T) {
	t.Parallel()
	replayRows(t, "fanout.aggression", isClanRow, everyLine, func(target *npc.Hostile, p attackable.Combatant) {
		target.NotifyAggression(p, 50)
	})
}

// TestClanFanOutFromASkill replays the clan row of fanout.skill: no call to
// the NPC itself, and only hostile NPCs of its clan are called.
func TestClanFanOutFromASkill(t *testing.T) {
	t.Parallel()
	slow := skill.Definition{ID: 1160, Level: 1, Offensive: true, Debuff: true}
	replayRows(t, "fanout.skill", isClanRow, everyLine, func(target *npc.Hostile, p attackable.Combatant) {
		target.SkillAttacked(p, slow)
	})
}

// TestPartyDiedFanOut replays fanout.party_died through a lethal hit: the
// dying NPC tells itself, its master and every other minion of its party,
// dead ones included, and a dying master lets its minions go.
func TestPartyDiedFanOut(t *testing.T) {
	t.Parallel()
	replayRows(t, "fanout.party_died", everyRow, deathLine, func(target *npc.Hostile, p attackable.Combatant) {
		if !target.TakeDamage(lethal, p) {
			t.Error("the lethal hit did not kill the target")
		}
	})
}

// TestClanDiedFanOut replays fanout.clan_died through a lethal hit: the
// dying NPC tells the live NPCs of its clan in range, civilian ones
// included, never itself.
func TestClanDiedFanOut(t *testing.T) {
	t.Parallel()
	replayRows(t, "fanout.clan_died", everyRow, deathLine, func(target *npc.Hostile, p attackable.Combatant) {
		if !target.TakeDamage(lethal, p) {
			t.Error("the lethal hit did not kill the target")
		}
	})
}

// TestClanRange replays fanout.clan_range: the clan range is 3D and
// inclusive, widened by both NPCs' collision radii.
func TestClanRange(t *testing.T) {
	t.Parallel()
	log := newHookLog()
	srv := bootClanHooks(t, log, nil, nil)
	startInWorld(t, srv.Client)
	p := liveCombatant(t, srv)
	log.name(p.ObjectID(), "p")

	scriptcontract.Run(t, "fanout.clan_range", func(t *testing.T, r scriptcontract.Row) {
		if got := r.Float(t, "radii"); got != 2*probeRadius {
			t.Fatalf("golden radii = %v, want the fixture's %v", got, 2*probeRadius)
		}
		c := srv.SpawnHostileNPCTemplateAt(t, clanTemplate(clanRoles["C"]), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
		same := srv.SpawnHostileNPCTemplateAt(t, clanTemplate(clanRoles["same"]), location.Location{X: hostileX + int(r.Int(t, "dx")), Y: hostileY, Z: hostileZ})
		log.track("C", c)
		log.track("same", same)
		onAttackerQueue(t, srv, p.ObjectID(), func() { c.TakeDamage(10, p) })

		got, _ := log.take()
		if !slices.Equal(got, r.Lines) {
			t.Fatalf("hook calls =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(r.Lines, "\n  "))
		}
		// Each row's NPCs leave, so the next row's C sees only its own.
		c.Decay(srv.State, nil)
		same.Decay(srv.State, nil)
		log.take()
	})
}

// sightWall is passable movement geo whose sight is cut by a wall at
// X = wallX: no line of sight crosses it.
type sightWall struct {
	gameservertest.Geo
	wallX int
}

func (g sightWall) CanSeeActor(ox, _, _ int, _ float64, tx, _, _ int, _ float64) bool {
	return (ox < g.wallX) == (tx < g.wallX)
}

// TestClanCallsNeedLineOfSight pins fanout.line_of_sight: each clan loop
// skips a neighbour the caller cannot see and goes on with the next one,
// while the call to the caller itself and the party calls take no sight.
func TestClanCallsNeedLineOfSight(t *testing.T) {
	t.Parallel()
	los := scriptcontract.Lookup(t, "fanout.line_of_sight")
	if len(los.Rows) == 0 {
		t.Fatal("fanout.line_of_sight has no rows")
	}
	log := newHookLog()
	srv := bootClanHooks(t, log, nil, nil)
	startInWorld(t, srv.Client)
	p := liveCombatant(t, srv)
	log.name(p.ObjectID(), "p")

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	c := srv.SpawnMovingHostileNPCTemplateAtGeo(t, clanTemplate(clanRoles["C"]), home, home, sightWall{wallX: hostileX + 30})
	hidden := srv.SpawnHostileNPCTemplateAt(t, clanTemplate(clanRoles["same"]), location.Location{X: hostileX + 60, Y: hostileY, Z: hostileZ})
	seen := srv.SpawnHostileNPCTemplateAt(t, clanTemplate(clanRoles["seen"]), location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	log.track("C", c)
	log.track("hidden", hidden)
	log.track("seen", seen)

	slow := skill.Definition{ID: 1160, Level: 1, Offensive: true, Debuff: true}
	onAttackerQueue(t, srv, p.ObjectID(), func() {
		c.TakeDamage(10, p)
		c.SkillAttacked(p, slow)
		c.TakeDamage(lethal, p)
	})
	got, _ := log.take()
	want := []string{
		"ATTACKED npc=C attacker=p damage=10 skill=none",
		"CLAN_ATTACKED caller=C called=C attacker=p damage=10 skill=none",
		"CLAN_ATTACKED caller=C called=seen attacker=p damage=10 skill=none",
		"ATTACKED npc=C attacker=p damage=120 skill=1160",
		"CLAN_ATTACKED caller=C called=seen attacker=p damage=120 skill=1160",
		fmt.Sprintf("ATTACKED npc=C attacker=p damage=%d skill=none", lethal),
		fmt.Sprintf("CLAN_ATTACKED caller=C called=C attacker=p damage=%d skill=none", lethal),
		fmt.Sprintf("CLAN_ATTACKED caller=C called=seen attacker=p damage=%d skill=none", lethal),
		"CLAN_DIED caller=C called=seen killer=p",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("hook calls =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// TestFolkAttackedRaisesItsHooksAndClanCalls: a hit on a civilian NPC runs
// its attacked hooks before the HP change, then its clan calls: itself,
// then its clan members in range of either kind. A skill landing on it
// makes no clan call, and its death tells them all.
func TestFolkAttackedRaisesItsHooksAndClanCalls(t *testing.T) {
	t.Parallel()
	const caller, mate = int32(30049), int32(30050)
	log := newHookLog()
	srv := bootClanHooks(t, log, nil, []int32{caller, mate})
	startInWorld(t, srv.Client)
	p := liveCombatant(t, srv)
	log.name(p.ObjectID(), "p")

	f := srv.SpawnFolkNPCAt(t, clanTemplate(clanRole{id: caller, clan: "probe_clan", folk: true}), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	m := srv.SpawnFolkNPCAt(t, clanTemplate(clanRole{id: mate, clan: "probe_clan", folk: true}), location.Location{X: hostileX + 20, Y: hostileY, Z: hostileZ})
	same := srv.SpawnHostileNPCTemplateAt(t, clanTemplate(clanRoles["same"]), location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})
	log.trackFolk("F", f)
	log.trackFolk("mate", m)
	log.track("same", same)

	before := f.HP()
	onAttackerQueue(t, srv, p.ObjectID(), func() { f.TakeDamage(10, p) })
	got, hp := log.take()
	if !slices.Equal(hp, []float64{before}) || f.HP() != before-10 {
		t.Fatalf("attacked hook saw HP %v and left %v, want it to see %v, before the hit took 10", hp, f.HP(), before)
	}
	slow := skill.Definition{ID: 1160, Level: 1, Offensive: true, Debuff: true}
	onAttackerQueue(t, srv, p.ObjectID(), func() {
		f.SkillAttacked(p, slow)
		f.TakeDamage(lethal, p)
	})
	more, _ := log.take()
	got = append(got, more...)

	want := []string{
		"ATTACKED npc=F attacker=p damage=10 skill=none",
		"CLAN_ATTACKED caller=F called=F attacker=p damage=10 skill=none",
		"CLAN_ATTACKED caller=F called=mate attacker=p damage=10 skill=none",
		"CLAN_ATTACKED caller=F called=same attacker=p damage=10 skill=none",
		"ATTACKED npc=F attacker=p damage=120 skill=1160",
		fmt.Sprintf("ATTACKED npc=F attacker=p damage=%d skill=none", lethal),
		fmt.Sprintf("CLAN_ATTACKED caller=F called=F attacker=p damage=%d skill=none", lethal),
		fmt.Sprintf("CLAN_ATTACKED caller=F called=mate attacker=p damage=%d skill=none", lethal),
		fmt.Sprintf("CLAN_ATTACKED caller=F called=same attacker=p damage=%d skill=none", lethal),
		"CLAN_DIED caller=F called=mate killer=p",
		"CLAN_DIED caller=F called=same killer=p",
	}
	if !sameLines(got, want) {
		t.Fatalf("hook calls =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	if !f.Dead() {
		t.Fatal("the civilian NPC survived the lethal hit")
	}
}

// sameLines reports whether got is want with each run of clan calls to
// neighbours in any order: the known list is unordered.
func sameLines(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	norm := func(lines []string) []string {
		out := slices.Clone(lines)
		for i := 0; i < len(out); {
			j := i
			for j < len(out) && isNeighbourCall(out[j]) {
				j++
			}
			if j > i {
				slices.Sort(out[i:j])
				i = j
				continue
			}
			i++
		}
		return out
	}
	return slices.Equal(norm(got), norm(want))
}

func isNeighbourCall(line string) bool {
	return (strings.HasPrefix(line, "CLAN_ATTACKED ") || strings.HasPrefix(line, "CLAN_DIED ")) && !strings.Contains(line, "called=F ")
}
