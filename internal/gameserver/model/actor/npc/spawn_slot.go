package npc

import "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"

// SpawnSlot is the spawn that placed an NPC, as the NPC sees it: the
// spawn's own AI parameters and the script memory the spawn keeps across
// the NPC's lives.
type SpawnSlot interface {
	// AIParams returns the spawn entry's <ai> values. They shadow the
	// template's.
	AIParams() AIParams
	// Scratch returns the script memory of the slot's NPC. It is the same
	// value for every NPC the slot spawns.
	Scratch() *Scratch
	// Row returns the persisted row of a database-tracked spawn; nil for
	// a spawn with none.
	Row() SpawnRow
}

// SpawnRow is the spawn_data row of a database-tracked spawn, as its NPC's
// scripts see it.
type SpawnRow interface {
	// DBValue returns the script value the row keeps; SetDBValue stores
	// one, saved with the row.
	DBValue() int
	SetDBValue(v int)
	// DBLoaded reports whether the spawn's NPC was placed from a live row:
	// its saved HP, MP and position.
	DBLoaded() bool
}

// spawnBinding is what a live NPC keeps of its spawn slot. It is set
// before the NPC is published and read-only after.
type spawnBinding struct {
	// template is the NPC's own template, the same pointer as
	// Instance.Template; both are set once at construction.
	template *Template
	spawn    AIParams
	scratch  *Scratch
	row      SpawnRow
}

// newSpawnBinding binds an NPC of tmpl to no spawn: no spawn parameters
// and a script memory of its own.
func newSpawnBinding(tmpl *Template) spawnBinding {
	return spawnBinding{template: tmpl, scratch: NewScratch()}
}

// bindSpawn binds the NPC to slot; a nil slot keeps the current binding.
func (b *spawnBinding) bindSpawn(slot SpawnSlot) {
	if slot == nil {
		return
	}
	b.spawn = slot.AIParams()
	b.row = slot.Row()
	if s := slot.Scratch(); s != nil {
		b.scratch = s
	}
}

func (b *spawnBinding) templateParams() AIParams {
	if b.template == nil {
		return nil
	}
	return b.template.AIParams
}

// AIInt returns the int AI parameter name: the spawn's value when it has
// one, else the template's, else def. A value that does not read as an
// int, in either place, gives def.
func (b *spawnBinding) AIInt(name string, def int32) int32 {
	return resolveAIInt(b.spawn, b.templateParams(), name, def)
}

// AIString returns the AI parameter name: the spawn's value when it has
// one, else the template's, else def.
func (b *spawnBinding) AIString(name, def string) string {
	return resolveAIString(b.spawn, b.templateParams(), name, def)
}

// AISkill returns the AI parameter name, looked up as AIString does, read
// as an "id-level" skill reference. ok is false when neither place has
// it; err reports a value that is not one.
func (b *spawnBinding) AISkill(name string) (ref skill.Ref, ok bool, err error) {
	return resolveAISkill(b.spawn, b.templateParams(), name)
}

// Scratch returns the NPC's script memory, its spawn slot's when it has
// one.
func (b *spawnBinding) Scratch() *Scratch {
	return b.scratch
}

// SpawnRow returns the persisted row of the NPC's spawn; nil when its spawn
// keeps none.
func (b *spawnBinding) SpawnRow() SpawnRow {
	return b.row
}
