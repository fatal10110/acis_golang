package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
)

// writeDataFile writes body as root's data/xml/dir/name.
func writeDataFile(t *testing.T, root, dir, name, body string) {
	t.Helper()
	path := filepath.Join(root, "data", "xml", dir)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestDataReloadsSwapItemAndSkillTemplates requires the item and skill
// reload hooks to read the datapack's items and skills directories into the
// tables the server already holds, and to leave those tables as they were
// when a file cannot be read.
func TestDataReloadsSwapItemAndSkillTemplates(t *testing.T) {
	root := t.TempDir()
	writeDataFile(t, root, "items", "0000-0099.xml", `<?xml version="1.0" encoding="UTF-8"?>
<list>
	<item id="57" type="EtcItem" name="Adena">
		<set name="material" val="GOLD" />
		<set name="is_stackable" val="true" />
	</item>
</list>`)
	writeDataFile(t, root, "skills", "0000-0099.xml", `<?xml version="1.0" encoding="UTF-8"?>
<list>
	<skill id="195" levels="2" name="Boost Breath">
		<table name="#magicLvl"> 20 55 </table>
		<set name="magicLvl" val="#magicLvl"/>
		<set name="target" val="SELF"/>
		<set name="skillType" val="BUFF"/>
		<set name="operateType" val="PASSIVE"/>
	</skill>
</list>`)
	data := &gameData{Items: item.NewTable(nil), Skills: skill.NewTable(nil)}
	items, skills := data.Items, data.Skills
	reloads := provideDataReloads(gameServerPaths{DataRoot: root}, data, nil, nil, nil, nil, gameplayConfig{}, zerolog.Nop())

	if err := reloads.Items(); err != nil {
		t.Fatalf("item reload: %v", err)
	}
	if err := reloads.Skills(); err != nil {
		t.Fatalf("skill reload: %v", err)
	}
	if tpl, ok := items.Get(item.AdenaID); !ok || tpl.Name != "Adena" {
		t.Fatalf("held item table after the reload = %+v, %v; want Adena", tpl, ok)
	}
	if def, ok := skills.Get(195, 2); !ok || def.Name != "Boost Breath" || skills.MaxLevel(195) != 2 {
		t.Fatalf("held skill table after the reload = %+v, %v, max %d; want Boost Breath up to level 2", def, ok, skills.MaxLevel(195))
	}

	writeDataFile(t, root, "items", "0000-0099.xml", "<list><item")
	writeDataFile(t, root, "skills", "0000-0099.xml", "<list><skill")
	if err := reloads.Items(); err == nil {
		t.Fatal("item reload of a broken file succeeded")
	}
	if err := reloads.Skills(); err == nil {
		t.Fatal("skill reload of a broken file succeeded")
	}
	if _, ok := items.Get(item.AdenaID); !ok {
		t.Fatal("a failed item reload emptied the held table")
	}
	if _, ok := skills.Get(195, 2); !ok {
		t.Fatal("a failed skill reload emptied the held table")
	}
}
