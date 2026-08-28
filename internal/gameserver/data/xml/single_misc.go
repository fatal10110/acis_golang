package xml

import (
	"fmt"
	"path/filepath"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/observer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
)

type soulCrystalFile struct {
	Crystals []soulCrystalElement `xml:"crystals>crystal"`
	NPCs     []soulCrystalNPCElement `xml:"npcs>npc"`
}

// soulCrystalElement is one <crystal> element. initial, staged and broken
// are required int32 ids; level is a required int.
type soulCrystalElement struct {
	Level   *coord   `xml:"level,attr"`
	Initial *coord32 `xml:"initial,attr"`
	Staged  *coord32 `xml:"staged,attr"`
	Broken  *coord32 `xml:"broken,attr"`
}

func (e soulCrystalElement) build() (item.SoulCrystal, error) {
	if e.Initial == nil {
		return item.SoulCrystal{}, fmt.Errorf("soul crystal: initial is required")
	}
	if e.Level == nil {
		return item.SoulCrystal{}, fmt.Errorf("soul crystal %d: level is required", *e.Initial)
	}
	if e.Staged == nil {
		return item.SoulCrystal{}, fmt.Errorf("soul crystal %d: staged is required", *e.Initial)
	}
	if e.Broken == nil {
		return item.SoulCrystal{}, fmt.Errorf("soul crystal %d: broken is required", *e.Initial)
	}
	return item.SoulCrystal{
		Level:         int(*e.Level),
		InitialItemID: int32(*e.Initial),
		StagedItemID:  int32(*e.Staged),
		BrokenItemID:  int32(*e.Broken),
	}, nil
}

// soulCrystalNPCElement is one leveling-info <npc> element. id, chanceStage,
// chanceBreak, absorbType and levelList are all required; skill defaults to
// false.
type soulCrystalNPCElement struct {
	ID          *coord32     `xml:"id,attr"`
	ChanceStage *coord       `xml:"chanceStage,attr"`
	ChanceBreak *coord       `xml:"chanceBreak,attr"`
	Skill       boolAttr     `xml:"skill,attr"`
	AbsorbType  *string      `xml:"absorbType,attr"`
	LevelList   *intListAttr `xml:"levelList,attr"`
}

func (e soulCrystalNPCElement) build() (item.SoulCrystalLevelingInfo, error) {
	if e.ID == nil {
		return item.SoulCrystalLevelingInfo{}, fmt.Errorf("soul crystal npc: id is required")
	}
	if e.ChanceStage == nil {
		return item.SoulCrystalLevelingInfo{}, fmt.Errorf("soul crystal npc %d: chanceStage is required", *e.ID)
	}
	if e.ChanceBreak == nil {
		return item.SoulCrystalLevelingInfo{}, fmt.Errorf("soul crystal npc %d: chanceBreak is required", *e.ID)
	}
	if e.AbsorbType == nil {
		return item.SoulCrystalLevelingInfo{}, fmt.Errorf("soul crystal npc %d: absorbType is required", *e.ID)
	}
	if e.LevelList == nil {
		return item.SoulCrystalLevelingInfo{}, fmt.Errorf("soul crystal npc %d: levelList is required", *e.ID)
	}
	return item.SoulCrystalLevelingInfo{
		NPCID:         int32(*e.ID),
		ChanceStage:   int(*e.ChanceStage),
		ChanceBreak:   int(*e.ChanceBreak),
		SkillRequired: bool(e.Skill),
		AbsorbType:    *e.AbsorbType,
		Levels:        []int(*e.LevelList),
	}, nil
}

func LoadSoulCrystalData(path string) (*item.SoulCrystalTable, error) {
	var doc soulCrystalFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("soul crystals: %w", err)
	}

	crystals := make([]item.SoulCrystal, 0, len(doc.Crystals))
	for _, el := range doc.Crystals {
		crystal, err := el.build()
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		crystals = append(crystals, crystal)
	}

	infos := make([]item.SoulCrystalLevelingInfo, 0, len(doc.NPCs))
	for _, el := range doc.NPCs {
		info, err := el.build()
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		infos = append(infos, info)
	}

	table, err := item.NewSoulCrystalTable(crystals, infos)
	if err != nil {
		return nil, fmt.Errorf("xml: %s: %w", path, err)
	}
	return table, nil
}

// spellbookElement is one <book> element. skillId and itemId are both
// required.
type spellbookElement struct {
	SkillID *coord32 `xml:"skillId,attr"`
	ItemID  *coord32 `xml:"itemId,attr"`
}

