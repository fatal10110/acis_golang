package script

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// ClassID returns the id of the class the player plays.
func (p *Player) ClassID() int32 { return int32(p.character().ClassID()) }

// IsMageClass reports whether the class the player plays is a mystic one:
// a mystic starting class or one of its upgrades.
func (p *Player) IsMageClass() bool { return player.ClassMage(p.character().ClassID()) }

// Race returns the player's race.
func (p *Player) Race() player.Race { return p.character().Race }

// HasItem reports whether the player's inventory holds any instance of
// itemID.
func (p *Player) HasItem(itemID int32) bool {
	inv := p.character().Inventory()
	return inv != nil && inv.HasItem(itemID)
}

// GetInt returns the state's variable key as a number, 0 when it is not
// set. A value that is not a 32-bit integer panics, so the invocation
// aborts there.
func (qs *QuestState) GetInt(key string) int32 {
	v, ok := qs.state.Get(key)
	if !ok {
		return 0
	}
	n, err := commons.ParseInt(v, 32)
	if err != nil {
		panic(fmt.Sprintf("script: quest %s variable %s %q: %v", qs.state.Quest().Name, key, v, err))
	}
	return int32(n)
}
