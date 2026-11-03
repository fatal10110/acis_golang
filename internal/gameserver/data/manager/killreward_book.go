package manager

import (
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// KillRewardSettings are the properties-file settings a kill's rewards
// read, with the party range and the multiple item drop rule every other
// reader of the two shares: party loot, the scripts and a character's own
// item creation.
type KillRewardSettings struct {
	Rates         item.Rates
	AutoLoot      bool
	AutoLootRaid  bool
	AutoLootHerbs bool
	// MultipleItemDrop drops or gives a non-stackable item counted above
	// one as that many instances; false makes it just one.
	MultipleItemDrop  bool
	DeepBlueDropRules bool
	// PartyRange is how near a party member must stand to share a reward.
	PartyRange int
	// RateXP and RateSP scale every kill's exp and sp reward before it is
	// split between the attackers.
	RateXP, RateSP float64
	// PartyXP are the rules a party shares a kill's exp and sp by.
	PartyXP player.PartyXPRules
}

// KillRewardBook holds the KillRewardSettings in force. //reload config
// puts new ones in force whole, so a reader that takes them once per
// action sees one set from before or after, never a mix.
type KillRewardBook struct {
	settings atomic.Pointer[KillRewardSettings]
}

// NewKillRewardBook returns a KillRewardBook holding s.
func NewKillRewardBook(s KillRewardSettings) *KillRewardBook {
	b := &KillRewardBook{}
	b.Set(s)
	return b
}

// Set puts s in force in place of the settings before.
func (b *KillRewardBook) Set(s KillRewardSettings) {
	b.settings.Store(&s)
}

// noKillRewardSettings is what a nil book reads.
var noKillRewardSettings = &KillRewardSettings{}

// Settings returns the settings in force, never to be changed; a nil book
// reads the zero settings.
func (b *KillRewardBook) Settings() *KillRewardSettings {
	if b == nil {
		return noKillRewardSettings
	}
	return b.settings.Load()
}

// PartyRange is the party range in force.
func (b *KillRewardBook) PartyRange() int { return b.Settings().PartyRange }

// MultipleItemDrop reports whether the multiple item drop rule is in force.
func (b *KillRewardBook) MultipleItemDrop() bool { return b.Settings().MultipleItemDrop }
