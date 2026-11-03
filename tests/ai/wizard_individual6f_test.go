package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The NPC ids of the sixth PR of the sixth individual behavior batch: the
// datapack ids each class binds.
const (
	i6fDDID          = int32(20092) // WizardDDMagic2
	i6fDDClanID      = int32(20331) // WizardDDMagic2
	i6fClanBuffID    = int32(18138) // RoyalRushWizardClanBuff
	i6fRangeDebuffID = int32(18139) // RoyalRushWizardRangeDebuff
	i6fPriestID      = int32(22143) // WizardHereticPriestAggressive
)

// The skills the PR's classes list by type, beside the batch's long and
// short range DD magics and range debuff.
const (
	i6fClanBuffSkill = modelskill.ID(9420)
	i6fDebuffSkill   = modelskill.ID(9421)
	i6fHoldSkill     = modelskill.ID(9422)
)

// individual6fScripts are the behavior chain down to Wizard, the classes
// of the PR in scripts.xml order, and the event listener.
func individual6fScripts() ([]script.Listing, script.Catalog) {
	entries := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.DefaultNpc", defaultnpc.New},
		{"script.ai.individual.Monster.MonsterAI", monster.New},
		{"script.ai.individual.Monster.WizardBase.WizardBase", wizard.Base},
		{"script.ai.individual.Monster.WizardBase.Wizard.Wizard", wizard.Wizard},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.RoyalRushWizardClanBuff", wizard.RoyalRushClanBuff},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.RoyalRushWizardRangeDebuff", wizard.RoyalRushRangeDebuff},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2", wizard.DDMagic2},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardHereticPriestAggressive", wizard.HereticPriestAggressive},
		{"ai.EventListener", eventListener},
	}
	var list []script.Listing
	catalog := script.Catalog{}
	for _, e := range entries {
		list = append(list, script.Listing{Path: e.path})
		catalog[e.path] = e.ctor
	}
	return list, catalog
}

func bootIndividual6f(t *testing.T, extra raidBoot, templates ...*npc.Template) *individual6World {
	t.Helper()
	list, catalog := individual6fScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draw := new(atomic.Int32)
	extra.opts = append(extra.opts,
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithScriptRand(func(n int) int { return min(int(draw.Load()), n-1) }),
	)
	return &individual6World{warriorWorld: bootRaid(t, extra, templates...), draw: draw}
}

// i6fTemplate is a fixture wizard of id with ai as its AI parameters,
// listing every skill type the PR's classes cast.
func i6fTemplate(id int32, ai npc.AIParams) *npc.Template {
	return i6bTemplate(id, ai, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeWLongRangeDDMagic: wLongDDID, npc.SkillTypeWShortRangeDDMagic: wShortDDID,
		npc.SkillTypeWRangeDebuff: i6eRangeDebuffID, npc.SkillTypeWClanBuff: i6fClanBuffSkill,
		npc.SkillTypeDebuff: i6fDebuffSkill, npc.SkillTypeHoldMagic: i6fHoldSkill,
	})
}

// i6fPriest is a fixture heretic priest with ai as its AI parameters, in
// the clan the scenario's priests share, standing still unless walk is
// set.
func i6fPriest(ai npc.AIParams, walk bool) *npc.Template {
	tmpl := i6fTemplate(i6fPriestID, ai)
	tmpl.Clans, tmpl.ClanRange = []string{"heretic_priest"}, 600
	tmpl.CanMove = walk
	return tmpl
}

// i6fArmed spawns an NPC of id dx, dy from the character, its skills
// harmless one-target magics costing nothing, or 100000 MP for the costly
// ones.
func (w *individual6World) i6fArmed(t *testing.T, id int32, dx, dy int, costly ...modelskill.ID) *npc.Hostile {
	t.Helper()
	h := w.spawn(t, id, dx, dy)
	var defs []modelskill.Definition
	for _, skill := range []modelskill.ID{wLongDDID, wShortDDID, i6eRangeDebuffID, i6fClanBuffSkill, i6fDebuffSkill, i6fHoldSkill} {
		d := harmlessSkill(skill)
		if slices.Contains(costly, skill) {
			d.MPConsume = 100000
		}
		defs = append(defs, d)
	}
	w.srv.InstallNPCCastSeam(t, h, modelskill.NewTable(defs))
	return h
}