func (e spellbookElement) build() (skill.Spellbook, error) {
	if e.SkillID == nil {
		return skill.Spellbook{}, fmt.Errorf("spellbook: skillId is required")
	}
	if e.ItemID == nil {
		return skill.Spellbook{}, fmt.Errorf("spellbook %d: itemId is required", *e.SkillID)
	}
	return skill.NewSpellbook(int32(*e.SkillID), int32(*e.ItemID)), nil
}

type spellbookFile struct {
	Books []spellbookElement `xml:"book"`
}

func LoadSpellbooks(path string) (*skill.SpellbookTable, error) {
	var doc spellbookFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("spellbooks: %w", err)
	}

	books := make([]skill.Spellbook, 0, len(doc.Books))
	for _, el := range doc.Books {
		book, err := el.build()
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		books = append(books, book)
	}
	return skill.NewSpellbookTable(books)
}

// summonItemElement is one <item> element. id, npcId and summonType are all
// required.
type summonItemElement struct {
	ID         *coord32 `xml:"id,attr"`
	NPCID      *coord32 `xml:"npcId,attr"`
	SummonType *coord   `xml:"summonType,attr"`
}

func (e summonItemElement) build() (item.SummonItem, error) {
	if e.ID == nil {
		return item.SummonItem{}, fmt.Errorf("summon item: id is required")
	}
	if e.NPCID == nil {
		return item.SummonItem{}, fmt.Errorf("summon item %d: npcId is required", *e.ID)
	}
	if e.SummonType == nil {
		return item.SummonItem{}, fmt.Errorf("summon item %d: summonType is required", *e.ID)
	}
	return item.SummonItem{
		ItemID:     int32(*e.ID),
		NPCID:      int32(*e.NPCID),
		SummonType: int(*e.SummonType),
	}, nil
}

type summonItemFile struct {
	Items []summonItemElement `xml:"item"`
}

func LoadSummonItems(path string) (*item.SummonItemTable, error) {
	var doc summonItemFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("summon items: %w", err)
	}

	items := make([]item.SummonItem, 0, len(doc.Items))
	for _, el := range doc.Items {
		entry, err := el.build()
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		items = append(items, entry)
	}
	return item.NewSummonItemTable(items)
}

// healSpsElement is one <healSps> element. correction and neededMatk are
// required; skillId/skillLevel and magicLevel are each optional (skillLevel
// is required when skillId is present), but at least one selector must be
// set.
type healSpsElement struct {
	Correction *floatAttr `xml:"correction,attr"`
	NeededMAtk *coord     `xml:"neededMatk,attr"`
	SkillID    *coord32   `xml:"skillId,attr"`
	SkillLevel *coord     `xml:"skillLevel,attr"`
	MagicLevel *coord     `xml:"magicLevel,attr"`
}

func (e healSpsElement) build() (skill.HealSps, error) {
	if e.Correction == nil {
		return skill.HealSps{}, fmt.Errorf("heal sps: correction is required")
	}
	if e.NeededMAtk == nil {
		return skill.HealSps{}, fmt.Errorf("heal sps: neededMatk is required")
	}
	var skillID *int32
	if e.SkillID != nil {
		v := int32(*e.SkillID)
		skillID = &v
	}
	var skillLevel *int
	if e.SkillLevel != nil {
		v := int(*e.SkillLevel)
		skillLevel = &v
	}
	var magicLevel *int
	if e.MagicLevel != nil {
		v := int(*e.MagicLevel)
		magicLevel = &v
	}
	return skill.NewHealSps(float64(*e.Correction), int(*e.NeededMAtk), skillID, skillLevel, magicLevel)
}

type healSpsFile struct {
	Entries []healSpsElement `xml:"healSps"`
}

func LoadHealSps(path string) (*skill.HealSpsTable, error) {
	var doc healSpsFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("heal sps: %w", err)
	}

	entries := make([]skill.HealSps, 0, len(doc.Entries))
	for _, el := range doc.Entries {
		entry, err := el.build()
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		entries = append(entries, entry)
	}
	return skill.NewHealSpsTable(entries)
}

// newbieBuffElement is one <buff> element. Every attribute is required
// except isMagicClass, which defaults to false.
type newbieBuffElement struct {
	SkillID      *coord32 `xml:"skillId,attr"`
	SkillLevel   *coord   `xml:"skillLevel,attr"`
	LowerLevel   *coord   `xml:"lowerLevel,attr"`
	UpperLevel   *coord   `xml:"upperLevel,attr"`
	IsMagicClass boolAttr `xml:"isMagicClass,attr"`
}

