package social

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestBlockListOrderAfterUnblock pins the block list a player reads after
// blocking twenty characters and unblocking every other one (#5104 reads
// lists per character): /blocklist names exactly the ten still blocked,
// grouped by bucket of the 16-bucket set the client order follows, the
// order the manager lists them in.
func TestBlockListOrderAfterUnblock(t *testing.T) {
	t.Parallel()
	p := bootPair(t)
	var ids []int32
	names := map[int32]string{}
	for i := range 20 {
		name := fmt.Sprintf("Ignored%02d", i)
		ids = append(ids, p.srv.SeedCharacterFor(t, "ignored", name, 1, 0).ID)
		names[ids[i]] = name
	}
	p.enterAll(t)

	for _, id := range ids {
		p.alice.Send(encodeBlock(clientpackets.BlockAdd, names[id]))
		assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1AddedToYourIgnoreList, names[id])
	}
	var kept []int32
	for i, id := range ids {
		if i%2 == 1 {
			kept = append(kept, id)
			continue
		}
		p.alice.Send(encodeBlock(clientpackets.BlockRemove, names[id]))
		assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1RemovedFromYourIgnoreList, names[id])
	}

	byBucket := func(a, b int32) int { return int(a%16) - int(b%16) }
	if slices.IsSortedFunc(kept, byBucket) {
		t.Fatalf("kept ids %v already ascend by bucket; the scenario needs ids past one lap", kept)
	}
	want := p.srv.Relations.BlockedIDs(p.aliceID)
	if !slices.IsSortedFunc(want, byBucket) || !slices.Equal(slices.Sorted(slices.Values(want)), kept) {
		t.Fatalf("BlockedIDs = %v, want the ids %v grouped by bucket of 16", want, kept)
	}

	p.alice.Send(encodeBlock(clientpackets.BlockList, ""))
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageBlockListHeader)
	for i, id := range want {
		assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1, strconv.Itoa(i+1)+". "+names[id])
	}
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageFriendListFooter)
	assertSilent(t, p.alice, "block list reader")
}
