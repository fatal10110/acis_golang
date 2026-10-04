package serverpackets

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func clampInt32(value int) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	if value < math.MinInt32 {
		return math.MinInt32
	}
	return int32(value)
}

// statInt32 narrows a live stat value to its wire int the way the client
// status fields expect: truncated toward zero, saturating at the int32 range.
func statInt32(value float64) int32 {
	return clampInt32(int(value))
}

// OpcodeUserInfo is the wire opcode for UserInfo, the full self-status
// packet sent on world entry and after any change to a character's own
// visible state.
const OpcodeUserInfo = 0x04

// weaponEquippedBonusSlots and noWeaponBonusSlots are the two values the
// client's per-character bonus-slot field takes, gated on whether a weapon
// is equipped. The client-side meaning of "bonus slots" here (commonly
// documented elsewhere as a talisman-slot count) isn't stated anywhere in
// this behavior's own source — only the 20/40 branch on weapon presence is.
const (
	noWeaponBonusSlots       = 20
	weaponEquippedBonusSlots = 40
)

// mountNpcIdOffset is added to a mounted character's mount npc id so the
// client can distinguish the mount field's id space from regular npc ids;
// an unmounted character reports 0.
const mountNpcIdOffset = 1000000

// teamBlue is TeamType.BLUE's wire id, the team byte shown while spawn
// protection holds.
const teamBlue = 1

// UserInfoSnapshot is everything UserInfo needs about one character at the
// moment of encoding. The
// attributes and combat stats are Character's live values, so gear, buffs,
// level and passives all reach the status window.
type UserInfoSnapshot struct {
	Character *player.Character
	Template  *player.Template
	Items     []*item.Instance
	// IsGM is the accessLevels.xml isGM flag for Character's access level,
	// not merely AccessLevel > 0.
	IsGM bool
	// SpawnProtectedTeam reports the team byte the client sees while spawn
	// protection is active: TeamType.BLUE when spawn protection is enabled
	// and currently held, the character's duel team otherwise.
	SpawnProtectedTeam bool
	// Clan is Character's clan as the status window shows it; zero when
	// clanless.
	Clan ClanFields
}

// ClanFields is the clan part of a player's UserInfo and CharInfo.
type ClanFields struct {
	CrestID      int32
	CrestLargeID int32
	AllyID       int32
	AllyCrestID  int32
	// Leader sets the clan-leader relation bit in UserInfo.
	Leader bool
	// Privileges is the member's privilege mask, UserInfo only.
	Privileges int32
	PledgeType int32
}

// clanLeaderRelation is UserInfo's relation bit for a clan leader.
const clanLeaderRelation = 0x40

// EncodeUserInfo builds the UserInfo packet payload for an unframed send,
// or nil when s cannot be encoded.
func EncodeUserInfo(s UserInfoSnapshot) []byte {
	w := wire.NewPacketWriter(OpcodeUserInfo)
	if err := writeUserInfo(w, s); err != nil {
		return nil
	}
	return w.Bytes()
}

