package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	quest001 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	quest211 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q211"
	quest260 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q260"
	quest316 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q316"
	quest360 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q360"
	quest508 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q508"
	"github.com/fatal10110/acis_golang/internal/testsupport/scenario"
)

// scenarioScripts are the ported scripts a quest scenario may list, by
// scripts.xml path, as the server's catalogs map them.
var scenarioScripts = script.Catalog{
	"quest.Q001_LettersOfLove":         quest001.New,
	"quest.Q211_TrialOfTheChallenger":  quest211.New,
	"quest.Q260_HuntTheOrcs":           quest260.New,
	"quest.Q316_DestroyPlagueCarriers": quest316.New,
	"quest.Q360_PlunderTheirSupplies":  quest360.New,
	"quest.Q508_AClansReputation":      quest508.New,
}

// TestScenarios plays every quest scenario of testdata/scenarios through
// client packets against the booted server and MariaDB (format: package
// scenario).
func TestScenarios(t *testing.T) {
	t.Parallel()
	scenario.RunDir(t, "testdata/scenarios", scenario.Config{Catalog: scenarioScripts})
}
