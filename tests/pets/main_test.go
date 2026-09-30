package pets

import (
	"os"
	"testing"

	"github.com/fatal10110/acis_golang/internal/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}