// i6fCast is an expected cast desire: its target's object id, its weight,
// and whether it is held in place.
type i6fCast struct {
	target int32
	weight float64
	held   bool
}

// i6fWantCasts fails unless h's queued cast desires are exactly want.
func i6fWantCasts(t *testing.T, when string, h *npc.Hostile, want map[modelskill.ID]i6fCast) {
	t.Helper()
	got := ind1Casts(h)
	for id, c := range want {
		d, ok := got[id]
		if !ok || d.Weight != c.weight || d.FinalTarget == nil || d.FinalTarget.ObjectID() != c.target || d.MoveToTarget == c.held {
			t.Errorf("%s: npc %d cast desire of skill %d = %+v (%v), want weight %v on %d held %v", when, h.NpcID(), id, d, ok, c.weight, c.target, c.held)
		}
	}
	for id, d := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("%s: npc %d unexpected cast desire %+v", when, h.NpcID(), d)
		}
	}
}

// i6fMoveNear puts the character dx from h.
func (w *individual6World) i6fMoveNear(t *testing.T, h *npc.Hostile, dx int) {
	t.Helper()
	(&ind1World{warriorWorld: w.warriorWorld}).moveNear(t, w.player, h, dx)
}

// i6fLookAt moves the character 100 in front of h and runs one AI cycle,
// whose see-creature scan notices it, and returns the frames the
// character was sent until the server settled.
func (w *individual6World) i6fLookAt(t *testing.T, h *npc.Hostile) [][]byte {
	t.Helper()
	w.i6fMoveNear(t, h, -100)
	frames := w.ticks(t, 1)
	w.srv.Settle(t)
	return append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
}

// TestWizardDDMagic2AnswersHits runs WizardDDMagic2's attacked and
// skill-finished hooks. A playable's hit, with the attacker on its hate
// list, wants its long range DD magic on an attacker 300 away and its
// short range one on one 50 away; on a draw of 90 the short range one even
// 300 away. One that cannot pay gives up its spells and wants to attack,
// weight 1000, and its next hit adds the damage's 125 to that desire with
// no cast. The end of its cast brings the next one.
func TestWizardDDMagic2AnswersHits(t *testing.T) {
	t.Parallel()
	w := bootIndividual6f(t, raidBoot{}, i6fTemplate(i6fDDID, nil))
	me := w.player.ObjectID()

	far := w.i6fArmed(t, i6fDDID, 300, 0)
	near := w.i6fArmed(t, i6fDDID, 50, 0)
	w.hit(t, far, 10)
	w.hit(t, near, 10)
	i6fWantCasts(t, "300 away after a hit", far, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})
	i6fWantCasts(t, "50 away after a hit", near, map[modelskill.ID]i6fCast{wShortDDID: {me, 1000000, false}})

	w.draw.Store(90)
	wide := w.i6fArmed(t, i6fDDID, 0, 300)
	w.hit(t, wide, 10)
	i6fWantCasts(t, "300 away after a hit on a draw of 90", wide, map[modelskill.ID]i6fCast{wShortDDID: {me, 1000000, false}})

	w.draw.Store(0)
	broke := w.i6fArmed(t, i6fDDID, -300, 0, wLongDDID)
	w.hit(t, broke, 10)
	w.i6eGaveUp(t, broke, 1000)
	w.hit(t, broke, 10)
	if got := i6cAttackWeight(broke, me); got != 1125 {
		t.Fatalf("given up wizard attack desire after another hit = %v, want 1125", got)
	}
	i6fWantCasts(t, "given up", broke, nil)

	w.i6eCastsWithin(t, far, wLongDDID, 2)
}

