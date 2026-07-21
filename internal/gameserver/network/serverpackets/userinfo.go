package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// OpcodeUserInfo is the wire opcode for UserInfo, the full self-status
// packet sent on world entry and after any change to a character's own
// visible state.
const OpcodeUserInfo = 0x04

// defaultNameColor and defaultTitleColor are the shipped defaults for a
// character with no name/title color override — nothing in this server sets
// one yet.
const (
	defaultNameColor  = 0xFFFFFF
	defaultTitleColor = 0xFFFF77
)

// dwarfInventoryLimit and nonDwarfInventoryLimit are the shipped default
// inventory slot counts by race; nothing here scales them with equipment or
// skills yet.
const (
	nonDwarfInventoryLimit = 80
	dwarfInventoryLimit    = 100
)

// weaponEquippedBonusSlots and noWeaponBonusSlots are the two values the
// client's per-character bonus-slot field takes, gated on whether a weapon
// is equipped. The client-side meaning of "bonus slots" here (commonly
// documented elsewhere as a talisman-slot count) isn't stated anywhere in
// this behavior's own source — only the 20/40 branch on weapon presence is.
const (
	noWeaponBonusSlots       = 20
	weaponEquippedBonusSlots = 40
)

// UserInfoSnapshot is everything UserInfo needs about one character at the
// moment of encoding. It is deliberately narrower than the client's full
// field list: systems this server hasn't built yet (clans, cubics,
// hero/noble status, mounts, fishing, recommendations, and the
// formula-derived combat stats — attack/cast speed, evasion, accuracy,
// critical rate) always report their at-rest default, matching a freshly
// entered character that has none of them. Base P.Atk/P.Def/M.Atk/M.Def come
// straight from the profession template with no gear or skill bonus applied.
type UserInfoSnapshot struct {
	Character *player.Character
	Template  *player.Template
	Items     []*item.Instance
}

