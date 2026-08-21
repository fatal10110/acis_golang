//go:build integration

package network

import (
	"bytes"
	"testing"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestGameClientLinkRequestPledgeCrestDispatch(t *testing.T) {
	data := bytes.Repeat([]byte{0x5a}, 256)
	c, _, _, _, _, _ := newLinkedSQLGameClientFull(t, nil, nil, testCrestCache(t, map[int][]byte{101: data}), modelskill.BookPolicy{}, nil, true, nil, 0)

	c.send(encodeRequestPledgeCrest(101))

	reply := c.read()
	assertPledgeCrestFrame(t, reply, 101, data)
}

func TestGameClientLinkRequestPledgeCrestDispatchMissingData(t *testing.T) {
	c, _, _, _, _, _ := newLinkedSQLGameClientFull(t, nil, nil, datacache.NewCrests(), modelskill.BookPolicy{}, nil, true, nil, 0)

	c.send(encodeRequestPledgeCrest(999))

	reply := c.read()
	assertPledgeCrestFrame(t, reply, 999, nil)
}

func TestGameClientLinkRequestAllyCrestDispatch(t *testing.T) {
	data := bytes.Repeat([]byte{0x7b}, 192)
	c := newInGameSQLClientWithCrests(t, testAllyCrestCache(t, map[int][]byte{103: data}))

	c.send(encodeRequestAllyCrest(103))

	reply := c.read()
	assertAllyCrestFrame(t, reply, 103, data)
}

func TestGameClientLinkRequestAllyCrestDispatchMissingData(t *testing.T) {
	c := newInGameSQLClientWithCrests(t, datacache.NewCrests())

	c.send(encodeRequestAllyCrest(999))

	assertNoReply(t, c)
}

func TestGameClientLinkRequestExPledgeCrestLargeDispatch(t *testing.T) {
	data := bytes.Repeat([]byte{0x6c}, 2176)
	c := newInGameSQLClientWithCrests(t, testLargePledgeCrestCache(t, map[int][]byte{105: data}))

	c.send(encodeRequestExPledgeCrestLarge(105))

	reply := c.read()
	assertExPledgeCrestLargeFrame(t, reply, 105, data)
}

func TestGameClientLinkRequestExPledgeCrestLargeDispatchMissingData(t *testing.T) {
	c := newInGameSQLClientWithCrests(t, datacache.NewCrests())

	c.send(encodeRequestExPledgeCrestLarge(999))

	assertNoReply(t, c)
}

func newInGameSQLClientWithCrests(t *testing.T, crests *datacache.Crests) *fakeGameClient {
	t.Helper()
	c, _, _, _, _, _ := newLinkedSQLGameClientFull(t, nil, nil, crests, modelskill.BookPolicy{}, nil, true, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		seedSelectableSQLCharacter(t, chars, "player1", "CrestTester", 1, 0)
	}, 1)
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	return c
}