// TestWizardDDMagic2FamilyAnswersItsClan runs the clan-attacked hooks of
// WizardDDMagic2 and its royal rush children, every draw 0, for a hit on
// their clan member. A settled WizardDDMagic2 with an empty hate list
// wants its long range DD magic on an attacker 300 away and its short
// range one on one closer than 100; one that cannot pay gives up its
// spells and wants to attack, weight 1000. A settled
// RoyalRushWizardClanBuff casts its clan buff at the attacker and, through
// its parent, its long range DD magic. A fresh RoyalRushWizardRangeDebuff
// wants its range debuff on the attacker, and its parent, not settled,
// nothing more.
func TestWizardDDMagic2FamilyAnswersItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual6f(t, raidBoot{},
		inClan(i6fTemplate(i6fDDClanID, nil)), inClan(i6fTemplate(i6fClanBuffID, nil)),
		inClan(i6fTemplate(i6fRangeDebuffID, nil)), inClan(warriorTemplate(i6eMateID, nil)))
	me := w.player.ObjectID()
	ddFar := w.i6fArmed(t, i6fDDClanID, 300, 0)
	ddNear := w.i6fArmed(t, i6fDDClanID, 50, 50)
	ddBroke := w.i6fArmed(t, i6fDDClanID, -300, 0, wLongDDID)
	buff := w.i6fArmed(t, i6fClanBuffID, 0, 300)
	mate := w.spawn(t, i6eMateID, 150, 150)
	w.i6eSettled(t, ddFar, ddNear, ddBroke, buff)
	debuff := w.i6fArmed(t, i6fRangeDebuffID, 0, -300)

	w.hit(t, mate, 10)
	i6fWantCasts(t, "settled 300 away", ddFar, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})
	i6fWantCasts(t, "settled 70 away", ddNear, map[modelskill.ID]i6fCast{wShortDDID: {me, 1000000, false}})
	w.i6eGaveUp(t, ddBroke, 1000)
	i6fWantCasts(t, "settled clan buff", buff, map[modelskill.ID]i6fCast{
		i6fClanBuffSkill: {me, 1000000, false}, wLongDDID: {me, 1000000, false},
	})
	i6fWantCasts(t, "fresh range debuff", debuff, map[modelskill.ID]i6fCast{i6eRangeDebuffID: {me, 1000000, false}})
}

// TestRoyalRushWizardRangeDebuffHoldsBack runs RoyalRushWizardRangeDebuff's
// clan-attacked hook where it does nothing of its own: on a draw of 50,
// and, every draw 0, once it gave up its spells.
func TestRoyalRushWizardRangeDebuffHoldsBack(t *testing.T) {
	t.Parallel()
	w := bootIndividual6f(t, raidBoot{}, inClan(i6fTemplate(i6fRangeDebuffID, nil)), inClan(warriorTemplate(i6eMateID, nil)))
	quit := w.i6fArmed(t, i6fRangeDebuffID, 300, 0)
	w.onQueue(t, quit, func() { quit.Scratch().SetInt(npc.IntAI0, 1) })
	mate := w.spawn(t, i6eMateID, 150, 150)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "given up", quit, nil)

	w.draw.Store(50)
	calm := w.i6fArmed(t, i6fRangeDebuffID, 0, 300)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "on a draw of 50", calm, nil)
}

// TestWizardHereticPriestKeepsHomeAndCallsForHelp ports the priest's
// created, attacked and no-desire hooks, every draw 0. As it spawns it
// wants to walk to its spawn point, weight 30. A hit while it neither
// attacks nor casts broadcasts event 10033 naming the attacker. Moved 200
// away with nothing left to do, its no-desire hook wants it back at its
// spawn point, weight 30, where its parent would wander.
func TestWizardHereticPriestKeepsHomeAndCallsForHelp(t *testing.T) {
	t.Parallel()
	w := bootIndividual6f(t, raidBoot{}, i6fPriest(nil, true), warriorTemplate(plwListenerID, nil))
	h := w.i6fArmed(t, i6fPriestID, 1500, 0)
	listener := w.spawn(t, plwListenerID, 1500, 100)
	home := script.NPCOf(h).SpawnLocation()
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire after spawning = %+v (%v), want weight 30 to %d,%d", d, ok, home.X, home.Y)
	}

	w.hit(t, h, 10)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), listener.ObjectID()); !slices.Contains(got, fmt.Sprintf("event 10033 %d", w.player.ObjectID())) {
		t.Fatalf("events heard after an idle hit = %q, want event 10033 naming the character", got)
	}

	w.teleport(t, h, 200, 0)
	w.onQueue(t, h, func() {
		script.NPCOf(h).RemoveAllAttackDesire()
		h.AI().Desires().Clear()
	})
	w.ticks(t, 2)
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire with nothing to do = %+v (%v), want weight 30 to %d,%d", d, ok, home.X, home.Y)
	}
	if d, ok := desire(h, actorai.IntentionWander); ok {
		t.Fatalf("wander desire with nothing to do = %+v, want none", d)
	}
}

