package quest

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	quest001 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	"github.com/fatal10110/acis_golang/internal/testsupport/scenario"
)

// unofferedBypass is a scenario whose last bypass step sends a quest link
// that Darin's first page does not offer: the server drops it, silently.
const unofferedBypass = `character Talker 20
script quest.Q001_LettersOfLove
page default/30048.htm
page script/quest/Q001_LettersOfLove/
npc darin 30048 Folk 10 0 0

action darin
action darin
  S ActionFailed
  S NpcHtmlMessage obj=darin item=0 page=default/30048.htm
  S ActionFailed
bypass Quest Q001_LettersOfLove 30048-06.htm
%s  state Q001_LettersOfLove -
  rows Q001_LettersOfLove -
`

// unofferedBypassEnv makes the test binary play the unmarked scenario, which
// must fail, instead of testing it.
const unofferedBypassEnv = "ACIS_UNOFFERED_BYPASS_SCENARIO"

func runUnofferedBypass(t *testing.T, marker string) {
	t.Helper()
	sc, err := scenario.Parse("unoffered.scenario", []byte(strings.Replace(unofferedBypass, "%s", marker, 1)))
	if err != nil {
		t.Fatal(err)
	}
	scenario.Run(t, sc, scenario.Config{Catalog: script.Catalog{"quest.Q001_LettersOfLove": quest001.New}})
}

// TestScenarioRunnerFailsABypassTheLastPageDidNotOffer: the server drops a
// quest link the last page did not offer and answers nothing, so a step that
// sends one and expects nothing would pass without doing anything. The runner
// fails it, unless the step is marked rejected.
func TestScenarioRunnerFailsABypassTheLastPageDidNotOffer(t *testing.T) {
	if os.Getenv(unofferedBypassEnv) == "1" {
		runUnofferedBypass(t, "")
		return
	}
	t.Run("marked rejected", func(t *testing.T) {
		runUnofferedBypass(t, "  rejected\n")
	})
	t.Run("unmarked", func(t *testing.T) {
		// The failing run is another process: a scenario fails its test.
		cmd := exec.Command(os.Args[0], "-test.run=^TestScenarioRunnerFailsABypassTheLastPageDidNotOffer$")
		cmd.Env = append(os.Environ(), unofferedBypassEnv+"=1")
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("the unmarked scenario passed:\n%s", out)
		}
		if want := "does not offer Quest Q001_LettersOfLove 30048-06.htm"; !strings.Contains(string(out), want) {
			t.Fatalf("the unmarked scenario failed without %q:\n%s", want, out)
		}
	})
}
