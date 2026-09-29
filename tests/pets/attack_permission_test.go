package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// wholeWorldForm is a zone volume covering every test spawn.
func wholeWorldForm(t *testing.T) zone.Form {
	t.Helper()
	form, err := zone.NewCuboid(-100_000, 100_000, -100_000, 100_000, -10_000, 10_000)
	if err != nil {
		t.Fatalf("zone form: %v", err)
	}
	return form
}

// arenaZones is a zone index whose one arena covers every test spawn.
func arenaZones(t *testing.T) *zone.Index {
	t.Helper()
	zones := zone.NewIndex()
	zones.Add(zone.NewArena(1, wholeWorldForm(t)))
	return zones
}

// bootKarmaOwnerWithCollar is bootOwnerWithCollarOpts for an owner of the
// given level and karma, set on its row before it enters the world.
func bootKarmaOwnerWithCollar(t *testing.T, level, karma int, extra ...gameservertest.Option) *petWorld {
	t.Helper()
	srv := bootPets(t, extra...)
	ownerID := srv.SoleObjectID(t)
	ch, err := srv.Chars.Get(petCtx(), ownerID)
	if err != nil {
		t.Fatalf("load owner: %v", err)
	}
	ch.CharLevel, ch.KarmaPoints = level, karma
	if err := srv.Chars.Save(petCtx(), ch.SaveState()); err != nil {
		t.Fatalf("save owner: %v", err)
	}
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: srv.GiveItem(t, ownerID, wolfCollarID, 1), seeded: map[int32][]int32{}}
	startInWorld(t, h.client)
	return h
}

// blessPlayer lands Blessing of Protection on a live player.
func blessPlayer(t *testing.T, p attackable.Combatant) {
	t.Helper()
	holder := p.(interface {
		effect.Actor
		EffectList() *effect.List
		Queue() *sim.Queue
	})
	e, err := effect.New(effect.Skill{ID: 1323, Level: 1}, modelskill.EffectTemplate{Name: "ProtectionBlessing", Time: 30})
	if err != nil {
		t.Fatalf("effect.New(ProtectionBlessing): %v", err)
	}
	e.Effector, e.Effected = holder, holder
	runOn(t, holder.Queue(), func() { holder.EffectList().Add(e) })
	if !holder.EffectList().IsAffected(effect.FlagProtectionBlessing) {
		t.Fatal("Blessing of Protection did not land")
	}
}

// petRelationAutoAttackable reads the auto-attackable flag of the last
// RelationChanged for petID among frames.
func petRelationAutoAttackable(frames [][]byte, petID int32) (auto, seen bool) {
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeRelationChanged {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != petID {
			continue
		}
		r.ReadInt32() // relation
		auto, seen = r.ReadInt32() == 1, true
	}
	return auto, seen
}

// hasSystemMessage reports whether frames carry the system message id.
func hasSystemMessage(frames [][]byte, id int) bool {
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && wire.NewReader(f[1:]).ReadInt32() == int32(id) {
			return true
		}
	}
	return false
}

// TestPetInsideArenaIsAttackedWithoutForce puts an unflagged owner's pet and
// another player inside an arena: the pet's spawn RelationChanged marks it
// auto-attackable for the other player, whose plain second click on the pet
// attacks it.
func TestPetInsideArenaIsAttackedWithoutForce(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithZones(arenaZones(t))})
	other := h.joinSecondPlayer(t, "Hitter")
	landEveryHit(t, h.srv, other.id)
	pet, _ := h.spawnWolf(t)
	if !pet.InPvPZone() {
		t.Fatal("pet is not inside the arena")
	}
	if auto, seen := petRelationAutoAttackable(drainFrames(t, other.client), pet.ObjectID()); !seen || !auto {
		t.Fatalf("pet spawn RelationChanged for the other player: seen %v auto-attackable %v, want auto-attackable", seen, auto)
	}
	x, y, z := pet.Position()

	other.client.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, other.client)
	full := pet.HP()
	other.client.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	h.srv.AdvanceUntil(t, "plain attack landing on a pet inside an arena", func() bool { return pet.HP() < full })
}

