package script

import (
	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// The quest sounds.
const (
	SoundAccept       = "ItemSound.quest_accept"
	SoundItemGet      = "ItemSound.quest_itemget"
	SoundMiddle       = "ItemSound.quest_middle"
	SoundFinish       = "ItemSound.quest_finish"
	SoundGiveUp       = "ItemSound.quest_giveup"
	SoundJackpot      = "ItemSound.quest_jackpot"
	SoundFanfare      = "ItemSound.quest_fanfare_2"
	SoundBeforeBattle = "Itemsound.quest_before_battle"
	SoundTutorial     = "ItemSound.quest_tutorial"
)

// MaxChance is a drop chance that always succeeds.
const MaxChance = 1000000

// DropType is how a quest drop's chance and amount follow the drop rate.
type DropType uint8

// The drop types.
const (
	// DropDivmod scales the chance by the rate: every whole MaxChance of it
	// gives count, and the remainder is the chance of count more.
	DropDivmod DropType = iota
	// DropFixedRate keeps the chance and scales the amount.
	DropFixedRate
	// DropFixedCount scales the chance and keeps the amount.
	DropFixedCount
	// DropFixedBoth scales neither.
	DropFixedBoth
)

// GiveItems gives p count units of itemID, with no slot or weight check.
// A count below one gives nothing.
func (s *Script) GiveItems(p *Player, itemID, count int32) {
	s.GiveItemsEnchanted(p, itemID, count, 0)
}

// GiveItemsEnchanted is GiveItems with a positive enchant set on the
// instance given last.
func (s *Script) GiveItemsEnchanted(p *Player, itemID, count, enchant int32) {
	if count <= 0 {
		return
	}
	s.give(p.character(), itemID, count, enchant)
}

func (s *Script) give(c *player.Character, itemID, count, enchant int32) {
	if count <= 0 {
		return
	}
	c.GiveScriptItems(itemID, int(count), int(enchant), s.env.MultipleItemDrop, s.env.NewItemID)
}

// TakeItems takes count units of itemID from p; a negative count takes
// them all.
func (s *Script) TakeItems(p *Player, itemID, count int32) {
	p.character().TakeScriptItems(itemID, int(count))
}

// AddItems gives p count units of itemID as GiveItems does, with the chat
// line of items picked up.
func (s *Script) AddItems(p *Player, itemID, count int32) {
	p.character().AddScriptItems(itemID, int(count), s.env.MultipleItemDrop, s.env.NewItemID)
}

// DestroyItems destroys count units of itemID held by p, with no chat line,
// and reports whether it did: p must hold that many in one instance.
func (s *Script) DestroyItems(p *Player, itemID, count int32) bool {
	return p.character().DestroyScriptItems(itemID, int(count))
}

// RewardItems gives p count units of itemID scaled by the reward rate, the
// adena rate for adena.
func (s *Script) RewardItems(p *Player, itemID, count int32) {
	rate := s.env.Rates.Reward
	if itemID == item.AdenaID {
		rate = s.env.Rates.RewardAdena
	}
	s.GiveItems(p, itemID, commons.JavaInt(float64(count)*rate))
}

// RewardExpAndSp gives p exp experience and sp skill points, each scaled
// by its reward rate.
func (s *Script) RewardExpAndSp(p *Player, exp int64, sp int32) {
	p.character().AddExpAndSp(commons.JavaLong(float64(exp)*s.env.Rates.XP), int(commons.JavaInt(float64(sp)*s.env.Rates.SP)))
}

// PlaySound plays sound for p alone.
func (s *Script) PlaySound(p *Player, sound string) {
	p.character().NotifySound(sound)
}

// DropItemsAlways gives p count units of itemID scaled by the drop rate,
// toward needed, a drop of DropFixedRate that cannot miss.
func (s *Script) DropItemsAlways(p *Player, itemID, count, needed int32) bool {
	return s.DropItemsAs(p, itemID, count, needed, MaxChance, DropFixedRate)
}

// DropItems is DropItemsAs with DropDivmod.
func (s *Script) DropItems(p *Player, itemID, count, needed, chance int32) bool {
	return s.DropItemsAs(p, itemID, count, needed, chance, DropDivmod)
}

// DropItemsAs rolls a quest drop of count units of itemID for p, at chance
// out of MaxChance, as typ scales it. With a positive needed the drop
// stops at needed units held, and it reports whether p holds them; a drop
// that would not fit p's inventory slots gives nothing. Each drop that gives
// plays the item-get sound, or the middle sound when it completes needed.
// A zero needed collects without limit and reports false.
func (s *Script) DropItemsAs(p *Player, itemID, count, needed, chance int32, typ DropType) bool {
	c := p.character()
	current := int32(c.ItemCount(int(itemID)))
	if needed > 0 && current >= needed {
		return true
	}
	amount := s.dropAmount(count, chance, typ)
	reached := false
	if amount > 0 {
		if needed > 0 {
			reached = current+amount >= needed
			if reached {
				amount = needed - current
			}
		}
		if !c.RewardItemFits(itemID, int(amount)) {
			return false
		}
		s.give(c, itemID, amount, 0)
		if reached {
			c.NotifySound(SoundMiddle)
		} else {
			c.NotifySound(SoundItemGet)
		}
	}
	return needed > 0 && reached
}

// DropInfo is one item of DropMultipleItems: Count units of ItemID at
// Chance out of MaxChance, toward Needed.
type DropInfo struct {
	ItemID, Count, Needed, Chance int32
}

// DropMultipleItems rolls one drop per item of drops, as DropItemsAs does,
// skipping each item p already holds its needed units of. One sound plays
// at the end when anything was given: the middle sound when every rolled
// item reached its needed units, else the item-get sound. It reports
// whether every rolled item holds its needed units now; an item with no
// needed units never does.
func (s *Script) DropMultipleItems(p *Player, drops []DropInfo, typ DropType) bool {
	c := p.character()
	given, reached := false, true
	for _, d := range drops {
		current := int32(c.ItemCount(int(d.ItemID)))
		if d.Needed > 0 && current >= d.Needed {
			continue
		}
		amount := s.dropAmount(d.Count, d.Chance, typ)
		if amount > 0 {
			if d.Needed > 0 && current+amount >= d.Needed {
				amount = d.Needed - current
			}
			if !c.RewardItemFits(d.ItemID, int(amount)) {
				continue
			}
			s.give(c, d.ItemID, amount, 0)
			given = true
		}
		if d.Needed <= 0 || current+amount < d.Needed {
			reached = false
		}
	}
	if given {
		if reached {
			c.NotifySound(SoundMiddle)
		} else {
			c.NotifySound(SoundItemGet)
		}
	}
	return reached
}

// dropAmount rolls one drop of count units at chance as typ scales it and
// returns the units it gives. Every type draws once; an unknown type
// draws nothing and gives nothing.
func (s *Script) dropAmount(count, chance int32, typ DropType) int32 {
	rate := s.env.Rates.Drop
	switch typ {
	case DropDivmod:
		chance = commons.JavaInt(float64(chance) * rate)
		amount := count * (chance / MaxChance)
		if int32(s.env.Rand(MaxChance)) < chance%MaxChance {
			amount += count
		}
		return amount
	case DropFixedRate:
		if int32(s.env.Rand(MaxChance)) < chance {
			return commons.JavaInt(float64(count) * rate)
		}
	case DropFixedCount:
		if float64(s.env.Rand(MaxChance)) < float64(chance)*rate {
			return count
		}
	case DropFixedBoth:
		if int32(s.env.Rand(MaxChance)) < chance {
			return count
		}
	}
	return 0
}
