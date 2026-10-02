package serverpackets

import (
	"bytes"
	"testing"
)

// Golden bytes follow the record writer field by field: opcode 0xf5, page,
// period ordinal, then the page's body. Page 1 is [d dd cc dd (ddd c)(ddd c)],
// page 3 [ccc (cccc)x3], page 4 [cc (cchh)x3]; any other page stops after
// the period.
func TestFrameSSQStatusPages(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  []byte
		want []byte
	}{
		{
			name: "record page",
			got: framePayload(t, FrameSSQStatusRecord(1, SSQRecord{
				Cycle: 3, PeriodMessage: SystemMessageQuestEventPeriod, UntilMessage: SystemMessageUntilMonday6PM,
				PlayerCabal: 2, PlayerSeal: 2, PlayerStones: 2, PlayerAdena: 8,
				Dusk: SSQCabalStanding{StoneScore: 357, FestivalScore: 0, TotalScore: 357, Percent: 71},
				Dawn: SSQCabalStanding{StoneScore: 143, FestivalScore: 40, TotalScore: 183, Percent: 29},
			})),
			want: []byte{
				0xf5, 0x01, 0x01,
				0x03, 0x00, 0x00, 0x00, // cycle
				0x98, 0x04, 0x00, 0x00, // 1176 QUEST_EVENT_PERIOD
				0x06, 0x05, 0x00, 0x00, // 1286 UNTIL_MONDAY_6PM
				0x02, 0x02, // cabal dawn, seal gnosis
				0x02, 0x00, 0x00, 0x00, // stones turned in
				0x08, 0x00, 0x00, 0x00, // ancient adena to collect
				0x65, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x65, 0x01, 0x00, 0x00, 0x47, // dusk
				0x8f, 0x00, 0x00, 0x00, 0x28, 0x00, 0x00, 0x00, 0xb7, 0x00, 0x00, 0x00, 0x1d, // dawn
			},
		},
		{
			name: "seals page",
			got: framePayload(t, FrameSSQStatusSeals(3, []SSQSealVotes{
				{Seal: 1, Owner: 2, DuskPercent: 0, DawnPercent: 33},
				{Seal: 2, Owner: 1, DuskPercent: 50, DawnPercent: 0},
				{Seal: 3, Owner: 0, DuskPercent: 0, DawnPercent: 0},
			})),
			want: []byte{
				0xf5, 0x03, 0x03,
				0x0a, 0x23, 0x03, // keep 10%, claim 35%, three seals
				0x01, 0x02, 0x00, 0x21,
				0x02, 0x01, 0x32, 0x00,
				0x03, 0x00, 0x00, 0x00,
			},
		},
		{
			name: "prediction page",
			got: framePayload(t, FrameSSQStatusPrediction(1, 2, []SSQSealPrediction{
				{Owner: 0, Predicted: 2, Message: SystemMessageSealNotOwned35MoreVoted},
				{Owner: 1, Predicted: 1, Message: SystemMessageSealOwned10MoreVoted},
				{Owner: 2, Predicted: 0, Message: SystemMessageSealOwned10LessVoted},
			})),
			want: []byte{
				0xf5, 0x04, 0x01,
				0x02, 0x03, // predicted winner dawn, three seals
				0x00, 0x02, 0x0a, 0x05, 0x00, 0x00, // 1290
				0x01, 0x01, 0x09, 0x05, 0x00, 0x00, // 1289
				0x02, 0x00, 0x0b, 0x05, 0x00, 0x00, // 1291
			},
		},
		{
			name: "page without content",
			got:  framePayload(t, FrameSSQStatusHeader(7, 0)),
			want: []byte{0xf5, 0x07, 0x00},
		},
	} {
		if !bytes.Equal(tc.got, tc.want) {
			t.Errorf("%s = % x, want % x", tc.name, tc.got, tc.want)
		}
	}
}
