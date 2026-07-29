package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// signetIDAllocator hands out fresh world object ids for a spawned signet
// actor.
type signetIDAllocator interface {
	NextID() (int32, error)
}

// signetTemplates resolves an NPC template by id, the source of a signet
// actor's spawnable shape.
type signetTemplates interface {
	Get(id int) (*npc.Template, bool)
}

// signetPositioned is implemented by a caster whose world position a
// signet actor spawns at.
type signetPositioned interface {
	Position() (x, y, z int)
}

// signetIdentified optionally reports a caster's world object id, recorded
// as the spawned actor's owner; a caster without one spawns an unowned
// actor.
type signetIdentified interface {
	ObjectID() int32
}

// signetHeaded optionally reports a caster's facing, applied to the
// spawned actor; a caster without one spawns facing 0.
type signetHeaded interface {
	Heading() int
}

// signetCastTarget is the minimal surface a found signet target must
// expose to appear as the target endpoint of a broadcast skill-use/launch
// packet pair.
type signetCastTarget interface {
	ObjectID() int32
	Position() (x, y, z int)
}

// signetNearby is implemented by a world object a signet tick's radius
// scan can find: alive, and not itself a door or ground item, since
// neither implements Dead().
type signetNearby interface {
	Dead() bool
}

// signetPeaceZoned optionally reports whether a found object sits in a
// peace zone; an object without one is never excluded on that basis.
type signetPeaceZoned interface {
	InPeaceZone() bool
}

// signetDancer optionally reports whether a found object's active effects
// list can be inspected and stripped, for the SignetNoise family's
// dance-cancel tick.
type signetDancer interface {
	EffectList() *effect.List
}

// signetUnsummonable is implemented by a found summon actor the
// SignetAntiSummon family can dismiss.
type signetUnsummonable interface {
	Dead() bool
	Unsummon()
	ObjectID() int32
	Position() (x, y, z int)
	BroadcastFrame(wire.Frame)
}

type signetHandler struct {
	defs      Definitions
	templates signetTemplates
	ids       signetIDAllocator
	world     *world.State
}

func (signetHandler) Types() []string { return []string{"SIGNET", "SIGNET_CASTTIME"} }

func (h signetHandler) Use(cast Cast) {
	if alikeDead(cast.Caster) || h.templates == nil || h.ids == nil || h.world == nil {
		return
	}

	if skillTypeKey(cast.Skill.SkillType) == "SIGNET_CASTTIME" {
		h.useCasttime(cast)
		return
	}
	h.useSignet(cast)
}

// useSignet spawns the actor immediately, at its ground target when the
// caster supplies one for a ground-targeted skill, then applies the skill's
// own effect templates onto that actor. If none of the templates yield a
// recognized effect, the actor is despawned immediately since nothing will
// ever call Despawn on its behalf.
func (h signetHandler) useSignet(cast Cast) {
	actor, ok := h.spawnActor(cast.Caster, cast.Skill)
	if !ok {
		return
	}

	meta := signetEffectMeta(cast.Skill)
	added := false
	for _, tmpl := range cast.Skill.Effects {
		e := h.newActorEffect(cast.Skill, meta, tmpl, actor)
		if e == nil {
			continue
		}
		actor.EffectList().Add(e)
		added = true
	}
	if !added {
		actor.Despawn()
	}
}

// useCasttime applies a self-targeted effect (SignetMDam) whose own
// onStart spawns the actor, since the caster - not a pre-spawned actor -
// is the effect's target.
func (h signetHandler) useCasttime(cast Cast) {
	target, ok := cast.Caster.(effectListTarget)
	if !ok {
		return
	}
	list := target.EffectList()
	if list == nil {
		return
	}

	meta := signetEffectMeta(cast.Skill)
	for _, tmpl := range cast.Skill.SelfEffects {
		if tmpl.Name != "SignetMDam" {
			continue
		}
		e := h.newSignetMDamEffect(cast.Caster, cast.Skill, meta, tmpl)
		list.Add(e)
	}
}

// spawnActor builds and spawns the world actor a signet-family skill
// carries its area effect on, attributed to and positioned at caster.
func (h signetHandler) spawnActor(caster any, def modelskill.Definition) (*npc.EffectPoint, bool) {
	tmpl, ok := h.templates.Get(def.EffectNpcID)
	if !ok {
		return nil, false
	}
	id, err := h.ids.NextID()
	if err != nil {
		return nil, false
	}

	var ownerID int32
	if oid, ok := caster.(signetIdentified); ok {
		ownerID = oid.ObjectID()
	}
	actor, err := npc.NewEffectPoint(id, tmpl, ownerID)
	if err != nil {
		return nil, false
	}
	actor.SetWorld(h.world)

	pos, ok := caster.(signetPositioned)
	if !ok {
		return nil, false
	}
	x, y, z := pos.Position()
	if def.Target == modelskill.TargetGround {
		if ground, ok := caster.(interface{ GroundTarget() (int, int, int) }); ok {
			x, y, z = ground.GroundTarget()
		}
	}
	heading := 0
	if hd, ok := caster.(signetHeaded); ok {
		heading = hd.Heading()
	}
	actor.Spawn(x, y, z, heading)
	return actor, true
}

