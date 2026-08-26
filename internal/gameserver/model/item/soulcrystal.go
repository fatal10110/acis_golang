package item

import "fmt"

type SoulCrystal struct {
	Level         int
	InitialItemID int32
	StagedItemID  int32
	BrokenItemID  int32
}

type SoulCrystalLevelingInfo struct {
	NPCID         int32
	ChanceStage   int
	ChanceBreak   int
	SkillRequired bool
	AbsorbType    string
	Levels        []int
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