// TestWizardHereticPriestAnswersHits ports the priest's attacked hook,
// every draw 0, for a hit from 300 away, the character on its hate list
// once its parent has heard the hit. Free to move (Hold 0) it wants its
// debuff on the character held in place and its long range DD magic
// closing in; holding its ground (Hold 1) it also wants the long range DD
// magic held, the two desires adding up. Short of MP for that magic it
// gives up its spells and wants to attack, weight 1000, when the character
// stands within its aggro range, and not from beyond it. Once it gave up
// its spells a hit makes it want to attack by the damage, 125, held in
// place when holding its ground, with no cast.
func TestWizardHereticPriestAnswersHits(t *testing.T) {
	t.Parallel()
	for _, hold := range []bool{false, true} {
		t.Run(fmt.Sprintf("hold %v", hold), func(t *testing.T) {
			t.Parallel()
			var ai npc.AIParams
			if hold {
				ai = npc.AIParams{"Hold": "1"}
			}
			// In no clan, a hit raises no clan call on another priest.
			tmpl := i6fPriest(ai, false)
			tmpl.Clans, tmpl.AggroRange = nil, 400
			w := bootIndividual6f(t, raidBoot{}, tmpl)
			me := w.player.ObjectID()
			h := w.i6fArmed(t, i6fPriestID, 1500, 0)
			w.i6fMoveNear(t, h, 300)

			w.hit(t, h, 10)
			want := map[modelskill.ID]i6fCast{i6fDebuffSkill: {me, 1000000, true}, wLongDDID: {me, 1000000, false}}
			if hold {
				want[wLongDDID] = i6fCast{me, 2000000, true}
			}
			i6fWantCasts(t, "after the hit", h, want)

			w.onQueue(t, h, func() {
				h.Scratch().SetInt(npc.IntAI0, 1)
				h.AI().Desires().Clear()
			})
			w.hit(t, h, 10)
			i6fWantCasts(t, "given up", h, nil)
			if d, ok := desire(h, actorai.IntentionAttack); !ok || d.Weight != 125 || d.FinalTarget.ObjectID() != me || d.MoveToTarget == hold {
				t.Fatalf("given up attack desire = %+v (%v), want weight 125 on the character held %v", d, ok, hold)
			}

			// Within its aggro range of 400 it may attack the character; from
			// 600 it may not, and wants no attack.
			for _, dx := range []int{300, 600} {
				broke := w.i6fArmed(t, i6fPriestID, 1500, 2*dx, wLongDDID)
				w.i6fMoveNear(t, broke, dx)
				w.hit(t, broke, 10)
				if got := broke.Scratch().Int(npc.IntAI0); got != 1 {
					t.Fatalf("priest short of MP %d away int slot 0 = %d, want 1 (spells given up)", dx, got)
				}
				d, ok := desire(broke, actorai.IntentionAttack)
				if dx == 600 {
					if ok {
						t.Fatalf("priest short of MP out of its aggro range attack desire = %+v, want none", d)
					}
					continue
				}
				if !ok || d.FinalTarget.ObjectID() != me || d.MoveToTarget == hold {
					t.Fatalf("priest short of MP attack desire = %+v (%v), want one on the character held %v", d, ok, hold)
				}
				if !hold && d.Weight != 1000 {
					t.Fatalf("priest short of MP attack desire weight = %v, want 1000", d.Weight)
				}
			}
		})
	}
}