// signetEffectMeta builds the effect metadata a signet-family effect
// carries, shared by every kind this skill applies.
func signetEffectMeta(def modelskill.Definition) effect.Skill {
	return effect.Skill{
		ID:         def.ID,
		Level:      def.Level,
		SkillType:  def.SkillType,
		Debuff:     def.Debuff,
		EffectType: def.EffectType,
	}
}

// newActorEffect builds the actor-hosted tick effect for one of def's
// effect templates, dispatching by the template's core-effect name. It
// returns nil for a template this port doesn't carry a signet kind for.
func (h signetHandler) newActorEffect(def modelskill.Definition, meta effect.Skill, tmpl modelskill.EffectTemplate, actor *npc.EffectPoint) *effect.Effect {
	switch tmpl.Name {
	case "Signet":
		return h.newSignetBuffEffect(def, meta, tmpl, actor)
	case "SignetNoise":
		return h.newSignetNoiseEffect(def, meta, tmpl, actor)
	case "SignetAntiSummon":
		return h.newSignetAntiSummonEffect(def, meta, tmpl, actor)
	default:
		return nil
	}
}

// newSignetBuffEffect builds the effect that, each tick, applies the
// skill's linked effectId sub-skill's own effects onto every living,
// non-peace-zone creature the actor finds within skill radius.
func (h signetHandler) newSignetBuffEffect(def modelskill.Definition, meta effect.Skill, tmpl modelskill.EffectTemplate, actor *npc.EffectPoint) *effect.Effect {
	e := &effect.Effect{Skill: meta, Template: tmpl, Effector: actor, Effected: actor}
	e.OnStart = func(*effect.Effect) bool { return true }
	e.OnAction = func(*effect.Effect) bool {
		sub, ok := h.defs.Definition(modelskill.Ref{ID: modelskill.ID(def.EffectID), Level: def.Level})
		if !ok {
			return true
		}
		var ids []int32
		h.forEachSignetTarget(actor, def.Radius, func(target signetNearby) {
			applyEffects(actor, target, sub, sub.Effects)
			if ct, ok := target.(signetCastTarget); ok {
				actor.BroadcastSkillUse(ct, int32(sub.ID), int32(sub.Level))
				ids = append(ids, ct.ObjectID())
			}
		})
		if len(ids) > 0 {
			actor.BroadcastSkillLaunched(int32(sub.ID), int32(sub.Level), ids)
		}
		return true
	}
	e.OnExit = func(*effect.Effect) { actor.Despawn() }
	return e
}

// newSignetNoiseEffect builds the effect that, after skipping its first
// tick, strips every dance/song effect from every living, non-peace-zone
// creature the actor finds within skill radius on each subsequent tick.
func (h signetHandler) newSignetNoiseEffect(def modelskill.Definition, meta effect.Skill, tmpl modelskill.EffectTemplate, actor *npc.EffectPoint) *effect.Effect {
	e := &effect.Effect{Skill: meta, Template: tmpl, Effector: actor, Effected: actor}
	e.OnStart = func(*effect.Effect) bool { return true }
	e.OnAction = func(ef *effect.Effect) bool {
		if ef.Remaining() == ef.Template.Count-1 {
			return true
		}
		sub, ok := h.defs.Definition(modelskill.Ref{ID: modelskill.ID(def.EffectID), Level: def.Level})
		if !ok {
			return true
		}
		var ids []int32
		h.forEachSignetTarget(actor, def.Radius, func(target signetNearby) {
			if dancer, ok := target.(signetDancer); ok {
				list := dancer.EffectList()
				for _, cand := range list.All() {
					if cand.Skill.Dance {
						list.Remove(cand)
					}
				}
			}
			if ct, ok := target.(signetCastTarget); ok {
				actor.BroadcastSkillUse(ct, int32(sub.ID), int32(sub.Level))
				ids = append(ids, ct.ObjectID())
			}
		})
		if len(ids) > 0 {
			actor.BroadcastSkillLaunched(int32(sub.ID), int32(sub.Level), ids)
		}
		return true
	}
	e.OnExit = func(*effect.Effect) { actor.Despawn() }
	return e
}

