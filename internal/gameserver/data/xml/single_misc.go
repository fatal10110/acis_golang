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
	Crystals []attrsElement `xml:"crystals>crystal"`
	NPCs     []attrsElement `xml:"npcs>npc"`
}

func LoadSoulCrystalData(path string) (*item.SoulCrystalTable, error) {
	var doc soulCrystalFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("xml: soul crystals %q: %w", path, err)
	}

	crystals, err := buildAll(path, doc.Crystals, item.NewSoulCrystal)
	if err != nil {
		return nil, err
	}

	infos, err := buildAll(path, doc.NPCs, item.NewSoulCrystalLevelingInfo)
	if err != nil {
		return nil, err
	}

	table, err := item.NewSoulCrystalTable(crystals, infos)
	if err != nil {
		return nil, fmt.Errorf("xml: %s: %w", path, err)
	}
	return table, nil
}

type spellbookFile struct {
	Books []attrsElement `xml:"book"`
}

func LoadSpellbooks(path string) (*skill.SpellbookTable, error) {
	var doc spellbookFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("xml: spellbooks %q: %w", path, err)
	}

	books, err := buildAll(path, doc.Books, skill.NewSpellbook)
	if err != nil {
		return nil, err
	}
	return skill.NewSpellbookTable(books)
}

type summonItemFile struct {
	Items []attrsElement `xml:"item"`
}

func LoadSummonItems(path string) (*item.SummonItemTable, error) {
	var doc summonItemFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("xml: summon items %q: %w", path, err)
	}

	items, err := buildAll(path, doc.Items, item.NewSummonItem)
	if err != nil {
		return nil, err
	}
	return item.NewSummonItemTable(items)
}

type healSpsFile struct {
	Entries []attrsElement `xml:"healSps"`
}

func LoadHealSps(path string) (*skill.HealSpsTable, error) {
	var doc healSpsFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("xml: heal sps %q: %w", path, err)
	}

	entries, err := buildAll(path, doc.Entries, skill.NewHealSps)
	if err != nil {
		return nil, err
	}
	return skill.NewHealSpsTable(entries)
}

type newbieBuffFile struct {
	Buffs []attrsElement `xml:"buff"`
}

func LoadNewbieBuffs(path string) (*skill.NewbieBuffTable, error) {
	var doc newbieBuffFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("xml: newbie buffs %q: %w", path, err)
	}

	buffs, err := buildAll(path, doc.Buffs, skill.NewNewbieBuff)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("xml: admin access levels %q: %w", accessPath, err)
	}
	levels, err := buildAll(accessPath, accessDoc.Entries, admin.NewAccessLevel)
	if err != nil {
		return nil, err
	}

	var commandDoc adminCommandFile
	if err := readXML(commandPath, &commandDoc); err != nil {
		return nil, fmt.Errorf("xml: admin commands %q: %w", commandPath, err)
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
		return nil, fmt.Errorf("xml: announcements %q: %w", path, err)
	}

	return buildAll(path, doc.Entries, admin.NewAnnouncement)
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
		return nil, fmt.Errorf("xml: observer groups %q: %w", path, err)
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
		return nil, fmt.Errorf("xml: static objects %q: %w", path, err)
	}

	templates, err := buildAll(path, doc.Objects, staticobject.NewTemplate)
	if err != nil {
		return nil, err
	}
	return staticobject.NewTable(templates)
}

type cursedWeaponFile struct {
	Items []attrsElement `xml:"item"`
}

func LoadCursedWeapons(path string, skills *skill.Table) (*entity.CursedWeaponTable, error) {
	var doc cursedWeaponFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("xml: cursed weapons %q: %w", path, err)
	}

	weapons, err := buildAll(path, doc.Items, func(set *commons.StatSet) (entity.CursedWeapon, error) {
		return entity.NewCursedWeapon(set, skills)
	})
	if err != nil {
		return nil, err
	}
	return entity.NewCursedWeaponTable(weapons)
}

type bufferSkillFile struct {
	Categories []bufferSkillCategory `xml:"category"`
}

type bufferSkillCategory struct {
	Type  string         `xml:"type,attr"`
	Buffs []attrsElement `xml:"buff"`
}

func LoadBufferSkills(path string, skills *skill.Table) (*skill.BufferTable, error) {
	var doc bufferSkillFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("xml: buffer skills %q: %w", path, err)
	}

	entries := make([]skill.BufferSkill, 0)
	for _, category := range doc.Categories {
		for _, el := range category.Buffs {
			set := commons.StatSetFromXMLAttrs(el.Attrs)
			set.Set("type", category.Type)

			entry, err := skill.NewBufferSkill(set, skills)
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", path, err)
			}
			entries = append(entries, entry)
		}
	}
	return skill.NewBufferTable(entries)
}
