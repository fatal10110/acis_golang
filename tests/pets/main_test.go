package pets

import (
	"os"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func TestMain(m *testing.M) {
	gameservertest.DriveClock()
	os.Exit(sqltest.Main(m))
}
