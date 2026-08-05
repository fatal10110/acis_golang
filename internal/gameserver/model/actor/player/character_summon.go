package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// SummonSpawner spawns c's pet or servitor in response to a SUMMON_CREATURE
// (or SUMMON servitor-branch) cast, mirroring the reference's
// Pet.restore/World.addPet/Player.setSummon sequence. The domain layer
// depends on this narrow interface instead of the network package, which
// owns world placement, persistence, and AI wiring — the same split
// SetCastController uses for the cast controller.
type SummonSpawner interface {
	// SpawnPet summons owner's pet from controlItem, the pet-collar item
	// instance the SUMMON_CREATURE cast was cast with. It reports whether a
	// pet was actually spawned.
	SpawnPet(owner *Character, controlItem *item.Instance) bool
}

// SetSummonSpawner wires c's live summon spawner, called once by the
// network layer when it creates one for c.
func (c *Character) SetSummonSpawner(spawner SummonSpawner) {
	c.summonSpawnMu.Lock()
	defer c.summonSpawnMu.Unlock()
	c.summonSpawner = spawner
}

func (c *Character) summonSpawnerLocked() SummonSpawner {
	c.summonSpawnMu.RLock()
	defer c.summonSpawnMu.RUnlock()
	return c.summonSpawner
}

// SummonCreature is the SUMMON_CREATURE skill handler's entry point
// (handler/skill/summon.go's creatureSummonRuntime), matching Java's
// SummonCreature.useSkill: only a pet-collar-item cast reaches here, so a
// non-*item.Instance item (or no spawner attached) is a silent no-op, same
// as Java's item==nil / getSummonItem==null early returns.
func (c *Character) SummonCreature(_ modelskill.Definition, itemArg any) {
	spawner := c.summonSpawnerLocked()
	if spawner == nil {
		return
	}
	inst, ok := itemArg.(*item.Instance)
	if !ok {
		return
	}
	spawner.SpawnPet(c, inst)
}