// TestWizardHereticPriestAnswersItsClan ports the priest's clan-attacked
// hook, every draw 0, on a settled priest (9 AI cycles) 300 from the
// character whose clan member the character hits: free to move it wants
// its debuff held and its long range DD magic closing in, and holding its
// ground the long range one held too, adding up. A fresh priest (life time
// 7 or less) wants nothing.
func TestWizardHereticPriestAnswersItsClan(t *testing.T) {
	t.Parallel()
	for _, hold := range []bool{false, true} {
		t.Run(fmt.Sprintf("hold %v", hold), func(t *testing.T) {
			t.Parallel()
			var ai npc.AIParams
			if hold {
				ai = npc.AIParams{"Hold": "1"}
			}
			w := bootIndividual6f(t, raidBoot{}, i6fPriest(ai, false))
			caller := w.i6fArmed(t, i6fPriestID, 1500, 0)
			called := w.i6fArmed(t, i6fPriestID, 1500, 300)
			me := w.player.ObjectID()
			w.ticks(t, 9)
			fresh := w.i6fArmed(t, i6fPriestID, 1500, -300)
			w.i6fMoveNear(t, called, 300)

			w.hit(t, caller, 10)
			want := map[modelskill.ID]i6fCast{i6fDebuffSkill: {me, 1000000, true}, wLongDDID: {me, 1000000, false}}
			if hold {
				want[wLongDDID] = i6fCast{me, 2000000, true}
			}
			i6fWantCasts(t, "after the hit on its clan member", called, want)
			i6fWantCasts(t, "fresh, after the hit on its clan member", fresh, nil)
		})
	}
}

// TestWizardHereticPriestSeesAndBindsHealers ports the priest's
// see-creature hook for a character in no party, seen 100 away by a
// settled priest with an empty hate list. It broadcasts event 10033
// naming the character, hates it, and, free to move, wants its short
// range DD magic on it; holding its ground, its long range one held, on a
// draw under 80 with the character farther than 40. On a draw under 33 a
// paladin (5), a phoenix knight (90) or a cleric also draws its hold magic
// and its long range DD magic, both held; a warrior (1), or a cleric on a
// draw of 33, does not.
func TestWizardHereticPriestSeesAndBindsHealers(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		class int
		draw  int32
		hold  bool
		bind  bool
	}{{1, 0, false, false}, {5, 0, false, true}, {90, 0, false, true}, {15, 0, false, true}, {15, 33, false, false}, {1, 0, true, false}} {
		t.Run(fmt.Sprintf("class %d draw %d hold %v", row.class, row.draw, row.hold), func(t *testing.T) {
			t.Parallel()
			var ai npc.AIParams
			if row.hold {
				ai = npc.AIParams{"Hold": "1"}
			}
			w := bootIndividual6f(t, raidBoot{}, i6fPriest(ai, false), warriorTemplate(plwListenerID, nil))
			h := w.i6fArmed(t, i6fPriestID, 1500, 0)
			listener := w.spawn(t, plwListenerID, 1500, 100)
			me := w.player.ObjectID()
			w.ticks(t, 9)
			w.onQueue(t, h, func() { h.AI().Desires().Clear() })
			(&ind1World{warriorWorld: w.warriorWorld}).setClass(t, w.player, row.class)
			w.srv.ReadQueued(t, w.srv.Client)
			w.draw.Store(row.draw)

			if got := says(w.i6fLookAt(t, h), listener.ObjectID()); !slices.Contains(got, fmt.Sprintf("event 10033 %d", me)) {
				t.Fatalf("events heard after it saw the character = %q, want event 10033 naming it", got)
			}
			want := map[modelskill.ID]i6fCast{wShortDDID: {me, 1000000, false}}
			if row.hold {
				want = map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, true}}
			}
			if row.bind {
				want[i6fHoldSkill] = i6fCast{me, 1000000, true}
				want[wLongDDID] = i6fCast{me, 1000000, true}
			}
			i6fWantCasts(t, "after it saw the character", h, want)
			if got := h.AI().Hates().Hate(w.player); got <= 0 {
				t.Fatalf("hate after it saw the character = %v, want its default hate", got)
			}
		})
	}
}

// i6fTelPos are the priest's three teleport points, relative to the
// character's start.
var i6fTelPos = [3][2]int{{3000, 0}, {0, 3000}, {-3000, 0}}