// FrameUserInfo builds the UserInfo packet for s as an owned frame.
func FrameUserInfo(s UserInfoSnapshot) wire.Frame {
	w := newFrameWriter(OpcodeUserInfo)
	writeUserInfo(w, s)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeUserInfo(w *wire.Writer, s UserInfoSnapshot) {
	c, t := s.Character, s.Template
	x, y, z := c.Position()
	resources := c.ResourceValues()
	paperdoll := item.Paperdoll(s.Items)
	rhand := paperdoll[rhandPaperdollIndex]

	bonusSlots := int32(noWeaponBonusSlots)
	if rhand.ObjectID != 0 {
		bonusSlots = weaponEquippedBonusSlots
	}

	collisionRadius, collisionHeight := t.CollisionRadius, t.CollisionHeight
	if c.Sex == player.SexFemale {
		collisionRadius, collisionHeight = t.CollisionRadiusFemale, t.CollisionHeightFemale
	}

	inventoryLimit := nonDwarfInventoryLimit
	if c.Race == player.RaceDwarf {
		inventoryLimit = dwarfInventoryLimit
	}

	enchantEffect := rhand.EnchantLevel
	if enchantEffect > maxDisplayedEnchant {
		enchantEffect = maxDisplayedEnchant
	}

	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	w.WriteInt32(int32(c.CurrentHeading()))
	w.WriteInt32(c.ObjectID())
	w.WriteString(c.Name)
	w.WriteInt32(int32(c.Race))
	w.WriteInt32(int32(c.Sex))
	w.WriteInt32(int32(c.ClassID))
	w.WriteInt32(int32(c.CharLevel))
	w.WriteInt64(c.Exp)
	w.WriteInt32(int32(t.STR))
	w.WriteInt32(int32(t.DEX))
	w.WriteInt32(int32(t.CON))
	w.WriteInt32(int32(t.INT))
	w.WriteInt32(int32(t.WIT))
	w.WriteInt32(int32(t.MEN))
	w.WriteInt32(int32(resources.MaxHP))
	w.WriteInt32(int32(resources.CurrentHP))
	w.WriteInt32(int32(resources.MaxMP))
	w.WriteInt32(int32(resources.CurrentMP))
	w.WriteInt32(int32(c.SP))
	w.WriteInt32(0) // current weight: encumbrance is not modeled
	w.WriteInt32(0) // weight limit: encumbrance is not modeled
	w.WriteInt32(bonusSlots)

	for _, pos := range paperdollWriteOrder {
		w.WriteInt32(paperdoll[pos].ObjectID)
	}
	for _, pos := range paperdollWriteOrder {
		w.WriteInt32(paperdoll[pos].TemplateID)
	}

	// Per-slot augmentation option pairs: always empty except the two
	// weapon-hand slots, since only a weapon can be augmented and
	// augmentation is not modeled here.
	for i := 0; i < 14; i++ {
		w.WriteUint16(0)
	}
	w.WriteInt32(0) // right-hand augmentation id
	for i := 0; i < 12; i++ {
		w.WriteUint16(0)
	}
	w.WriteInt32(0) // left-hand augmentation id
	for i := 0; i < 4; i++ {
		w.WriteUint16(0)
	}

	w.WriteInt32(int32(t.PAtk))
	w.WriteInt32(0) // P.Atk speed: combat-formula stats are not modeled
	w.WriteInt32(int32(t.PDef))
	w.WriteInt32(0) // evasion: combat-formula stats are not modeled
	w.WriteInt32(0) // accuracy: combat-formula stats are not modeled
	w.WriteInt32(0) // critical rate: combat-formula stats are not modeled
	w.WriteInt32(int32(t.MAtk))
	w.WriteInt32(0) // M.Atk speed: combat-formula stats are not modeled
	w.WriteInt32(0) // P.Atk speed (repeated field): combat-formula stats are not modeled
	w.WriteInt32(int32(t.MDef))
	w.WriteInt32(0) // pvp flag: not in combat on world entry
	w.WriteInt32(int32(c.Karma()))

	runSpd := int32(t.RunSpeed)
	walkSpd := int32(t.WalkSpeed)
	swimSpd := int32(t.SwimSpeed)
	w.WriteInt32(runSpd)
	w.WriteInt32(walkSpd)
	w.WriteInt32(swimSpd)
	w.WriteInt32(swimSpd)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0) // flying run speed: flight is not modeled
	w.WriteInt32(0) // flying walk speed: flight is not modeled

	w.WriteFloat64(1) // movement speed multiplier: no active haste/slow effect
	w.WriteFloat64(1) // attack speed multiplier: no active haste/slow effect
	w.WriteFloat64(collisionRadius)
	w.WriteFloat64(collisionHeight)

	w.WriteInt32(int32(c.HairStyle))
	w.WriteInt32(int32(c.HairColor))
	w.WriteInt32(int32(c.Face))
	w.WriteInt32(boolInt32(c.AccessLevel > 0))

	w.WriteString(c.Title)

	w.WriteInt32(int32(c.ClanID))
	w.WriteInt32(0) // clan crest id: clans are not modeled
	w.WriteInt32(0) // ally id: clans are not modeled
	w.WriteInt32(0) // ally crest id: clans are not modeled
	w.WriteInt32(0) // relation flags: clan leadership/siege state is not modeled
	w.WriteUint8(0) // mount type: mounts are not modeled
	w.WriteUint8(0) // operate type: shops/crafting are not modeled
	w.WriteUint8(0) // crystallize flag: not modeled

	w.WriteInt32(int32(c.PKKills))
	w.WriteInt32(int32(c.PvPKills))

	w.WriteUint16(0) // cubic count: cubics are not modeled

	w.WriteUint8(0) // in party-match room: party matching is not modeled
	w.WriteInt32(0) // abnormal effect mask: status effects are not modeled
	w.WriteUint8(0)
	w.WriteInt32(0)  // clan privileges: clans are not modeled
	w.WriteUint16(0) // recommendations left: recommendations are not modeled
	w.WriteUint16(0) // recommendations received: recommendations are not modeled
	w.WriteInt32(0)  // mount npc id: mounts are not modeled
	w.WriteUint16(uint16(inventoryLimit))
	w.WriteInt32(int32(c.ClassID))
	w.WriteInt32(0)
	w.WriteInt32(int32(resources.MaxCP))
	w.WriteInt32(int32(resources.CurrentCP))
	w.WriteUint8(byte(enchantEffect))
	w.WriteUint8(0) // team: teams (duel/event) are not modeled
	w.WriteInt32(0) // large clan crest id: clans are not modeled
	w.WriteUint8(0) // noble flag: nobility is not modeled
	w.WriteUint8(0) // hero flag: heroism is not modeled
	w.WriteUint8(0) // fishing flag: fishing is not modeled
	w.WriteInt32(0) // fishing stance x: fishing is not modeled
	w.WriteInt32(0) // fishing stance y: fishing is not modeled
	w.WriteInt32(0) // fishing stance z: fishing is not modeled
	w.WriteInt32(defaultNameColor)
	w.WriteUint8(boolUint8(c.Running()))
	w.WriteInt32(0) // pledge class: clans are not modeled
	w.WriteInt32(0) // pledge type: clans are not modeled
	w.WriteInt32(defaultTitleColor)
	w.WriteInt32(0) // cursed weapon stage: cursed weapons are not modeled
}
