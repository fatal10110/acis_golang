// Package lottery holds the behavior suites for the Lucky Lottery: its
// weekly calendar, the drawing and its payout, and their persistence.
package lottery

import (
	"os"
	"testing"

	"github.com/fatal10110/acis_golang/internal/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}
