package quest

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/blackjudge"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/cabalbuffer"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/newbiehelper"
	featuretutorial "github.com/fatal10110/acis_golang/internal/gameserver/script/feature/tutorial"
	quest001 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	quest023 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q023"
	quest211 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q211"
	quest234 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q234"
	quest235 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q235"
	quest260 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q260"
	quest316 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q316"
	quest360 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q360"
	quest422 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q422"
	quest508 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q508"
	quest510 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q510"
	quest607 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q607"
	quest646 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q646"
	quest651 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q651"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/noblesseteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/primevalsummoner"
	"github.com/fatal10110/acis_golang/internal/testsupport/scenario"
)

// scenarioScripts are the ported scripts a quest scenario may list, by
// scripts.xml path, as the server's catalogs map them; the newbie helper
// casts from an empty newbie buff table.
var scenarioScripts = script.Catalog{
	"script.feature.NewbieHelper":        func() script.Script { return newbiehelper.New(modelskill.NewNewbieBuffTable(nil)) },
	"script.feature.Tutorial":            featuretutorial.New,
	"script.feature.BlackJudge":          blackjudge.New,
	"script.feature.CabalBuffer":         cabalbuffer.New,
	"script.teleport.NoblesseTeleporter": noblesseteleporter.New,
	"script.teleport.PrimevalSummoner":   primevalsummoner.New,
	"quest.Q001_LettersOfLove":           quest001.New,
	"quest.Q023_LidiasHeart":             quest023.New,
	"quest.Q211_TrialOfTheChallenger":    quest211.New,
	"quest.Q234_FatesWhisper":            quest234.New,
	"quest.Q235_MimirsElixir":            quest235.New,
	"quest.Q260_HuntTheOrcs":             quest260.New,
	"quest.Q316_DestroyPlagueCarriers":   quest316.New,
	"quest.Q360_PlunderTheirSupplies":    quest360.New,
	"quest.Q422_RepentYourSins":          quest422.New,
	"quest.Q508_AClansReputation":        quest508.New,
	"quest.Q510_AClansReputation":        quest510.New,
	"quest.Q607_ProveYourCourage":        quest607.New,
	"quest.Q646_SignsOfRevolt":           quest646.New,
	"quest.Q651_RunawayYouth":            quest651.New,
}

// TestScenarios plays every quest scenario of testdata/scenarios through
// client packets against the booted server and MariaDB (format: package
// scenario).
func TestScenarios(t *testing.T) {
	t.Parallel()
	scenario.RunDir(t, "testdata/scenarios", scenario.Config{Catalog: scenarioScripts})
}
