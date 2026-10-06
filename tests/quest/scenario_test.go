package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	quest001 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	"github.com/fatal10110/acis_golang/internal/testsupport/scenario"
)

// scenarioScripts are the ported scripts a quest scenario may list, by
// scripts.xml path, as the server's catalogs map them.
var scenarioScripts = script.Catalog{
	"quest.Q001_LettersOfLove": quest001.New,
}

// TestScenarios plays every quest scenario of testdata/scenarios through
// client packets against the booted server and MariaDB (format: package
// scenario).
func TestScenarios(t *testing.T) {
	t.Parallel()
	scenario.RunDir(t, "testdata/scenarios", scenario.Config{Catalog: scenarioScripts})
}
