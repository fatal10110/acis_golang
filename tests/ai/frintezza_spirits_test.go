package ai

import (
	"fmt"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/frintezza"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The tomb's portrait spirits and dark choir captain (#198 PR 6).
const (
	fsBandmasterID = int32(18334)
	fsListenerID   = int32(18399)
	fsPhysicalID   = modelskill.ID(29501)
	fsDDMagicID    = modelskill.ID(29511)
	fsBombID       = modelskill.ID(5011)
	fsHoldID       = modelskill.ID(5012)
	fsVampiricID   = modelskill.ID(5013)
	fsArrowID      = int32(8192)
	fsDirector     = "FsDirector"
)

// fsWorld is the character beside the spirits and the bandmaster, a
// listener NPC recording the script events it hears.
type fsWorld struct {
	*warriorWorld
	draws *ind1Draws
	heard *ind1Heard
}

// fsSkills are the spirits' skills: their fixture physical special and DD
// magic, the bomb they cast on themselves, the hold, which leaves an effect
// of stack type fs_hold, and the vampiric touch.
func fsSkills() []modelskill.Definition {
	hold := ind1Skill(fsHoldID, 600, false)
	hold.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "fs_hold", StackOrder: 1}}
	return []modelskill.Definition{
		ind1Skill(fsPhysicalID, 600, false), ind1Skill(fsDDMagicID, 600, false), ind1Skill(fsBombID, 0, true),
		hold, ind1Skill(fsVampiricID, 600, false),
	}
}

// bootTombSpirits boots the character beside the spirits, the portrait that
// raises the plain ones, the bandmaster with bandmaster as its AI
// parameters, and the listener. Each spirit lists its typed attack skill and
// calls the other spirits of its clan within 600. None of them walks.
func bootTombSpirits(t *testing.T, bandmaster npc.AIParams) *fsWorld {
	t.Helper()
	heard := &ind1Heard{}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":               defaultnpc.New,
		"script.ai.boss.frintezza.EvilateA":             frintezza.EvilateA,
		"script.ai.boss.frintezza.PortraitSpirit":       frintezza.PortraitSpirit,
		"script.ai.boss.frintezza.PortraitSpiritWinged": frintezza.PortraitSpiritWinged,
		"script.ai.boss.frintezza.UndeadBandmaster":     frintezza.UndeadBandmaster,
		"test.FsListener": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{fsListenerID}, Hooks: script.Hooks{
				OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
					heard.mu.Lock()
					defer heard.mu.Unlock()
					heard.events = append(heard.events, fmt.Sprintf("event=%d arg1=%d arg2=%d", e.EventID, e.Arg1, e.Arg2))
				},
			}}
		},
		"test." + fsDirector: func() script.Script { return script.Script{} },
	}
	var list []script.Listing
	for _, path := range []string{
		"script.ai.individual.DefaultNpc", "script.ai.boss.frintezza.EvilateA",
		"script.ai.boss.frintezza.PortraitSpirit", "script.ai.boss.frintezza.PortraitSpiritWinged",
		"script.ai.boss.frintezza.UndeadBandmaster", "test.FsListener", "test." + fsDirector,
	} {
		list = append(list, script.Listing{Path: path})
	}
	spirit := warriorTemplate(fhSpiritID, nil)
	spirit.SkillsByType = map[npc.SkillType]modelskill.Ref{npc.SkillTypePhysicalSpecial: {ID: fsPhysicalID, Level: 1}}
	winged := warriorTemplate(fhWingedID, nil)
	winged.SkillsByType = map[npc.SkillType]modelskill.Ref{npc.SkillTypeDDMagic: {ID: fsDDMagicID, Level: 1}}
	params := npc.AIParams{
		"SpawnPosX": fmt.Sprint(fhSpiritAt[0]), "SpawnPosY": fmt.Sprint(fhSpiritAt[1]),
		"SpawnPosZ": fmt.Sprint(fhSpiritAt[2]), "SpawnAngle": fmt.Sprint(fhSpiritAt[3]),
	}
	templates := []*npc.Template{
		spirit, winged, warriorTemplate(fhEvilateAID, params),
		warriorTemplate(fsBandmasterID, bandmaster), warriorTemplate(fsListenerID, nil),
	}
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		tmpl.CanMove, tmpl.HPMax = false, 100000
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	spirit.Clans, spirit.ClanRange = []string{"fs_spirits"}, 600
	winged.Clans, winged.ClanRange = []string{"fs_spirits"}, 600
	draws := &ind1Draws{}
	draws.set(99)
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithScriptSkills(fsSkills()...),
		gameservertest.WithItemTemplates(fsItems()),
	}}, templates...)
	return &fsWorld{warriorWorld: w, draws: draws, heard: heard}
}

