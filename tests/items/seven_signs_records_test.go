package items

import (
	"bytes"
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// recordOfSevenSignsID is the shipped Record of Seven Signs (handler
// SevenSignsRecords).
const recordOfSevenSignsID int32 = 5707

// Using the Record of Seven Signs opens the record's first page for the
// user and nothing else: no ActionFailed, and the record is kept. The seeded
// competition is cycle 1 with no scores; the user signed up for Dusk
// choosing Gnosis.
func TestUseRecordOfSevenSignsOpensRecordPage(t *testing.T) {
	t.Parallel()
	datapack.Require(t)
	_, shipped := shippedData()
	tmpl, ok := shipped.Get(recordOfSevenSignsID)
	if !ok {
		t.Fatalf("shipped item %d missing", recordOfSevenSignsID)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), tmpl))),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	)
	objID := srv.SoleObjectID(t)
	record := srv.GiveItem(t, objID, recordOfSevenSignsID, 1)
	c := srv.Client
	startInWorld(t, c)
	drainUntilQuiet(t, c)
	if err := srv.SevenSigns.SetPlayerInfo(context.Background(), objID, sevensigns.Dusk, sevensigns.Gnosis); err != nil {
		t.Fatalf("sign up: %v", err)
	}

	c.Send(encodeUseItem(record, false))
	want := []byte{
		0xf5, 0x01, 0x01, // page 1, competition
		0x01, 0x00, 0x00, 0x00, // cycle 1
		0x98, 0x04, 0x00, 0x00, // QUEST_EVENT_PERIOD
		0x06, 0x05, 0x00, 0x00, // UNTIL_MONDAY_6PM
		0x01, 0x02, // dusk, gnosis
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // no stones, no ancient adena
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // dusk
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // dawn
	}
	if got := c.Read(); !bytes.Equal(got, want) {
		t.Fatalf("record page = % x, want % x", got, want)
	}
	assertQuiet(t, c, "Record of Seven Signs")
	assertStillHeld(t, srv, objID, record)
}