func (e newbieBuffElement) build() (skill.NewbieBuff, error) {
	if e.SkillID == nil {
		return skill.NewbieBuff{}, fmt.Errorf("newbie buff: skillId is required")
	}
	if e.SkillLevel == nil {
		return skill.NewbieBuff{}, fmt.Errorf("newbie buff %d: skillLevel is required", *e.SkillID)
	}
	if e.LowerLevel == nil {
		return skill.NewbieBuff{}, fmt.Errorf("newbie buff %d: lowerLevel is required", *e.SkillID)
	}
	if e.UpperLevel == nil {
		return skill.NewbieBuff{}, fmt.Errorf("newbie buff %d: upperLevel is required", *e.SkillID)
	}
	return skill.NewNewbieBuff(int32(*e.SkillID), int(*e.SkillLevel), int(*e.LowerLevel), int(*e.UpperLevel), bool(e.IsMagicClass)), nil
}

type newbieBuffFile struct {
	Buffs []newbieBuffElement `xml:"buff"`
}

func LoadNewbieBuffs(path string) (*skill.NewbieBuffTable, error) {
	var doc newbieBuffFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("newbie buffs: %w", err)
	}

	buffs := make([]skill.NewbieBuff, 0, len(doc.Buffs))
	for _, el := range doc.Buffs {
		buff, err := el.build()
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		buffs = append(buffs, buff)
	}
	return skill.NewNewbieBuffTable(buffs), nil
}

type adminAccessFile struct {
	Entries []attrsElement `xml:"access"`
}

type adminCommandFile struct {
	Entries []attrsElement `xml:"aCar"`
}

func LoadAdminData(dir string) (*admin.Data, error) {
	accessPath := filepath.Join(dir, "accessLevels.xml")
	commandPath := filepath.Join(dir, "adminCommands.xml")

	var accessDoc adminAccessFile
	if err := readXML(accessPath, &accessDoc); err != nil {
		return nil, fmt.Errorf("admin access levels: %w", err)
	}
	levels, err := buildAll(accessPath, accessDoc.Entries, admin.NewAccessLevel)
	if err != nil {
		return nil, err
	}

	var commandDoc adminCommandFile
	if err := readXML(commandPath, &commandDoc); err != nil {
		return nil, fmt.Errorf("admin commands: %w", err)
	}
	commands, err := buildAll(commandPath, commandDoc.Entries, admin.NewCommand)
	if err != nil {
		return nil, err
	}

	data, err := admin.NewData(levels, commands)
	if err != nil {
		return nil, fmt.Errorf("xml: admin data in %s: %w", dir, err)
	}
	return data, nil
}

type announcementFile struct {
	Entries []attrsElement `xml:"announcement"`
}

func LoadAnnouncements(path string) ([]admin.Announcement, error) {
	var doc announcementFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("announcements: %w", err)
	}

	announcements := make([]admin.Announcement, 0, len(doc.Entries))
	for _, el := range doc.Entries {
		set := commons.StatSetFromXMLAttrs(el.Attrs)
		message, err := set.GetString("message")
		if err != nil || message == "" {
			continue
		}
		announcement, err := admin.NewAnnouncement(set)
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		announcements = append(announcements, announcement)
	}
	return announcements, nil
}

type observerGroupFile struct {
	Groups []observerGroupElement `xml:"groups>group"`
	Spawns []attrsElement         `xml:"spawns>spawn"`
}

type observerGroupElement struct {
	ID      int            `xml:"id,attr"`
	Entries []attrsElement `xml:"entry"`
}

func LoadObserverGroups(path string) (*observer.Table, error) {
	var doc observerGroupFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("observer groups: %w", err)
	}

	groups := make(map[int][]observer.Location, len(doc.Groups))
	for _, groupEl := range doc.Groups {
		entries := groups[groupEl.ID]
		for _, el := range groupEl.Entries {
			entry, err := observer.NewLocation(commons.StatSetFromXMLAttrs(el.Attrs))
			if err != nil {
				return nil, fmt.Errorf("xml: %s: group %d: %w", path, groupEl.ID, err)
			}
			entries = append(entries, entry)
		}
		groups[groupEl.ID] = entries
	}

	spawns := make([]observer.Spawn, 0, len(doc.Spawns))
	for _, el := range doc.Spawns {
		entry, err := observer.NewSpawn(commons.StatSetFromXMLAttrs(el.Attrs))
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		spawns = append(spawns, entry)
	}
	return observer.NewTable(groups, spawns), nil
}

type staticObjectFile struct {
	Objects []attrsElement `xml:"object"`
}