// fsItems are the boot's item templates and the soul breaking arrow.
func fsItems() *item.Table {
	templates := append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: fsArrowID, Name: "Soul Breaking Arrow", Kind: item.KindEtcItem, Duration: -1,
		Stackable: true, Dropable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
	})
	return item.NewTable(templates)
}

// spirit places a spirit of id dx from the character, with no master, its
// cast seam over fsSkills.
func (w *fsWorld) spirit(t *testing.T, id int32, dx int) *npc.Hostile {
	t.Helper()
	h := w.spawn(t, id, dx, 0)
	w.srv.InstallNPCCastSeam(t, h, modelskill.NewTable(fsSkills()))
	return h
}

// event sends h script event eventID with arg1 from the director.
func (w *fsWorld) event(t *testing.T, h *npc.Hostile, eventID, arg1 int32) {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), fsDirector, func(s *script.Script, _ *script.Player) {
		s.SendScriptEvent(script.NPCOf(h), eventID, arg1, 0)
	}) {
		t.Fatal("the script event panicked")
	}
	w.srv.Settle(t)
}

// fsAttackSkill is the typed attack skill the spirit of id lists.
func fsAttackSkill(id int32) modelskill.ID {
	if id == fhWingedID {
		return fsDDMagicID
	}
	return fsPhysicalID
}

// wantSpiritPress fails unless h wants its typed attack skill on the
// character, weight 1000000, and to attack it by 50.
func (w *fsWorld) wantSpiritPress(t *testing.T, when string, h *npc.Hostile, id int32) {
	t.Helper()
	wantCast(t, when, h, fsAttackSkill(id), w.player, 1000000, true)
	if d, ok := attackDesireOn(h, w.player.ObjectID()); !ok || d.Weight != 50 {
		t.Fatalf("%s: attack desire = %+v, %v, want weight 50 on the character", when, d, ok)
	}
}

// wantSpiritOn fails unless h wants to cast skill id on the character and
// to attack it, after an AI cycle that may have worn their weights down.
func (w *fsWorld) wantSpiritOn(t *testing.T, when string, h *npc.Hostile, id modelskill.ID) {
	t.Helper()
	me := w.player.ObjectID()
	if d, ok := castDesireOf(h, id); !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != me || d.Weight <= 0 || d.Weight > 1000000 {
		t.Fatalf("%s: cast desire of %d = %+v, %v, want one on the character", when, id, d, ok)
	}
	if d, ok := attackDesireOn(h, me); !ok || d.Weight <= 0 {
		t.Fatalf("%s: attack desire = %+v, %v, want one on the character", when, d, ok)
	}
}

// wantNoPress fails when h wants to cast or attack.
func wantNoPress(t *testing.T, when string, h *npc.Hostile) {
	t.Helper()
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionCast || d.Kind == actorai.IntentionAttack {
			t.Fatalf("%s: desire %+v, want no cast or attack", when, d)
		}
	}
}

// fsSpiritIDs are both spirits: the plain one fights with its physical
// special skill, the winged one with its DD magic.
var fsSpiritIDs = []int32{fhSpiritID, fhWingedID}

