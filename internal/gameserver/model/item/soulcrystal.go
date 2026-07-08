package item

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

type SoulCrystal struct {
	Level         int
	InitialItemID int32
	StagedItemID  int32
	BrokenItemID  int32
}

func NewSoulCrystal(set *commons.StatSet) (SoulCrystal, error) {
	initial, err := set.GetInt32("initial")
	if err != nil {
		return SoulCrystal{}, fmt.Errorf("item: soul crystal: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("item: soul crystal %d: %w", initial, err) }

	level, err := set.GetInt("level")
	if err != nil {
		return SoulCrystal{}, wrap(err)
	}
	staged, err := set.GetInt32("staged")
	if err != nil {
		return SoulCrystal{}, wrap(err)
	}
	broken, err := set.GetInt32("broken")
	if err != nil {
		return SoulCrystal{}, wrap(err)
	}
	return SoulCrystal{Level: level, InitialItemID: initial, StagedItemID: staged, BrokenItemID: broken}, nil
}

type SoulCrystalLevelingInfo struct {
	NPCID         int32
	ChanceStage   int
	ChanceBreak   int
	SkillRequired bool
	AbsorbType    string
	Levels        []int
}

func NewSoulCrystalLevelingInfo(set *commons.StatSet) (SoulCrystalLevelingInfo, error) {
	npcID, err := set.GetInt32("id")
	if err != nil {
		return SoulCrystalLevelingInfo{}, fmt.Errorf("item: soul crystal npc info: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("item: soul crystal npc %d: %w", npcID, err) }

	stage, err := set.GetInt("chanceStage")
	if err != nil {
		return SoulCrystalLevelingInfo{}, wrap(err)
	}
	breakChance, err := set.GetInt("chanceBreak")
	if err != nil {
		return SoulCrystalLevelingInfo{}, wrap(err)
	}
	absorbType, err := set.GetString("absorbType")
	if err != nil {
		return SoulCrystalLevelingInfo{}, wrap(err)
	}
	levels, err := set.GetIntArray("levelList")
	if err != nil {
		return SoulCrystalLevelingInfo{}, wrap(err)
	}

	return SoulCrystalLevelingInfo{
		NPCID:         npcID,
		ChanceStage:   stage,
		ChanceBreak:   breakChance,
		SkillRequired: set.GetBoolDefault("skill", false),
		AbsorbType:    absorbType,
		Levels:        levels,
	}, nil
}

type SoulCrystalTable struct {
	crystals map[int32]SoulCrystal
	npcs     map[int32]SoulCrystalLevelingInfo
}

func NewSoulCrystalTable(crystals []SoulCrystal, infos []SoulCrystalLevelingInfo) (*SoulCrystalTable, error) {
	crystalMap := make(map[int32]SoulCrystal, len(crystals))
	for _, crystal := range crystals {
		if _, exists := crystalMap[crystal.InitialItemID]; exists {
			return nil, fmt.Errorf("item: duplicate soul crystal %d", crystal.InitialItemID)
		}
		crystalMap[crystal.InitialItemID] = crystal
	}

	infoMap := make(map[int32]SoulCrystalLevelingInfo, len(infos))
	for _, info := range infos {
		if _, exists := infoMap[info.NPCID]; exists {
			return nil, fmt.Errorf("item: duplicate soul crystal leveling info %d", info.NPCID)
		}
		infoMap[info.NPCID] = info
	}

	return &SoulCrystalTable{crystals: crystalMap, npcs: infoMap}, nil
}

func (t *SoulCrystalTable) Crystal(initialItemID int32) (SoulCrystal, bool) {
	value, ok := t.crystals[initialItemID]
	return value, ok
}

func (t *SoulCrystalTable) LevelingInfo(npcID int32) (SoulCrystalLevelingInfo, bool) {
	value, ok := t.npcs[npcID]
	return value, ok
}

func (t *SoulCrystalTable) CrystalCount() int      { return len(t.crystals) }
func (t *SoulCrystalTable) LevelingInfoCount() int { return len(t.npcs) }
