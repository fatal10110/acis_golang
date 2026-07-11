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
	idf := commons.NewFields(set, "item: soul crystal")
	initial := idf.Int32("initial")
	if err := idf.Err(); err != nil {
		return SoulCrystal{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("item: soul crystal %d", initial))
	crystal := SoulCrystal{
		Level:         f.Int("level"),
		InitialItemID: initial,
		StagedItemID:  f.Int32("staged"),
		BrokenItemID:  f.Int32("broken"),
	}
	if err := f.Err(); err != nil {
		return SoulCrystal{}, err
	}
	return crystal, nil
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
	idf := commons.NewFields(set, "item: soul crystal npc info")
	npcID := idf.Int32("id")
	if err := idf.Err(); err != nil {
		return SoulCrystalLevelingInfo{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("item: soul crystal npc %d", npcID))
	info := SoulCrystalLevelingInfo{
		NPCID:         npcID,
		ChanceStage:   f.Int("chanceStage"),
		ChanceBreak:   f.Int("chanceBreak"),
		SkillRequired: f.BoolDefault("skill", false),
		AbsorbType:    f.String("absorbType"),
		Levels:        f.IntArray("levelList"),
	}
	if err := f.Err(); err != nil {
		return SoulCrystalLevelingInfo{}, err
	}
	return info, nil
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