// TestPortraitSpiritsWanderUntilMasterless runs both spirits' created hook
// and timer 5000 (#2164). Appearing, a spirit clears its ai slot 4 and
// quest slots 1 and 2 and wants to wander every 200 s, weight 5. Every
// 10 s it checks its master: with none, and neither attacking nor casting,
// it leaves the world.
func TestPortraitSpiritsWanderUntilMasterless(t *testing.T) {
	t.Parallel()
	for _, id := range fsSpiritIDs {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			t.Parallel()
			w := bootTombSpirits(t, nil)
			h := w.spirit(t, id, 300)
			if d, ok := desire(h, actorai.IntentionWander); !ok || d.Weight != 5 || d.Timer != 200 {
				t.Fatalf("wander desire = %+v, %v, want timer 200, weight 5", d, ok)
			}
			n := script.NPCOf(h)
			for _, slot := range []script.IntSlot{script.SlotIntAI4, npc.IntQuest1, npc.IntQuest2} {
				if got := n.Int(slot); got != 0 {
					t.Fatalf("slot %d after the created hook = %d, want 0", slot, got)
				}
			}
			w.srv.Advance(t, 10*time.Second-time.Millisecond)
			if !w.inWorld(h) {
				t.Fatal("the spirit left before its first master check")
			}
			w.srv.Advance(t, time.Millisecond)
			if w.inWorld(h) {
				t.Fatal("the masterless wandering spirit is still in the world at 10 s")
			}
		})
	}
}

// TestPortraitSpiritOutlivesNoLiveMaster runs timer 5000 on a spirit an
// EvilateA raised: it stays while its portrait lives, re-checking every
// 10 s, and leaves at the first check after the portrait died.
func TestPortraitSpiritOutlivesNoLiveMaster(t *testing.T) {
	t.Parallel()
	w := bootTombSpirits(t, nil)
	portrait := w.spawn(t, fhEvilateAID, 100, 0)
	w.srv.Advance(t, 45*time.Second)
	spirits := w.liveNPCs(fhSpiritID)
	if len(spirits) != 1 {
		t.Fatalf("spirits at 45 s = %d, want 1", len(spirits))
	}
	w.srv.Advance(t, 19*time.Second)
	if !w.inWorld(spirits[0]) {
		t.Fatal("the spirit left while its portrait lives")
	}
	w.kill(t, portrait)
	w.srv.Advance(t, time.Second-time.Millisecond)
	if !w.inWorld(spirits[0]) {
		t.Fatal("the spirit left before its check")
	}
	w.srv.Advance(t, time.Millisecond)
	if w.inWorld(spirits[0]) {
		t.Fatal("the spirit is still in the world after its portrait died")
	}
}

// TestPortraitSpiritFightsWhatHitsIt runs both spirits' attacked hook and
// timers 2001 and 5000. A hit by the character makes the spirit want its
// typed attack skill on it, weight 1000000, and to attack it by 50, and
// stores the tick; 5 s later timer 2001 finds only 5 ticks gone, not more,
// and wants no hold. Fighting, the masterless spirit outlasts its 10 s
// check while the character stands within 5000, and leaves at the check
// once the character stands farther.
func TestPortraitSpiritFightsWhatHitsIt(t *testing.T) {
	t.Parallel()
	for _, id := range fsSpiritIDs {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			t.Parallel()
			w := bootTombSpirits(t, nil)
			h := w.spirit(t, id, 150)
			w.srv.Advance(t, 2*time.Second)
			var tick int32
			w.srv.RunScript(t, w.player.ObjectID(), fsDirector, func(s *script.Script, _ *script.Player) { tick = s.CurrentTick() })
			w.hit(t, h, 1)
			w.wantSpiritPress(t, "after the hit", h, id)
			if other := fsAttackSkill(fhSpiritID + fhWingedID - id); other != fsAttackSkill(id) {
				noCast(t, "after the hit", h, other)
			}
			if got := script.NPCOf(h).Int(npc.IntQuest1); got != tick {
				t.Fatalf("stored tick = %d, want %d", got, tick)
			}
			w.srv.Advance(t, 5*time.Second)
			noCast(t, "at timer 2001", h, fsHoldID)
			w.ticks(t, 2)
			if in := h.AI().CurrentIntention(); in != actorai.IntentionAttack && in != actorai.IntentionCast {
				t.Fatalf("setup: intention %v, want attack or cast", in)
			}
			w.srv.Advance(t, 3*time.Second)
			if !w.inWorld(h) {
				t.Fatal("the fighting spirit left with the character within 5000")
			}
			obj, ok := w.srv.State.Player(w.player.ObjectID())
			if !ok {
				t.Fatal("the character is not in the world")
			}
			x, y, z := h.Position()
			if err := w.srv.State.Move(obj, x+5001, y, z); err != nil {
				t.Fatalf("move away: %v", err)
			}
			if in := h.AI().CurrentIntention(); in != actorai.IntentionAttack && in != actorai.IntentionCast {
				t.Fatalf("setup: intention %v after the move, want attack or cast", in)
			}
			w.srv.Advance(t, 10*time.Second)
			if w.inWorld(h) {
				t.Fatal("the spirit is still in the world with the character beyond 5000")
			}
		})
	}
}