// TestWizardHereticPriestSendsAPartyPlayerAway ports the see-creature
// hook's teleport. Seeing a player of a party larger than its
// party_members parameter in its spawn's territory, on a draw under 33 it
// sends the player to its first, second or third teleport point as a draw
// of 3 picks: the character sees one ordinary TeleportToLocation of itself
// there and stands there. A player alone, a party no larger than
// party_members, or NoPCTeleport set keeps the player where it is.
func TestWizardHereticPriestSendsAPartyPlayerAway(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name         string
		party        bool
		partyMembers int
		noPCTeleport int
		draw         int32
		point        int
	}{
		{"first point", true, 1, 0, 0, 0},
		{"second point", true, 1, 0, 1, 1},
		{"third point", true, 1, 0, 2, 2},
		{"alone", false, 0, 0, 0, -1},
		{"party not larger", true, 2, 0, 0, -1},
		{"no PC teleport", true, 1, 1, 0, -1},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := i6fPriest(nil, false)
			w := bootIndividual6f(t, raidBoot{}, tmpl)
			// The points lie around the character, whose place the boot
			// decides; the priest spawns after they are set.
			tmpl.AIParams = npc.AIParams{"party_members": fmt.Sprint(row.partyMembers), "NoPCTeleport": fmt.Sprint(row.noPCTeleport)}
			for i, d := range i6fTelPos {
				tmpl.AIParams[fmt.Sprintf("TelPosX%d", i+1)] = fmt.Sprint(w.at.X + d[0])
				tmpl.AIParams[fmt.Sprintf("TelPosY%d", i+1)] = fmt.Sprint(w.at.Y + d[1])
				tmpl.AIParams[fmt.Sprintf("TelPosZ%d", i+1)] = fmt.Sprint(w.at.Z)
			}
			if row.party {
				w.joinMate(t)
			}
			h := w.spawn(t, i6fPriestID, 1500, 0)
			me := w.player.ObjectID()
			w.draw.Store(row.draw)

			frames := w.i6fLookAt(t, h)
			jumps := framesOf(frames, serverpackets.OpcodeTeleportToLocation, me)
			x, y, _ := w.srv.PlayerPosition(t, me)
			if row.point < 0 {
				if len(jumps) != 0 {
					t.Fatalf("TeleportToLocation frames of the character = %d, want none", len(jumps))
				}
				return
			}
			want := location.Location{X: w.at.X + i6fTelPos[row.point][0], Y: w.at.Y + i6fTelPos[row.point][1]}
			if len(jumps) != 1 {
				t.Fatalf("TeleportToLocation frames of the character = %d, want 1", len(jumps))
			}
			if at, fast := teleportFrame(jumps[0]); at.X != want.X || at.Y != want.Y || fast {
				t.Fatalf("TeleportToLocation = %v fast=%v, want an ordinary one to %d,%d", at, fast, want.X, want.Y)
			}
			if x != want.X || y != want.Y {
				t.Fatalf("character at %d,%d, want %d,%d", x, y, want.X, want.Y)
			}
		})
	}
}

// TestWizardHereticPriestStrikesAgainAfterACast ports the priest's
// skill-finished hook, every draw 0: hit from 300 away, it casts its long
// range DD magic at the character, and the end of each cast, with the
// character on its hate list, wants it again.
func TestWizardHereticPriestStrikesAgainAfterACast(t *testing.T) {
	t.Parallel()
	tmpl := i6fPriest(nil, false)
	tmpl.Clans = nil
	w := bootIndividual6f(t, raidBoot{}, tmpl)
	h := w.i6fArmed(t, i6fPriestID, 1500, 0)
	w.i6fMoveNear(t, h, 300)
	w.hit(t, h, 10)
	w.i6eCastsWithin(t, h, wLongDDID, 2)
}

