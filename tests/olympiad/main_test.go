// Package olympiad holds the behavior suites for the Olympiad: its
// calendar, the nobles' records and their persistence, and the commands and
// login state built on noble status.
package olympiad

import (
	"os"
	"testing"

	"github.com/fatal10110/acis_golang/internal/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}