// FrameUserInfo builds the UserInfo packet for s as an owned frame.
func FrameUserInfo(s UserInfoSnapshot) wire.Frame {
	w := newFrameWriter(OpcodeUserInfo)
	if err := writeUserInfo(w, s); err != nil {
		releaseFrameWriter(w)
		return wire.InvalidFrame(err)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeUserInfo(w *wire.Writer, s UserInfoSnapshot) error {
	c, t := s.Character, s.Template
	x, y, z := c.Position()
	resources := c.ResourceValues()
	progression := c.ProgressionValues()
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

	inventoryLimit := c.InventoryLimit()

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
	w.WriteInt32(int32(c.VisibleBaseClassID()))
	w.WriteInt32(int32(progression.CharLevel))
	w.WriteInt64(progression.Exp)
	w.WriteInt32(int32(c.STR()))
	w.WriteInt32(int32(c.DEX()))
	w.WriteInt32(int32(c.CON()))
	w.WriteInt32(int32(c.INT()))
	w.WriteInt32(int32(c.WIT()))
	w.WriteInt32(int32(c.MEN()))
	w.WriteInt32(int32(resources.MaxHP))
	w.WriteInt32(int32(resources.CurrentHP))
	w.WriteInt32(int32(resources.MaxMP))
	w.WriteInt32(int32(resources.CurrentMP))
	w.WriteInt32(int32(progression.SP))
	w.WriteInt32(clampInt32(c.CurrentWeight()))
	w.WriteInt32(clampInt32(c.WeightLimit()))
	w.WriteInt32(bonusSlots)

	for _, pos := range paperdollWriteOrder {
		w.WriteInt32(paperdoll[pos].ObjectID)
	}
	for _, pos := range paperdollWriteOrder {
		w.WriteInt32(paperdoll[pos].TemplateID)
	}

	// Per-slot augmentation option pairs: always empty except the two
	// weapon-hand slots, since only a weapon can be augmented.
	for i := 0; i < 14; i++ {
		w.WriteUint16(0)
	}
	w.WriteInt32(paperdoll[rhandPaperdollIndex].AugmentationID)
	for i := 0; i < 12; i++ {
		w.WriteUint16(0)
	}
	w.WriteInt32(paperdoll[lhandPaperdollIndex].AugmentationID)
	for i := 0; i < 4; i++ {
		w.WriteUint16(0)
	}

	attackSpeed := int32(c.AttackSpeed())
	w.WriteInt32(statInt32(c.PAtk()))
	w.WriteInt32(attackSpeed)
	w.WriteInt32(statInt32(c.PDef()))
	w.WriteInt32(int32(c.Evasion()))
	w.WriteInt32(int32(c.Accuracy()))
	w.WriteInt32(int32(c.CriticalRate()))
	w.WriteInt32(statInt32(c.MAtk()))
	w.WriteInt32(int32(c.MagicAttackSpeed()))
	w.WriteInt32(attackSpeed)
	w.WriteInt32(statInt32(c.MDef()))
	w.WriteInt32(int32(c.PvPFlagState()))
	w.WriteInt32(int32(progression.Karma))

	runSpd := int32(c.BaseRunSpeed())
	walkSpd := int32(c.BaseWalkSpeed())
	swimSpd := int32(c.BaseSwimSpeed())
	w.WriteInt32(runSpd)
	w.WriteInt32(walkSpd)
	w.WriteInt32(swimSpd)
	w.WriteInt32(swimSpd)
	w.WriteInt32(0)
	w.WriteInt32(0)
	if c.Flying() {
		w.WriteInt32(runSpd)
		w.WriteInt32(walkSpd)
	} else {
		w.WriteInt32(0)
		w.WriteInt32(0)
	}

	w.WriteFloat64(float64(c.MovementSpeedMultiplier()))
	w.WriteFloat64(float64(c.AttackSpeedMultiplier()))
	w.WriteFloat64(collisionRadius)
	w.WriteFloat64(collisionHeight)

	w.WriteInt32(int32(c.HairStyle))
	w.WriteInt32(int32(c.HairColor))
	w.WriteInt32(int32(c.Face))
	w.WriteInt32(boolInt32(s.IsGM))

	w.WriteString(c.Title())

	w.WriteInt32(c.ClanID())
	w.WriteInt32(s.Clan.CrestID)
	w.WriteInt32(s.Clan.AllyID)
	w.WriteInt32(s.Clan.AllyCrestID)
	// The siege-state relation bits join the leader's once sieges exist
	// (#3150).
	if s.Clan.Leader {
		w.WriteInt32(clanLeaderRelation)
	} else {
		w.WriteInt32(0)
	}
	w.WriteUint8(uint8(c.MountType()))
	w.WriteUint8(uint8(c.OperateType()))
	w.WriteUint8(0) // crystallize flag: not modeled

	w.WriteInt32(int32(progression.PKKills))
	w.WriteInt32(int32(progression.PvPKills))

	cubicIDs := c.CubicIDs()
	count, err := wire.Uint16Count(len(cubicIDs))
	if err != nil {
		return err
	}
	w.WriteUint16(count)
	for _, id := range cubicIDs {
		w.WriteUint16(uint16(id))
	}

	w.WriteUint8(boolUint8(c.PartyRoom() > 0))
	abnormal := c.AbnormalEffect()
	if s.IsGM && c.Invisible() {
		// An invisible game master sees itself drawn in stealth.
		abnormal |= skill.AbnormalStealth
	}
	w.WriteInt32(int32(abnormal))
	w.WriteUint8(0)
	w.WriteInt32(s.Clan.Privileges)
	w.WriteUint16(uint16(c.RecommendationsLeft()))
	w.WriteUint16(uint16(c.RecommendationsHave()))
	if mountID := c.MountNPCID(); mountID > 0 {
		w.WriteInt32(mountID + mountNpcIdOffset)
	} else {
		w.WriteInt32(0)
	}
	w.WriteUint16(uint16(inventoryLimit))
	w.WriteInt32(int32(c.ClassID()))
	w.WriteInt32(0)
	w.WriteInt32(int32(resources.MaxCP))
	w.WriteInt32(int32(resources.CurrentCP))
	w.WriteUint8(byte(enchantEffect))
	if s.SpawnProtectedTeam {
		w.WriteUint8(teamBlue)
	} else {
		w.WriteUint8(uint8(c.DuelTeam()))
	}
	w.WriteInt32(s.Clan.CrestLargeID)
	w.WriteUint8(boolUint8(c.IsNoble()))
	w.WriteUint8(boolUint8(c.IsHero()))
	bait := c.FishingBait()
	w.WriteUint8(boolUint8(c.Fishing()))
	w.WriteInt32(int32(bait.X))
	w.WriteInt32(int32(bait.Y))
	w.WriteInt32(int32(bait.Z))
	w.WriteInt32(c.NameColor())
	w.WriteUint8(boolUint8(c.Running()))
	w.WriteInt32(int32(c.PledgeClass()))
	w.WriteInt32(s.Clan.PledgeType)
	w.WriteInt32(c.TitleColor())
	w.WriteInt32(c.CursedWeaponStage())
	return nil
}