func LoadStaticObjects(path string) (*staticobject.Table, error) {
	var doc staticObjectFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("static objects: %w", err)
	}

	templates, err := buildAll(path, doc.Objects, staticobject.NewTemplate)
	if err != nil {
		return nil, err
	}
	return staticobject.NewTable(templates)
}

// cursedWeaponElement is one <item> element. Every attribute is required.
type cursedWeaponElement struct {
	ID              *coord32 `xml:"id,attr"`
	SkillID         *coord32 `xml:"skillId,attr"`
	Name            *string  `xml:"name,attr"`
	DropRate        *coord   `xml:"dropRate,attr"`
	Duration        *coord   `xml:"duration,attr"`
	DurationLost    *coord   `xml:"durationLost,attr"`
	DisappearChance *coord   `xml:"dissapearChance,attr"`
	StageKills      *coord   `xml:"stageKills,attr"`
}

func (e cursedWeaponElement) build(skills *skill.Table) (entity.CursedWeapon, error) {
	if e.ID == nil {
		return entity.CursedWeapon{}, fmt.Errorf("cursed weapon: id is required")
	}
	if e.SkillID == nil {
		return entity.CursedWeapon{}, fmt.Errorf("cursed weapon %d: skillId is required", *e.ID)
	}
	if e.Name == nil {
		return entity.CursedWeapon{}, fmt.Errorf("cursed weapon %d: name is required", *e.ID)
	}
	if e.DropRate == nil {
		return entity.CursedWeapon{}, fmt.Errorf("cursed weapon %d: dropRate is required", *e.ID)
	}
	if e.Duration == nil {
		return entity.CursedWeapon{}, fmt.Errorf("cursed weapon %d: duration is required", *e.ID)
	}
	if e.DurationLost == nil {
		return entity.CursedWeapon{}, fmt.Errorf("cursed weapon %d: durationLost is required", *e.ID)
	}
	if e.DisappearChance == nil {
		return entity.CursedWeapon{}, fmt.Errorf("cursed weapon %d: dissapearChance is required", *e.ID)
	}
	if e.StageKills == nil {
		return entity.CursedWeapon{}, fmt.Errorf("cursed weapon %d: stageKills is required", *e.ID)
	}
	return entity.NewCursedWeapon(int32(*e.ID), int32(*e.SkillID), *e.Name, int(*e.DropRate), int(*e.Duration), int(*e.DurationLost), int(*e.DisappearChance), int(*e.StageKills), skills)
}

type cursedWeaponFile struct {
	Items []cursedWeaponElement `xml:"item"`
}

func LoadCursedWeapons(path string, skills *skill.Table) (*entity.CursedWeaponTable, error) {
	var doc cursedWeaponFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("cursed weapons: %w", err)
	}

	weapons := make([]entity.CursedWeapon, 0, len(doc.Items))
	for _, el := range doc.Items {
		weapon, err := el.build(skills)
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		weapons = append(weapons, weapon)
	}
	return entity.NewCursedWeaponTable(weapons)
}

// bufferSkillElement is one <buff> element. id is required; level, when
// present, overrides the skill's max level from the skill table; price
// defaults to 0 and desc to "".
type bufferSkillElement struct {
	ID          *coord32 `xml:"id,attr"`
	Level       *coord   `xml:"level,attr"`
	Price       *coord   `xml:"price,attr"`
	Description string   `xml:"desc,attr"`
}

func (e bufferSkillElement) build(category string, skills *skill.Table) (skill.BufferSkill, error) {
	if e.ID == nil {
		return skill.BufferSkill{}, fmt.Errorf("buffer skill: id is required")
	}
	var level *int
	if e.Level != nil {
		v := int(*e.Level)
		level = &v
	}
	price := 0
	if e.Price != nil {
		price = int(*e.Price)
	}
	return skill.NewBufferSkill(int32(*e.ID), category, level, price, e.Description, skills)
}

type bufferSkillFile struct {
	Categories []bufferSkillCategory `xml:"category"`
}

type bufferSkillCategory struct {
	Type  string               `xml:"type,attr"`
	Buffs []bufferSkillElement `xml:"buff"`
}

func LoadBufferSkills(path string, skills *skill.Table) (*skill.BufferTable, error) {
	var doc bufferSkillFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("buffer skills: %w", err)
	}

	entries := make([]skill.BufferSkill, 0)
	for _, category := range doc.Categories {
		for _, el := range category.Buffs {
			entry, err := el.build(category.Type, skills)
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", path, err)
			}
			entries = append(entries, entry)
		}
	}
	return skill.NewBufferTable(entries)
}
