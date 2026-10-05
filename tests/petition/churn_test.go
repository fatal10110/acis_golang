package petition

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
)

// TestPetitionChurnIsBounded drives a RequestPetition/RequestPetitionCancel
// loop (#3224). The player gets every answer the reference gives, but the
// game masters hear only the first four notices of the minute, and the
// player keeps only its five most recent cancelled petitions, listed and
// stored; the older ones are dropped. The reference bounds neither.
func TestPetitionChurnIsBounded(t *testing.T) {
	t.Parallel()
	r := boot(t)
	r.enterAll(t)

	const rounds, kept = 12, 5
	var ids []int32
	for i := range rounds {
		frames := exchange(t, r.player, encodePetition("help", 3))
		if len(frames) != 3 {
			t.Fatalf("submit %d frames = %d, want the three acceptance messages", i, len(frames))
		}
		id, params := systemMessage(t, frames[0])
		if id != serverpackets.SystemMessagePetitionAcceptedRecentNoS1 {
			t.Fatalf("submit %d answer = %d, want PETITION_ACCEPTED_RECENT_NO_S1", i, id)
		}
		assertMessages(t, frames[1:],
			msg(serverpackets.SystemMessageSubmittedYourS1ThPetitionS2Left, int32(1), int32(4)),
			msg(serverpackets.SystemMessageS1PetitionOnWaitingList, int32(1)))
		ids = append(ids, params[0].(int32))
		assertMessages(t, exchange(t, r.player, encodePetitionCancel()), msg(serverpackets.SystemMessagePetitionCanceledSubmitS1MoreToday, int32(5)))
	}
	assertSays(t, collect(t, r.gm),
		say{r.playerID, sayHeroVoice, "Petition System", "Player has submitted a new petition."},
		say{r.playerID, sayHeroVoice, "Petition System", "Player has canceled a pending petition."},
		say{r.playerID, sayHeroVoice, "Petition System", "Player has submitted a new petition."},
		say{r.playerID, sayHeroVoice, "Petition System", "Player has canceled a pending petition."})

	list := r.srv.Petitions.List()
	if len(list) != kept {
		t.Fatalf("petitions = %+v, want the %d most recent cancelled", list, kept)
	}
	for i, p := range list {
		if want := ids[rounds-kept+i]; p.ID != want || p.State != petition.Cancelled {
			t.Fatalf("petition %d = %+v, want %d cancelled", i, p, want)
		}
	}
	r.srv.SavePetitions(t)
	var stored int
	if err := r.srv.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM petition WHERE petitioner_oid = ?", r.playerID).Scan(&stored); err != nil {
		t.Fatalf("count stored petitions: %v", err)
	}
	if stored != kept {
		t.Fatalf("stored petitions = %d, want %d", stored, kept)
	}
}
