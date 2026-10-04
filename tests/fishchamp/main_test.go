// Package fishchamp holds the behavior suites for the fishing
// championship: its weekly calendar, the catch ranking, the prize claims
// and their persistence.
package fishchamp

import (
	"os"
	"testing"

	"github.com/fatal10110/acis_golang/internal/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}
