package character

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// protectedLoginBurst is loginWeightBurst under PlayerSpawnProtection > 0.
// EnterWorld starts the protection just before spawnMe (EnterWorld.java:189),
// and setSpawnProtection ends in broadcastUserInfo (Player.java:2119), so a
// UserInfo reaches the client right after the clan and Seal of Strife block
// (here the EtcStatusUpdate that precedes them), ahead of the compass code,
// the welcome messages and the burst's own UserInfo.
var protectedLoginBurst = []byte{
	serverpackets.OpcodeSendMacroList,
	serverpackets.OpcodeExtended, // ExStorageMaxCount
	serverpackets.OpcodeHennaInfo,
	serverpackets.OpcodeEtcStatusUpdate,
	serverpackets.OpcodeUserInfo, // setSpawnProtection(true)
	serverpackets.OpcodeExtended, // ExSetCompassZoneCode
	serverpackets.OpcodeSystemMessage,
	serverpackets.OpcodeSystemMessage,
	serverpackets.OpcodeQuestList,
	serverpackets.OpcodeSkillList,
	serverpackets.OpcodeFriendList,
	serverpackets.OpcodeUserInfo,
	serverpackets.OpcodeStatusUpdate, // CUR_LOAD
	serverpackets.OpcodeItemList,
	serverpackets.OpcodeShortCutInit,
	serverpackets.OpcodeSkillCoolTime,
	serverpackets.OpcodeActionFailed,
}

// TestEnterWorldBurstOrderUnderSpawnProtection pins the EnterWorld burst
// with a spawn protection window: one extra UserInfo, in the place
// setSpawnProtection(true) queues it.
func TestEnterWorldBurstOrderUnderSpawnProtection(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSpawnProtection(time.Minute),
		gameservertest.WithSeed(func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
			objID := seedCharacter(t, chars, "Newbie", 1, 0)
			if err := items.Create(context.Background(), objID, item.Instance{
				ObjectID: 500, TemplateID: 9500, OwnerID: objID, Count: 5, Location: item.LocationInventory,
			}); err != nil {
				t.Fatalf("seed item: %v", err)
			}
		}),
	)
	readLoginBurst(t, srv.Client, protectedLoginBurst)
}
