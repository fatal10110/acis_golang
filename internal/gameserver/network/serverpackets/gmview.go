package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Opcodes of the game master's views of a character.
const (
	OpcodeGMViewCharacterInfo         = 0x8f
	OpcodeGMViewSkillInfo             = 0x91
	OpcodeGMViewQuestList             = 0x93
	OpcodeGMViewWarehouseWithdrawList = 0x95
)

// gmViewBonusSlots is the bonus-slot word GMViewCharacterInfo writes, armed
// or not.
const gmViewBonusSlots = 0x28

// gmViewRunningMark follows the running flag in GMViewCharacterInfo: the
// low byte of 321.
const gmViewRunningMark = 321 & 0xff

// FrameGMViewCharacterInfo builds the game master's status view of the
// character s describes: UserInfo's character, combat and appearance fields
// under the character's own name and title, without the party, cubic,
// fishing and cursed weapon state.
func FrameGMViewCharacterInfo(s UserInfoSnapshot) wire.Frame {
	c, t := s.Character, s.Template
	x, y, z := c.Position()
	resources := c.ResourceValues()
	progression := c.ProgressionValues()
	paperdoll := item.Paperdoll(s.Items)

	collisionRadius, collisionHeight := t.CollisionRadius, t.CollisionHeight
	if c.Sex() != player.SexMale {
		collisionRadius, collisionHeight = t.CollisionRadiusFemale, t.CollisionHeightFemale
	}

	w := newFrameWriter(OpcodeGMViewCharacterInfo)
	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	w.WriteInt32(int32(c.CurrentHeading()))
	w.WriteInt32(c.ObjectID())
	w.WriteString(c.CharacterName())
	w.WriteInt32(int32(c.Race))
	w.WriteInt32(int32(c.Sex()))
	w.WriteInt32(int32(c.ClassID()))
	w.WriteInt32(int32(progression.CharLevel))
	w.WriteInt64(progression.Exp)
	w.WriteInt32(int32(c.STR()))
	w.WriteInt32(int32(c.DEX()))
	w.WriteInt32(int32(c.CON()))
	w.WriteInt32(int32(c.INT()))
	w.WriteInt32(int32(c.WIT()))
	w.WriteInt32(int32(c.MEN()))
	w.WriteInt32(statInt32(resources.MaxHP))
	w.WriteInt32(statInt32(resources.CurrentHP))
	w.WriteInt32(statInt32(resources.MaxMP))
	w.WriteInt32(statInt32(resources.CurrentMP))
	w.WriteInt32(int32(progression.SP))
	w.WriteInt32(clampInt32(c.CurrentWeight()))
	w.WriteInt32(clampInt32(c.WeightLimit()))
	w.WriteInt32(gmViewBonusSlots)
	for _, pos := range paperdollWriteOrder {
		w.WriteInt32(paperdoll[pos].ObjectID)
	}
	for _, pos := range paperdollWriteOrder {
		w.WriteInt32(paperdoll[pos].TemplateID)
	}
	for range 34 {
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
	w.WriteUint8(uint8(c.MountType()))
	w.WriteUint8(uint8(c.OperateType()))
	w.WriteUint8(boolUint8(c.HasSkill(crystallizeSkillID)))
	w.WriteInt32(int32(progression.PKKills))
	w.WriteInt32(int32(progression.PvPKills))
	w.WriteUint16(uint16(c.RecommendationsLeft()))
	w.WriteUint16(uint16(c.RecommendationsHave()))
	w.WriteInt32(int32(c.ClassID()))
	w.WriteInt32(0)
	w.WriteInt32(statInt32(resources.MaxCP))
	w.WriteInt32(statInt32(resources.CurrentCP))
	w.WriteUint8(boolUint8(c.Running()))
	w.WriteUint8(gmViewRunningMark)
	w.WriteInt32(int32(c.PledgeClass()))
	w.WriteUint8(boolUint8(c.IsNoble()))
	w.WriteUint8(boolUint8(c.IsHero()))
	w.WriteInt32(c.NameColor())
	w.WriteInt32(c.TitleColor())
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameGMViewSkillInfo builds the game master's view of player's skills.
func FrameGMViewSkillInfo(player string, skills []SkillListEntry) wire.Frame {
	w := newFrameWriter(OpcodeGMViewSkillInfo)
	w.WriteString(player)
	writeSkillList(w, skills)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameGMViewQuestList builds the game master's view of player's quest log.
func FrameGMViewQuestList(player string, quests []QuestListEntry) wire.Frame {
	count, err := wire.Uint16Count(len(quests))
	if err != nil {
		return wire.InvalidFrame(err)
	}
	w := newFrameWriter(OpcodeGMViewQuestList)
	w.WriteString(player)
	w.WriteUint16(count)
	for _, q := range quests {
		w.WriteInt32(q.QuestID)
		w.WriteInt32(q.Flags)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameGMViewWarehouseWithdrawList builds the game master's view of a
// warehouse: owner names the player, or the leader of the clan, it belongs
// to. An item whose template is unknown is left out.
func FrameGMViewWarehouseWithdrawList(owner string, adena int32, items []*item.Instance, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodeGMViewWarehouseWithdrawList)
	w.WriteString(owner)
	w.WriteInt32(adena)
	countOffset, err := reserveUint16Count(w, len(items))
	if err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	var listed int
	for _, inst := range items {
		if inst == nil {
			continue
		}
		st := inst.Snapshot()
		tmpl, ok := templates.Get(st.TemplateID)
		if !ok {
			continue
		}
		listed++
		category, subCategory := tmpl.Category()
		w.WriteUint16(uint16(category))
		w.WriteInt32(st.ObjectID)
		w.WriteInt32(st.TemplateID)
		w.WriteInt32(int32(st.Count))
		w.WriteUint16(uint16(subCategory))
		w.WriteUint16(uint16(st.CustomType1))
		if tmpl.Equipable() {
			w.WriteInt32(int32(tmpl.Slot))
			w.WriteUint16(uint16(st.EnchantLevel))
			if weapon := tmpl.Weapon; weapon != nil {
				w.WriteUint16(uint16(weapon.SoulshotCount))
				w.WriteUint16(uint16(weapon.SpiritshotCount))
			} else {
				w.WriteUint16(0)
				w.WriteUint16(0)
			}
			if aug := st.Augmentation; aug != nil && tmpl.Weapon != nil {
				w.WriteInt32(aug.Attributes & 0x0000ffff)
				w.WriteInt32(aug.Attributes >> 16)
			} else {
				w.WriteInt64(0)
			}
		}
		w.WriteInt32(0)
	}
	w.PatchUint16(countOffset, uint16(listed))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}
