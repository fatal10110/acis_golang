package combat

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// The fan-out goldens' roles and the template ids they name.
var fanoutRoles = map[string]int32{"M": 20030, "m1": 20031, "m2": 20040}

// hookLog records, as the fanout goldens write them, the attacked and
// party-attacked hooks of a test behavior. Hooks run on the attacker's
// queue, so every field is guarded by mu.
type hookLog struct {
	mu sync.Mutex
	// names maps an object id to the role name the lines use.
	names map[int32]string
	// npcs are the NPCs by object id, for reading their HP from a hook.
	npcs  map[int32]*npc.Hostile
	lines []string
	// hp is, per attacked line, the attacked NPC's HP when the hook ran.
	hp []float64
}

func newHookLog() *hookLog {
	return &hookLog{names: map[int32]string{}, npcs: map[int32]*npc.Hostile{}}
}

func (l *hookLog) name(id int32, role string) {
	l.mu.Lock()
	l.names[id] = role
	l.mu.Unlock()
}

func (l *hookLog) track(role string, h *npc.Hostile) {
	l.mu.Lock()
	l.names[h.ObjectID()] = role
	l.npcs[h.ObjectID()] = h
	l.mu.Unlock()
}

// take returns the lines and HP readings since the last call.
func (l *hookLog) take() ([]string, []float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	lines, hp := l.lines, l.hp
	l.lines, l.hp = nil, nil
	return lines, hp
}

func (l *hookLog) nameOf(c script.Creature) string {
	if c == nil {
		return "none"
	}
	if n, ok := l.names[c.ObjectID()]; ok {
		return n
	}
	return strconv.Itoa(int(c.ObjectID()))
}

// behavior is a behavior bound to ids that records its attacked and
// party-attacked hooks.
func (l *hookLog) behavior(ids ...int32) func() script.Script {
	return func() script.Script {
		return script.Script{Behavior: true, NPCs: ids, Hooks: script.Hooks{
			OnAttacked: func(_ *script.Script, e script.Attacked) {
				l.mu.Lock()
				defer l.mu.Unlock()
				sk := "none"
				if e.Skill != (skill.Ref{}) {
					sk = strconv.Itoa(int(e.Skill.ID))
				}
				l.lines = append(l.lines, fmt.Sprintf("ATTACKED npc=%s attacker=%s damage=%d skill=%s", l.nameOf(e.NPC), l.nameOf(e.Attacker), e.Damage, sk))
				if h := l.npcs[e.NPC.ObjectID()]; h != nil {
					l.hp = append(l.hp, h.HP())
				}
			},
			OnPartyAttacked: func(_ *script.Script, e script.PartyAttacked) {
				l.mu.Lock()
				defer l.mu.Unlock()
				l.lines = append(l.lines, fmt.Sprintf("PARTY_ATTACKED caller=%s called=%s target=%s damage=%d", l.nameOf(e.Caller), l.nameOf(e.Called), l.nameOf(e.Target), e.Damage))
			},
		}}
	}
}

// bootAttackedHooks boots one character with the recording behavior bound
// to the behaved template ids, every role id having a hostile template.
func bootAttackedHooks(t *testing.T, log *hookLog, behaved []int32, opts ...gameservertest.Option) *gameservertest.Server {
	t.Helper()
	kinds := map[int32]script.NPCKind{}
	for _, id := range fanoutRoles {
		kinds[id] = script.KindHostile
	}
	for _, id := range behaved {
		kinds[id] = script.KindHostile
	}
	return gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "ai.Recorder"}}, script.Catalog{"ai.Recorder": log.behavior(behaved...)}),
	}, opts...)...)
}

// roleTemplate is a monster template with id; params are its AI
// parameters.
func roleTemplate(id int32, params npc.AIParams) *npc.Template {
	return &npc.Template{
		ID: int(id), TemplateID: int(id), Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		AIParams: params,
	}
}

// onAttackerQueue runs fn on the player objID's queue, where a hit on an
// NPC lands, and waits for it.
func onAttackerQueue(t *testing.T, srv *gameservertest.Server, objID int32, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !srv.PlayerQueue(t, objID).Post(func() {
		defer close(done)
		fn()
	}) {
		t.Fatalf("player %d's queue is closed", objID)
	}
	<-done
}

