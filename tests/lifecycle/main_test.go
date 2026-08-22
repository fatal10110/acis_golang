package lifecycle

import (
	"os"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
)

func TestMain(m *testing.M) {
	os.Exit(sqltest.Main(m))
}
