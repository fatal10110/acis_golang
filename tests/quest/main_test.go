package quest

import (
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/dbtest"
)

func TestMain(m *testing.M) {
	// The hall scenarios print the stored next siege date in the server's zone, which the
	// reference traces fix at UTC.
	time.Local = time.UTC
	os.Exit(dbtest.Main(m))
}