// replayFanout runs every row of the fan-out golden table that is not a
// clan row: the clan scans are A7's (#1800). attack applies the row's
// source to target from the player p.
func replayFanout(t *testing.T, table string, attack func(target *npc.Hostile, p attackable.Combatant)) {
	t.Helper()
	ids := []int32{fanoutRoles["M"], fanoutRoles["m1"], fanoutRoles["m2"]}
	log := newHookLog()
	srv := bootAttackedHooks(t, log, ids)
	startInWorld(t, srv.Client)
	p := liveCombatant(t, srv)
	log.name(p.ObjectID(), "p")

	scriptcontract.Run(t, table, func(t *testing.T, r scriptcontract.Row) {
		if strings.HasPrefix(r.ID, "clan") {
			t.Skip("the clan scan is A7's (#1800)")
		}
		roles := map[string]*npc.Hostile{}
		for i, role := range r.List(t, "npcs") {
			id, ok := fanoutRoles[role]
			if !ok {
				t.Fatalf("unknown role %q", role)
			}
			h := srv.SpawnHostileNPCTemplateAt(t, roleTemplate(id, nil), location.Location{X: hostileX + 40*i, Y: hostileY, Z: hostileZ})
			roles[role] = h
			log.track(role, h)
		}
		for _, s := range r.List(t, "setup") {
			if s == "party" {
				for role, h := range roles {
					if role != "M" {
						h.SetMaster(roles["M"])
						roles["M"].AddMinion(h)
					}
				}
				continue
			}
			role, state, ok := strings.Cut(s, ":")
			if !ok || state != "dead" || roles[role] == nil {
				t.Fatalf("unknown setup %q", s)
			}
			roles[role].MarkDead()
		}
		target := roles[r.Str(t, "target")]
		onAttackerQueue(t, srv, p.ObjectID(), func() { attack(target, p) })

		got, _ := log.take()
		if !slices.Equal(got, r.Lines) {
			t.Fatalf("hook calls =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(r.Lines, "\n  "))
		}
		for _, a := range r.List(t, "after") {
			minion, master, _ := strings.Cut(a, ".master=")
			if h := roles[minion]; h == nil || h.Master() != roles[master] {
				t.Fatalf("after: %s.master is not %s", minion, master)
			}
		}
	})
}

// TestAttackedFanOutFromAHit replays fanout.hit: a hit raises the attacked
// hook, then the party: the NPC itself, its live master, its live party
// minions other than itself. A dead NPC raises nothing.
func TestAttackedFanOutFromAHit(t *testing.T) {
	t.Parallel()
	replayFanout(t, "fanout.hit", func(target *npc.Hostile, p attackable.Combatant) {
		target.TakeDamage(10, p)
	})
}

// TestAttackedFanOutFromAnAggressionEffect replays fanout.aggression: the
// minion loop does not skip the caller, so a minion is called twice.
func TestAttackedFanOutFromAnAggressionEffect(t *testing.T) {
	t.Parallel()
	replayFanout(t, "fanout.aggression", func(target *npc.Hostile, p attackable.Combatant) {
		target.NotifyAggression(p, 50)
	})
}

// TestAttackedFanOutFromASkill replays fanout.skill with the golden's Slow
// (1160 level 1: offensive debuff, no aggro points): the value is
// max(120, aggro points), and a dead target is called too.
func TestAttackedFanOutFromASkill(t *testing.T) {
	t.Parallel()
	slow := skill.Definition{ID: 1160, Level: 1, Offensive: true, Debuff: true}
	replayFanout(t, "fanout.skill", func(target *npc.Hostile, p attackable.Combatant) {
		target.SkillAttacked(p, slow)
	})
}

// shotParams arms a monster with one soulshot it recharges on every hit
// the built-in reaction sees.
func shotParams() npc.AIParams {
	return npc.AIParams{"SoulShot": "1", "SoulShotRate": "100"}
}

// TestAttackedHitGatesStandIns pins the hit path per NPC id, on both
// executors. With a behavior bound, the attacked hook runs on the
// attacker's queue before the HP change, and the built-in reactions stay
// off: no attack desire from the hit's hate weight, no shot recharge, so
// the player sees only the NPC's StatusUpdate. Without one they still run:
// MagicSkillUse (the soulshot recharge) then StatusUpdate.
func TestAttackedHitGatesStandIns(t *testing.T) {
	t.Parallel()
	const bound, unbound = int32(20030), int32(20050)
	for _, exec := range []struct {
		name string
		opts []gameservertest.Option
	}{
		{"inline", nil},
		{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
	} {
		for _, tc := range []struct {
			name    string
			id      int32
			hooked  bool
			wantOps []byte
		}{
			{"bound", bound, true, []byte{serverpackets.OpcodeStatusUpdate}},
			{"unbound", unbound, false, []byte{serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeStatusUpdate}},
		} {
			t.Run(exec.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				log := newHookLog()
				srv := bootAttackedHooks(t, log, []int32{bound}, exec.opts...)
				c := srv.Client
				startInWorld(t, c)
				p := liveCombatant(t, srv)
				log.name(p.ObjectID(), "p")
				h := srv.SpawnHostileNPCTemplateAt(t, roleTemplate(tc.id, shotParams()), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
				log.track("M", h)
				srv.ReadQueued(t, c)
				targetHostile(t, c, h.ObjectID())
				before := h.HP()

				onAttackerQueue(t, srv, p.ObjectID(), func() { h.TakeDamage(10, p) })

				lines, hp := log.take()
				if tc.hooked {
					want := []string{"ATTACKED npc=M attacker=p damage=10 skill=none"}
					if !slices.Equal(lines, want) || !slices.Equal(hp, []float64{before}) {
						t.Fatalf("hook calls = %q seeing HP %v, want %q seeing the HP before the hit (%v)", lines, hp, want, before)
					}
				} else if len(lines) != 0 {
					t.Fatalf("hook calls on an unbound NPC = %q, want none", lines)
				}
				if got := h.HP(); got != before-10 {
					t.Fatalf("HP after the hit = %v, want %v", got, before-10)
				}
				if got := framesOf(srv.ReadQueued(t, c), h.ObjectID()); !slices.Equal(got, tc.wantOps) {
					t.Fatalf("NPC frames = % x, want % x", got, tc.wantOps)
				}
				d, queued := h.AI().Desires().Peek()
				if tc.hooked && queued {
					t.Fatalf("bound NPC desire after the hit = %+v, want none: the hate weight is the behavior's", d)
				}
				if !tc.hooked && (!queued || d.Kind != ai.IntentionAttack || d.FinalTarget.ObjectID() != p.ObjectID()) {
					t.Fatalf("unbound NPC desire after the hit = (%v, %+v), want an attack on the player", queued, d)
				}
			})
		}
	}
}

// TestPartyAssistGatedPerCalledNPC: the master's hit calls its party; a
// bound minion gets its party-attacked hook and no built-in assist, an
// unbound one the built-in assist.
func TestPartyAssistGatedPerCalledNPC(t *testing.T) {
	t.Parallel()
	const master, boundMinion, unboundMinion = int32(20050), int32(20031), int32(20051)
	log := newHookLog()
	srv := bootAttackedHooks(t, log, []int32{boundMinion})
	startInWorld(t, srv.Client)
	p := liveCombatant(t, srv)
	log.name(p.ObjectID(), "p")
	assist := npc.AIParams{"Party_Type": "1"}
	m := srv.SpawnHostileNPCTemplateAt(t, roleTemplate(master, npc.AIParams{"Party_Type": "2"}), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	b := srv.SpawnHostileNPCTemplateAt(t, roleTemplate(boundMinion, assist), location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})
	u := srv.SpawnHostileNPCTemplateAt(t, roleTemplate(unboundMinion, assist), location.Location{X: hostileX + 80, Y: hostileY, Z: hostileZ})
	for role, h := range map[string]*npc.Hostile{"M": m, "b": b, "u": u} {
		log.track(role, h)
	}
	for _, minion := range []*npc.Hostile{b, u} {
		minion.Instance.Template.CanMove = true
		minion.SetMaster(m)
		m.AddMinion(minion)
	}

	onAttackerQueue(t, srv, p.ObjectID(), func() { m.TakeDamage(30, p) })

	if lines, _ := log.take(); !slices.Equal(lines, []string{"PARTY_ATTACKED caller=M called=b target=p damage=30"}) {
		t.Fatalf("hook calls = %q, want only the bound minion's party-attacked hook", lines)
	}
	if d, ok := b.AI().Desires().Peek(); ok {
		t.Fatalf("bound minion desire = %+v, want none: the assist is the behavior's", d)
	}
	if d, ok := u.AI().Desires().Peek(); !ok || d.Kind != ai.IntentionAttack || d.FinalTarget.ObjectID() != p.ObjectID() || d.Weight != 30 {
		t.Fatalf("unbound minion desire = (%v, %+v), want the built-in assist on the player at 30", ok, d)
	}
}

// TestAggressionGatesHateWeight: an aggression effect on a bound NPC runs
// its attacked hook with the power as the damage and queues no built-in
// attack desire; an unbound NPC still queues one.
func TestAggressionGatesHateWeight(t *testing.T) {
	t.Parallel()
	const bound, unbound = int32(20030), int32(20050)
	log := newHookLog()
	srv := bootAttackedHooks(t, log, []int32{bound})
	startInWorld(t, srv.Client)
	p := liveCombatant(t, srv)
	log.name(p.ObjectID(), "p")
	b := srv.SpawnHostileNPCTemplateAt(t, roleTemplate(bound, nil), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	u := srv.SpawnHostileNPCTemplateAt(t, roleTemplate(unbound, nil), location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})
	log.track("b", b)
	log.track("u", u)

	onAttackerQueue(t, srv, p.ObjectID(), func() {
		b.NotifyAggression(p, 80)
		u.NotifyAggression(p, 80)
	})

	if lines, _ := log.take(); !slices.Equal(lines, []string{"ATTACKED npc=b attacker=p damage=80 skill=none"}) {
		t.Fatalf("hook calls = %q, want the bound NPC's attacked hook", lines)
	}
	if d, ok := b.AI().Desires().Peek(); ok {
		t.Fatalf("bound NPC desire = %+v, want none", d)
	}
	if d, ok := u.AI().Desires().Peek(); !ok || d.Kind != ai.IntentionAttack {
		t.Fatalf("unbound NPC desire = (%v, %+v), want the built-in attack", ok, d)
	}
}

// TestAttackedFromADamageOverTimeTick: a skill's damage-over-time tick on
// a bound NPC raises its attacked hook naming the skill, with the damage
// truncated to an int, and queues no built-in attack desire.
func TestAttackedFromADamageOverTimeTick(t *testing.T) {
	t.Parallel()
	const bound, unbound = int32(20030), int32(20050)
	log := newHookLog()
	srv := bootAttackedHooks(t, log, []int32{bound})
	startInWorld(t, srv.Client)
	p := liveCombatant(t, srv)
	log.name(p.ObjectID(), "p")
	b := srv.SpawnHostileNPCTemplateAt(t, roleTemplate(bound, nil), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	u := srv.SpawnHostileNPCTemplateAt(t, roleTemplate(unbound, nil), location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})
	log.track("b", b)
	log.track("u", u)

	effector, ok := p.(effect.Actor)
	if !ok {
		t.Fatalf("player %T is not an effect actor", p)
	}
	onAttackerQueue(t, srv, p.ObjectID(), func() {
		b.ReduceHPBySkillDOT(5.7, effector, skill.Ref{ID: 84, Level: 2})
		u.ReduceHPBySkillDOT(5.7, effector, skill.Ref{ID: 84, Level: 2})
	})

	if lines, _ := log.take(); !slices.Equal(lines, []string{"ATTACKED npc=b attacker=p damage=5 skill=84"}) {
		t.Fatalf("hook calls = %q, want the bound NPC's attacked hook", lines)
	}
	if d, ok := b.AI().Desires().Peek(); ok {
		t.Fatalf("bound NPC desire = %+v, want none", d)
	}
	if d, ok := u.AI().Desires().Peek(); !ok || d.Kind != ai.IntentionAttack {
		t.Fatalf("unbound NPC desire = (%v, %+v), want the built-in attack", ok, d)
	}
}