// newSignetAntiSummonEffect builds the effect that, after skipping its
// first tick, unsummons every living, non-peace-zone summon the actor
// finds within skill radius on each subsequent tick. Each dismissed
// summon broadcasts its own self-cast MagicSkillUse packet from its own
// known list, matching Java's summon.broadcastPacket(new
// MagicSkillUse(summon, ...)).
func (h signetHandler) newSignetAntiSummonEffect(def modelskill.Definition, meta effect.Skill, tmpl modelskill.EffectTemplate, actor *npc.EffectPoint) *effect.Effect {
	e := &effect.Effect{Skill: meta, Template: tmpl, Effector: actor, Effected: actor}
	e.OnStart = func(*effect.Effect) bool { return true }
	e.OnAction = func(ef *effect.Effect) bool {
		if ef.Remaining() == ef.Template.Count-1 {
			return true
		}
		var ids []int32
		h.forEachSignetTarget(actor, def.Radius, func(target signetNearby) {
			summon, ok := target.(signetUnsummonable)
			if !ok {
				return
			}
			sx, sy, sz := summon.Position()
			self := serverpackets.SkillCastObject{ObjectID: summon.ObjectID(), Location: location.Location{X: sx, Y: sy, Z: sz}}
			summon.BroadcastFrame(serverpackets.FrameMagicSkillUse(self, self, int32(def.ID), int32(def.Level), 0, 0, false))
			ids = append(ids, summon.ObjectID())
			summon.Unsummon()
		})
		if len(ids) > 0 {
			actor.BroadcastSkillLaunched(int32(def.ID), int32(def.Level), ids)
		}
		return true
	}
	e.OnExit = func(*effect.Effect) { actor.Despawn() }
	return e
}

// newSignetMDamEffect builds the effect whose onStart spawns the carrying
// actor at caster's position; each of its first two ticks does nothing,
// and every tick after pays the skill's MP cost (dropping the effect when
// the caster can't afford it) then deals magic damage, using the skill's
// own formula inputs, to every living, non-peace-zone creature the actor
// finds within skill radius.
func (h signetHandler) newSignetMDamEffect(caster any, def modelskill.Definition, meta effect.Skill, tmpl modelskill.EffectTemplate) *effect.Effect {
	e := &effect.Effect{Skill: meta, Template: tmpl, Type: effect.TypeSignetGround, Effector: caster, Effected: caster}
	var actor *npc.EffectPoint
	e.OnStart = func(*effect.Effect) bool {
		a, ok := h.spawnActor(caster, def)
		if !ok {
			return false
		}
		actor = a
		return true
	}
	e.OnAction = func(ef *effect.Effect) bool {
		if actor == nil {
			return false
		}
		if ef.Remaining() >= ef.Template.Count-2 {
			return true
		}

		mp, ok := caster.(mpPayer)
		if !ok {
			return false
		}
		if float64(def.MPConsume) > mp.MPValue() {
			return false
		}
		mp.ReduceMP(float64(def.MPConsume))

		var ids []int32
		h.forEachSignetTarget(actor, def.Radius, func(target signetNearby) {
			dmgTarget, ok := target.(magicDamageTarget)
			if !ok {
				return
			}
			in, ok := dmgTarget.MagicDamageInput(caster, def)
			if !ok {
				return
			}
			damage := int(formulas.MagicDamage(in))
			if damage > 0 {
				dmgTarget.ReduceHP(float64(damage), caster, def)
			}
			if ct, ok := target.(signetCastTarget); ok {
				actor.BroadcastSkillUse(ct, int32(def.ID), int32(def.Level))
				ids = append(ids, ct.ObjectID())
			}
		})
		if len(ids) > 0 {
			actor.BroadcastSkillLaunched(int32(def.ID), int32(def.Level), ids)
		}
		return true
	}
	e.OnExit = func(*effect.Effect) {
		if actor != nil {
			actor.Despawn()
		}
	}
	return e
}

// mpPayer is implemented by a caster whose mana pool can be checked and
// drained, the SignetMDam family's per-tick upkeep cost.
type mpPayer interface {
	MPValue() float64
	ReduceMP(float64) float64
}

// forEachSignetTarget calls fn for every living, non-peace-zone object
// actor's radius scan finds, excluding doors and ground items (neither
// implements Dead()).
func (h signetHandler) forEachSignetTarget(actor *npc.EffectPoint, radius int, fn func(signetNearby)) {
	actor.ForEachNearby(radius, func(o world.Tracked) {
		target, ok := o.(signetNearby)
		if !ok || target.Dead() {
			return
		}
		if pz, ok := o.(signetPeaceZoned); ok && pz.InPeaceZone() {
			return
		}
		fn(target)
	})
}
