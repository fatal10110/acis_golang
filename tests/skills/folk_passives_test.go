package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: L2Skill.getStatFuncs gives a passive's stat funcs only to a
// playable or an attackable owner, so the template passives of a Folk, a
// control tower or a siege headquarters change none of its stats.

const (
	// hpModifiersSkillID at level 11 triples max HP and HP regeneration,
	// as on "Flame of the Branded" (18299).
	hpModifiersSkillID = 4408
	// resistFullMagicSkillID cuts root, sleep, stun and paralysis
	// vulnerability to a tenth, as on most civilian NPCs.
	resistFullMagicSkillID = 4045
)

// TestFolkFamilyIgnoresTemplatePassives spawns, through the production
// civilian spawner over the shipped passive skills, civilian NPCs of every
// family carrying HP Modifiers 4408-11 and Resist Full Magic Attack
// 4045-1: each has its bare twin's max HP, and full root and stun
// vulnerability.
func TestFolkFamilyIgnoresTemplatePassives(t *testing.T) {
	t.Parallel()
	defs := []modelskill.Definition{shippedSkill(t, hpModifiersSkillID, 11), shippedSkill(t, resistFullMagicSkillID, 1)}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
	)
	table := modelskill.NewTable(defs)
	passives := []modelskill.Ref{{ID: hpModifiersSkillID, Level: 11}, {ID: resistFullMagicSkillID, Level: 1}}
	for i, kind := range []string{"Folk", "Merchant", "LifeTower", "FlameTower", "SiegeFlag"} {
		at := location.Location{X: hostileX + 100*i, Y: hostileY, Z: hostileZ}
		base := *gameservertest.FolkTemplate(kind, 18299)
		base.Name, base.HPMax = "Flame of the Branded", 772
		bare := srv.SpawnCastingFolkNPCAt(t, npc.NewTemplate(base, npc.TemplateSkills{}), at, table)
		armed := srv.SpawnCastingFolkNPCAt(t, npc.NewTemplate(base, npc.TemplateSkills{Passives: passives}), at, table)
		if got, want := armed.MaxHP(), bare.MaxHP(); got != want {
			t.Errorf("%s with HP Modifiers: max HP = %d, want %d", kind, got, want)
		}
		for _, vuln := range []stat.Stat{stat.RootVuln, stat.StunVuln} {
			if got := armed.CalcStat(vuln, 1); got != 1 {
				t.Errorf("%s with Resist Full Magic Attack: %v = %v, want 1", kind, vuln, got)
			}
		}
	}
}
