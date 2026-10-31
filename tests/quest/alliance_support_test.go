package quest

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/ketraorcsupport"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/varkasilenossupport"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// What the Ketra and Varka support scenarios (tests/quest/testdata/
// scenarios/{ketra,varka}_support_*, alliance_support_*) cannot show: the
// demotion of every ally of the killer's party (a scenario seeds no party
// with a standing).

// The support features and the NPCs the test uses.
const (
	ketraSupport = "script.feature.KetraOrcSupport"
	varkaSupport = "script.feature.VarkaSilenosSupport"
	hornID       = 7186
	footmanID    = 21324
)

// supportItems is the shared catalog with the badges and the horn.
func supportItems() gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{7211, 7212, 7213, 7214, 7215, 7221, 7222, 7223, 7224, 7225, hornID} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// supportScripts registers both features, the folk ids civilians and the
// footman hostile.
func supportScripts() gameservertest.Option {
	kinds := map[int32]script.NPCKind{footmanID: script.KindHostile}
	for id := int32(31370); id <= 31383; id++ {
		kinds[id] = script.KindFolk
	}
	return gameservertest.WithNPCScripts(kinds,
		[]script.Listing{{Path: ketraSupport}, {Path: varkaSupport}},
		script.Catalog{ketraSupport: ketraorcsupport.New, varkaSupport: varkasilenossupport.New})
}

// TestAllianceSupportDemotesTheKillersParty (onMyDying, testKetraDemote):
// Leader (Ketra 5, level 5 badge), Mate (Ketra 2, level 2 badge) and Third
// (Varka 3, level 3 Varka badge) are one party; Leader kills a Ketra
// footman. Leader and Mate lose the alliance, each badge going down a
// level; Third keeps the Varka alliance and its badge.
func TestAllianceSupportDemotesTheKillersParty(t *testing.T) {
	t.Parallel()
	datapack.Require(t)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Leader", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		supportScripts(),
		supportItems(),
	)
	leader := srv.SoleObjectID(t)
	mate := srv.SeedCharacterFor(t, "mate", "Mate", 40, 0).ID
	third := srv.SeedCharacterFor(t, "third", "Third", 40, 0).ID
	for _, m := range []struct {
		id       int32
		standing int
		badge    int32
	}{{leader, 5, 7215}, {mate, 2, 7212}, {third, -3, 7223}} {
		seedStanding(t, srv, m.id, m.standing)
		srv.GiveItem(t, m.id, m.badge, 1)
	}
	enterWorld(t, srv)
	members := map[int32]*testsupport.ScriptedClient{leader: srv.Client}
	for _, m := range []struct {
		id            int32
		account, name string
	}{{mate, "mate", "Mate"}, {third, "third", "Third"}} {
		c := srv.DialClient(t, m.account, 1)
		c.Send(encodeRequestGameStart(0))
		readUntil(t, c, serverpackets.OpcodeCharSelected)
		c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
		srv.ReadQueued(t, c)
		srv.ReadQueued(t, srv.Client)
		srv.Client.Send(encodeJoinParty(m.name))
		srv.ReadQueued(t, c)
		c.Send(encodeAnswerJoinParty(1))
		srv.ReadQueued(t, c)
		srv.ReadQueued(t, srv.Client)
		members[m.id] = c
	}
	for _, c := range members {
		srv.ReadQueued(t, c)
	}

	x, y, z := srv.PlayerPosition(t, leader)
	footman := srv.SpawnHostileNPCTemplateAt(t, proofMonster(footmanID), location.Location{X: x + 40, Y: y, Z: z})
	killOnPlayerQueue(t, srv, leader, footman)

	names := map[int32]string{leader: "Leader", mate: "Mate", third: "Third"}
	for id, w := range map[int32]struct {
		standing int32
		items    map[int32]int
	}{
		leader: {0, map[int32]int{7215: 0, 7214: 1}},
		mate:   {0, map[int32]int{7212: 0, 7211: 1}},
		third:  {-3, map[int32]int{7223: 1, 7222: 0}},
	} {
		for itemID, n := range w.items {
			if got := srv.PlayerItemCount(t, id, itemID); got != n {
				t.Errorf("%s's item %d = %d, want %d", names[id], itemID, got, n)
			}
		}
		var standing int32
		if !srv.RunScript(t, id, "KetraOrcSupport", func(_ *script.Script, p *script.Player) { standing = p.VarkaKetraAlliance() }) {
			t.Fatal("standing read panicked")
		}
		if standing != w.standing {
			t.Errorf("%s's standing = %d, want %d", names[id], standing, w.standing)
		}
	}
}