// i6fDen is an npcmaker spawning one heretic priest in a 400 by 400
// territory.
const i6fDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="wizard_heretic_priest_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="wizard_heretic_priest_den" territory="wizard_heretic_priest_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="22143" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// TestWizardHereticPriestWalksHomeFromOutside ports the priest's
// out-of-territory hook on a priest its npcmaker spawned. Free to move
// (Hold 0), arriving outside its territory it drops its attack desires and
// its hate, wants to walk to its spawn point with weight 100 more (on top
// of the 30 its created hook queued) and broadcasts event 10035 with
// argument 0. Holding its ground (Hold 1) it does none of that.
func TestWizardHereticPriestWalksHomeFromOutside(t *testing.T) {
	t.Parallel()
	for _, hold := range []bool{false, true} {
		t.Run(fmt.Sprintf("hold %v", hold), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(i6fDen), 0o644); err != nil {
				t.Fatalf("write spawnlist: %v", err)
			}
			table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				t.Fatalf("load spawnlist: %v", err)
			}
			var ai npc.AIParams
			if hold {
				ai = npc.AIParams{"Hold": "1"}
			}
			w := bootIndividual6f(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(table)}}, i6fPriest(ai, true), warriorTemplate(plwListenerID, nil))
			var h *npc.Hostile
			for _, obj := range w.srv.State.Objects() {
				if o, ok := obj.(*npc.Hostile); ok && o.NpcID() == int(i6fPriestID) {
					h = o
				}
			}
			if h == nil {
				t.Fatal("the den spawned no priest")
			}
			n, err := w.srv.NpcSpawns.AddSpawn(plwListenerID, 40, 800, 30, 0, false, 0)
			if err != nil {
				t.Fatalf("spawn the listener: %v", err)
			}
			w.srv.Settle(t)
			w.srv.ReadQueued(t, w.srv.Client)
			home := script.NPCOf(h).SpawnLocation()
			w.onQueue(t, h, func() { h.AI().Hates().Add(w.player, 100) })

			to := location.Location{X: 40, Y: 700, Z: 30}
			if !h.AI().AddMoveToDesire(to, 1e6) {
				t.Fatalf("AddMoveToDesire(%+v) = false, want the walk queued", to)
			}
			for i := 0; !h.IsMoving(); i++ {
				if i == 5 {
					t.Fatalf("no walk to %+v after five cycles", to)
				}
				cycle(t, w.srv)
			}
			for i := 0; h.IsMoving(); i++ {
				if i >= int(20*time.Second/move.PositionUpdateInterval) {
					t.Fatal("walk never arrived")
				}
				w.srv.TickPositions()
			}
			w.srv.Settle(t)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			if h.InTerritory() {
				t.Fatal("setup: the walk ended inside the territory")
			}
			heard := slices.Contains(says(frames, n.ObjectID()), "event 10035 0")
			d, ok := desire(h, actorai.IntentionMoveTo)
			homeward := ok && d.Location.X == home.X && d.Location.Y == home.Y
			hate := h.AI().Hates().Hate(w.player)
			if hold {
				if heard || hate != 100 || (homeward && d.Weight != 30) {
					t.Fatalf("held priest outside: heard 10035 %v, hate %v, move %+v (%v); want nothing changed", heard, hate, d, ok)
				}
				return
			}
			if !homeward || d.Weight != 30+100 {
				t.Fatalf("move desire after arriving outside = %+v (%v), want its walk home (30) raised by 100 to %d,%d", d, ok, home.X, home.Y)
			}
			if d, ok := desire(h, actorai.IntentionAttack); ok {
				t.Fatalf("attack desire after leaving its territory = %+v, want none", d)
			}
			if hate != 0 {
				t.Fatalf("hate after leaving its territory = %v, want none", hate)
			}
			if !heard {
				t.Fatalf("events heard = %q, want event 10035 0", says(frames, n.ObjectID()))
			}
		})
	}
}

// TestWizardHereticPriestSeesShortOfMP ports the see-creature hook's
// free-to-move short range answer when the priest cannot pay for the
// magic: a settled priest (Hold 0, aggro range 0, so it may not attack the
// character on its own) seeing the character 100 away gives up its spells
// and still wants to attack it, weight 1000, closing in. This answer alone
// skips the automatic attack check.
func TestWizardHereticPriestSeesShortOfMP(t *testing.T) {
	t.Parallel()
	w := bootIndividual6f(t, raidBoot{}, i6fPriest(nil, false))
	h := w.i6fArmed(t, i6fPriestID, 1500, 0, wShortDDID)
	me := w.player.ObjectID()
	w.ticks(t, 9)
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
	(&ind1World{warriorWorld: w.warriorWorld}).setClass(t, w.player, 1)
	w.srv.ReadQueued(t, w.srv.Client)

	w.i6fLookAt(t, h)
	if got := h.Scratch().Int(npc.IntAI0); got != 1 {
		t.Fatalf("priest short of MP int slot 0 = %d, want 1 (spells given up)", got)
	}
	d, ok := desire(h, actorai.IntentionAttack)
	if !ok || d.Weight != 1000 || d.FinalTarget.ObjectID() != me || !d.MoveToTarget {
		t.Fatalf("priest short of MP attack desire = %+v (%v), want weight 1000 on the character closing in", d, ok)
	}
	i6fWantCasts(t, "after it saw the character", h, nil)
}
