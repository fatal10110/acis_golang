package network

import (
	"testing"

	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestSendResurrectionScrollRefusal pins the answer to each refusal of a
// resurrection scroll: one frame, the reference's system message or text.
func TestSendResurrectionScrollRefusal(t *testing.T) {
	tests := []struct {
		name    string
		refusal itemhandler.ResurrectionScrollRefusal
		want    int
		text    string
	}{
		{name: "invalid target", refusal: itemhandler.ResurrectionScrollInvalidTarget, want: serverpackets.SystemMessageInvalidTarget},
		{name: "siege", refusal: itemhandler.ResurrectionScrollSiege, want: serverpackets.SystemMessageCannotBeResurrectedDuringSiege},
		{name: "festival", refusal: itemhandler.ResurrectionScrollFestival, want: serverpackets.SystemMessageS1, text: festivalResurrectionRefusal},
		{name: "pet offer open", refusal: itemhandler.ResurrectionScrollPetOfferOpen, want: serverpackets.SystemMessageCannotResMaster},
		{name: "already proposed", refusal: itemhandler.ResurrectionScrollAlreadyProposed, want: serverpackets.SystemMessageResHasAlreadyBeenProposed},
		{name: "owner offer open", refusal: itemhandler.ResurrectionScrollOwnerOfferOpen, want: serverpackets.SystemMessageCannotResPet2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frames := &testsupport.FrameCapture{}
			live := newTestLivePlayer(t, 1, frames)
			testsupport.ResetCapture(frames)

			sendResurrectionScrollRefusal(live, tt.refusal)

			got := frames.Frames()
			if len(got) != 1 {
				t.Fatalf("frames = %d, want 1", len(got))
			}
			if tt.text != "" {
				assertSystemMessageStringFrame(t, got[0], tt.want, tt.text)
				return
			}
			assertSystemMessageIDFrame(t, got[0], tt.want)
		})
	}
	if serverpackets.SystemMessageCannotBeResurrectedDuringSiege != 1053 || serverpackets.SystemMessageCannotResMaster != 1514 {
		t.Fatalf("siege/master ids = %d/%d, want 1053/1514",
			serverpackets.SystemMessageCannotBeResurrectedDuringSiege, serverpackets.SystemMessageCannotResMaster)
	}
}