// TestPetDeathPenaltyInsideCombatZones kills a level-10 wolf with 700
// experience: inside an arena it keeps its experience, while an active siege
// battlefield, a PvP zone too, still takes the 29-experience penalty.
func TestPetDeathPenaltyInsideCombatZones(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		zone    func(t *testing.T) zone.Kind
		wantExp int64
	}{
		{"arena", func(t *testing.T) zone.Kind { return zone.NewArena(1, wholeWorldForm(t)) }, 700},
		{"active siege", func(t *testing.T) zone.Kind {
			siege, err := zone.NewSiege(1, wholeWorldForm(t), commons.NewStatSet())
			if err != nil {
				t.Fatalf("siege zone: %v", err)
			}
			siege.SetActive(true)
			return siege
		}, 671},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			zones := zone.NewIndex()
			zones.Add(tt.zone(t))
			h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{penaltyWolfTemplate(), treeTemplate()})),
				gameservertest.WithZones(zones),
			})
			if err := h.srv.Pets.Save(petCtx(), h.collarID, pet.State{
				Level: wolfLevel, Exp: 700, CurHP: wolfMaxHP, CurMP: wolfMaxMP, Fed: wolfMaxMeal,
			}); err != nil {
				t.Fatalf("seed pets row: %v", err)
			}
			wolf, _ := h.spawnWolf(t)
			if !wolf.InPvPZone() {
				t.Fatalf("wolf is not inside the %s", tt.name)
			}
			drainUntilQuiet(t, h.client)

			obj, _ := h.srv.State.Player(h.ownerID)
			wolf.ReduceHP(wolf.HP()+1, obj.(attackable.Combatant), modelskill.Definition{})
			h.srv.AdvanceUntil(t, "wolf dead", wolf.Dead)
			h.srv.Settle(t)
			if got := wolf.Exp(); got != tt.wantExp {
				t.Fatalf("wolf killed inside an %s: exp = %d, want %d", tt.name, got, tt.wantExp)
			}
		})
	}
}

// TestKarmaOwnersPetCannotAttackBlessedLowLevelPlayer has a level-30 owner
// with karma command its pet onto a level-1 player under Blessing of
// Protection. Outside a PvP zone the owner is told TARGET_IS_INCORRECT and
// the pet never swings; inside an arena, where neither needs force, the pet
// attacks.
func TestKarmaOwnersPetCannotAttackBlessedLowLevelPlayer(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		arena bool
	}{
		{"outside a PvP zone", false},
		{"inside an arena", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var extra []gameservertest.Option
			if tt.arena {
				extra = append(extra, gameservertest.WithZones(arenaZones(t)))
			}
			h := bootKarmaOwnerWithCollar(t, 30, 500, extra...)
			wolf, _ := h.spawnWolf(t)
			victim := h.joinSecondPlayer(t, "Blessed")
			blessPlayer(t, victim.actor)
			drainUntilQuiet(t, h.client)
			x, y, z := victim.actor.Position()

			h.client.Send(encodeAction(victim.id, int32(x), int32(y), int32(z), false))
			drainUntilQuiet(t, h.client)
			// Outside a PvP zone the blessed player is only attackable
			// with force; the gate refuses the forced command.
			h.client.Send(encodeRequestActionUse(petAttackAction, !tt.arena))

			if tt.arena {
				h.srv.AdvanceUntil(t, "the pet's swing at the blessed player", wolf.IsAttackingNow)
				if hasSystemMessage(drainFrames(t, h.client), serverpackets.SystemMessageTargetIncorrect) {
					t.Fatal("pet attack inside an arena answered TARGET_IS_INCORRECT")
				}
				return
			}

			frames := advanceCollecting(t, h, 2*time.Second)
			if !hasSystemMessage(frames, serverpackets.SystemMessageTargetIncorrect) {
				t.Fatalf("refused pet attack frames = %x, want TARGET_IS_INCORRECT to the owner", frameOpcodes(frames))
			}
			if n := petAttacks(frames, wolf); n != 0 || wolf.IsAttackingNow() {
				t.Fatalf("refused pet attack swung %d times", n)
			}
		})
	}
}
