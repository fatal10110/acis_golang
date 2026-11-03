package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestFishingEmoteFloodAnswersOncePerSocialTime pins the SOCIAL reuse gate
// ahead of the fishing check: a fisher spamming emotes is told
// CANNOT_DO_WHILE_FISHING_3 once per SocialTime, and the requests in between
// answer nothing.
func TestFishingEmoteFloodAnswersOncePerSocialTime(t *testing.T) {
	t.Parallel()
	const socialTime = 2 * time.Second
	r := bootFishing(t, gameservertest.WithSocialDelay(socialTime))
	if !r.srv.DrivesClock() {
		t.Skip("placing requests on either side of the reuse window needs the driven clock")
	}
	r.cast(t, fishingSkill)

	start := r.c.Now()
	r.c.Send(encodeRequestSocialActionFishing(2))
	requireEvents(t, "first emote while fishing", fishingEvents(t, r.c), "msg 1471")
	r.c.Send(encodeRequestSocialActionFishing(3))
	requireEvents(t, "emote while fishing inside SocialTime", fishingEvents(t, r.c))

	if d := start.Add(socialTime).Sub(r.c.Now()); d > 0 {
		r.srv.Advance(t, d)
	}
	r.c.Send(encodeRequestSocialActionFishing(4))
	requireEvents(t, "emote while fishing after SocialTime", fishingEvents(t, r.c), "msg 1471")
}