// TestPortraitSpiritAnswersItsClanAndWhatItSees runs both spirits'
// clan-attacked and see-creature hooks: past 7 AI cycles a spirit wants its
// typed attack skill, weight 1000000, and to attack by 50, on the playable
// that hit a clan member, and on the one it comes to see; younger it wants
// neither.
func TestPortraitSpiritAnswersItsClanAndWhatItSees(t *testing.T) {
	t.Parallel()
	for _, id := range fsSpiritIDs {
		t.Run(fmt.Sprintf("clan %d", id), func(t *testing.T) {
			t.Parallel()
			w := bootTombSpirits(t, nil)
			struck := w.spirit(t, fhSpiritID+fhWingedID-id, 150)
			h := w.spirit(t, id, 250)
			w.hit(t, struck, 1)
			wantNoPress(t, "young, its clan member hit", h)
			w.ticks(t, 8)
			if h.LifeTime() <= 7 {
				t.Fatalf("setup: life time %d, want past 7", h.LifeTime())
			}
			w.hit(t, struck, 1)
			w.wantSpiritPress(t, "settled, its clan member hit", h, id)
		})
		for _, settled := range []bool{false, true} {
			t.Run(fmt.Sprintf("see %d settled %v", id, settled), func(t *testing.T) {
				t.Parallel()
				w := bootTombSpirits(t, nil)
				h := w.spirit(t, id, 3000)
				if settled {
					w.ticks(t, 8)
				}
				obj, ok := w.srv.State.Player(w.player.ObjectID())
				if !ok {
					t.Fatal("the character is not in the world")
				}
				x, y, z := h.Position()
				if err := w.srv.State.Move(obj, x-100, y, z); err != nil {
					t.Fatalf("approach: %v", err)
				}
				w.ticks(t, 1)
				if settled {
					w.wantSpiritOn(t, "settled, it saw the character", h, fsAttackSkill(id))
				} else {
					wantNoPress(t, "young, it saw the character", h)
				}
			})
		}
	}
}

// TestPortraitSpiritPressesAHeldPlayer runs both spirits' skill-finished
// hook on a character held by 5012: after each finished cast the spirit
// wants the vampiric touch on its player target, three times at most,
// counting in quest slot 2, and to attack it by 50.
func TestPortraitSpiritPressesAHeldPlayer(t *testing.T) {
	t.Parallel()
	for _, id := range fsSpiritIDs {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			t.Parallel()
			w := bootTombSpirits(t, nil)
			h := w.spirit(t, id, 150)
			me := w.player.ObjectID()
			hold := modelskill.Ref{ID: fsHoldID, Level: 1}
			w.srv.RunScript(t, me, fsDirector, func(s *script.Script, p *script.Player) { s.ApplyEffects(hold, script.NPCOf(h), p) })
			w.srv.AdvanceUntil(t, "the hold on the character", func() bool {
				var level int32
				w.srv.RunScript(t, me, fsDirector, func(s *script.Script, p *script.Player) { level = s.AbnormalLevel(p, hold) })
				return level > 0
			})
			w.hit(t, h, 1)
			n := script.NPCOf(h)
			w.srv.AdvanceUntil(t, "the first vampiric touch wanted", func() bool {
				w.ticks(t, 1)
				return n.Int(npc.IntQuest2) == 1
			})
			w.wantSpiritOn(t, "after the first finished cast", h, fsVampiricID)
			w.srv.AdvanceUntil(t, "the third vampiric touch wanted", func() bool {
				w.ticks(t, 1)
				return n.Int(npc.IntQuest2) == 3
			})
			for range 3 {
				w.srv.Advance(t, 5*time.Second)
				w.ticks(t, 3)
			}
			if got := n.Int(npc.IntQuest2); got != 3 {
				t.Fatalf("vampiric touches = %d, want 3 at most", got)
			}
		})
	}
}

// TestPortraitSpiritScriptEvents runs both spirits' script-event hook,
// which reads arg1 whatever the event id: 20000 drops every desire and
// casts the bomb on itself, the spirit decaying once it lands; 40000 takes
// it out of the world; 50000 sets ai slot 4 and drops every desire, after
// which a hit draws no press; another arg1 changes nothing.
func TestPortraitSpiritScriptEvents(t *testing.T) {
	t.Parallel()
	for _, id := range fsSpiritIDs {
		t.Run(fmt.Sprintf("bomb %d", id), func(t *testing.T) {
			t.Parallel()
			w := bootTombSpirits(t, nil)
			h := w.spirit(t, id, 150)
			w.event(t, h, 7, 20000)
			if _, ok := desire(h, actorai.IntentionWander); ok {
				t.Fatal("the spirit still wants to wander after event 20000")
			}
			wantCast(t, "after event 20000", h, fsBombID, h, 1000000, true)
			w.srv.AdvanceUntil(t, "the bomb decayed the spirit", func() bool {
				w.ticks(t, 1)
				return !w.inWorld(h)
			})
			if h.CurrentHP() != h.MaxHP() {
				t.Fatalf("the bombed spirit is at HP %d of %d, want it decayed whole", h.CurrentHP(), h.MaxHP())
			}
		})
		t.Run(fmt.Sprintf("delete %d", id), func(t *testing.T) {
			t.Parallel()
			w := bootTombSpirits(t, nil)
			h := w.spirit(t, id, 150)
			w.event(t, h, 40000, 0)
			if !w.inWorld(h) {
				t.Fatal("event id 40000 with arg1 0 took the spirit out")
			}
			w.event(t, h, 0, 40000)
			if w.inWorld(h) {
				t.Fatal("the spirit is still in the world after arg1 40000")
			}
		})
		t.Run(fmt.Sprintf("calm %d", id), func(t *testing.T) {
			t.Parallel()
			w := bootTombSpirits(t, nil)
			h := w.spirit(t, id, 150)
			w.event(t, h, 0, 50000)
			if got := script.NPCOf(h).Int(script.SlotIntAI4); got != 1 {
				t.Fatalf("ai slot 4 after arg1 50000 = %d, want 1", got)
			}
			if got := h.AI().Desires().Snapshot(); len(got) != 0 {
				t.Fatalf("desires after arg1 50000 = %+v, want none", got)
			}
			w.hit(t, h, 1)
			wantNoPress(t, "calmed, hit", h)
		})
	}
}

// TestUndeadBandmasterShoutsAndMarks runs the bandmaster's created and
// script-event hooks. Its ai slot 0 starts at 1; event 10032 with arg1 0,
// 2, 4 or 6 sets it to arg1, any other arg1 or event leaves it. With a
// Shoutman AI parameter of 1 it shouts 1010644 as it appears, and 1010645
// or 1010646 on arg1 10042 or 10043 of any event; without it, never.
func TestUndeadBandmasterShoutsAndMarks(t *testing.T) {
	t.Parallel()
	for _, shoutman := range []bool{false, true} {
		t.Run(fmt.Sprint("shoutman ", shoutman), func(t *testing.T) {
			t.Parallel()
			var params npc.AIParams
			if shoutman {
				params = npc.AIParams{"Shoutman": "1"}
			}
			w := bootTombSpirits(t, params)
			h, frames := w.spawnSeen(t, fsBandmasterID, location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z})
			var want []string
			if shoutman {
				want = []string{npcstringText(t, 1010644)}
			}
			if got := shouts(t, frames, h.ObjectID()); !slices.Equal(got, want) {
				t.Fatalf("shouts as it appeared = %q, want %q", got, want)
			}
			slot := func() int32 { return script.NPCOf(h).Int(script.SlotIntAI0) }
			if got := slot(); got != 1 {
				t.Fatalf("ai slot 0 after the created hook = %d, want 1", got)
			}
			for _, step := range []struct {
				eventID, arg1, want int32
			}{
				{10032, 4, 4}, {10032, 3, 4}, {10031, 2, 4}, {10032, 0, 0}, {10032, 6, 6}, {10032, 2, 2},
			} {
				w.event(t, h, step.eventID, step.arg1)
				if got := slot(); got != step.want {
					t.Fatalf("ai slot 0 after event %d arg1 %d = %d, want %d", step.eventID, step.arg1, got, step.want)
				}
			}
			w.srv.ReadQueued(t, w.srv.Client)
			for _, step := range []struct {
				eventID, arg1, line int32
			}{
				{0, 10042, 1010645}, {10032, 10043, 1010646}, {10042, 0, 0},
			} {
				w.event(t, h, step.eventID, step.arg1)
				var want []string
				if shoutman && step.line != 0 {
					want = []string{npcstringText(t, step.line)}
				}
				if got := shouts(t, w.srv.ReadQueued(t, w.srv.Client), h.ObjectID()); !slices.Equal(got, want) {
					t.Fatalf("shouts on event %d arg1 %d = %q, want %q", step.eventID, step.arg1, got, want)
				}
			}
		})
	}
}

// TestUndeadBandmasterCallsOnAHit runs the bandmaster's attacked hook: a
// draw under 33 in 100 tells the NPCs within 1500 event 0, arg1 10033,
// arg2 the attacker's object id; a draw of 33 tells nobody.
func TestUndeadBandmasterCallsOnAHit(t *testing.T) {
	t.Parallel()
	w := bootTombSpirits(t, nil)
	h := w.spawn(t, fsBandmasterID, 100, 0)
	w.spawn(t, fsListenerID, 1500, 0)
	w.spawn(t, fsListenerID, 1700, 0)
	w.draws.set(99, 33)
	w.hit(t, h, 1)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("events heard on a draw of 33 = %q, want none", got)
	}
	w.draws.set(99, 32)
	w.hit(t, h, 1)
	if got, want := w.heard.take(), []string{fmt.Sprintf("event=0 arg1=10033 arg2=%d", w.player.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("events heard on a draw of 32 = %q, want %q", got, want)
	}
}

// TestUndeadBandmasterDropsTheArrow runs the bandmaster's dying hook: with
// ai slot 0 at 0, 2, 4 or 6 it drops one soul breaking arrow (8192) for its
// killer; at its starting 1 it drops none.
func TestUndeadBandmasterDropsTheArrow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		arg1 int32
		drop bool
	}{
		{-1, false}, {0, true}, {2, true}, {4, true}, {6, true},
	} {
		t.Run(fmt.Sprint(tc.arg1), func(t *testing.T) {
			t.Parallel()
			w := bootTombSpirits(t, nil)
			h := w.spawn(t, fsBandmasterID, 100, 0)
			if tc.arg1 >= 0 {
				w.event(t, h, 10032, tc.arg1)
			}
			w.hit(t, h, h.CurrentHP())
			if !h.Dead() {
				t.Fatal("setup: the bandmaster survived")
			}
			w.srv.Advance(t, 3*time.Second)
			var arrows []int
			for _, obj := range w.srv.State.Objects() {
				if g, ok := obj.(*grounditem.Item); ok && g.ItemID() == fsArrowID {
					arrows = append(arrows, g.Count())
				}
			}
			var want []int
			if tc.drop {
				want = []int{1}
			}
			if !slices.Equal(arrows, want) {
				t.Fatalf("arrows on the ground = %v, want %v", arrows, want)
			}
		})
	}
}
