//go:build integration

package network

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
)

func newLinkedSQLGameClientWithShortcuts(t *testing.T) (*fakeGameClient, *gamesql.CharacterStore, *gamesql.ShortcutStore, *gamesql.CharacterSkillStore) {
	t.Helper()

	db := sqltest.NewDB(t)
	chars := gamesql.NewCharacterStore(db)
	items := gamesql.NewItemStore(db)
	shortcuts := gamesql.NewShortcutStore(db)
	knownSkills := gamesql.NewCharacterSkillStore(db)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), skill.NewTable([]skill.Definition{{ID: 248, Level: 3}}), knownSkills)
	loginAddr, servers, sessions := newTestLoginServer(t, false)
	servers.Register(1, testHexID)
	validator := NewSessionValidator()
	loginLink, err := DialLoginLink(context.Background(), loginAddr, LoginServerAuth{ServerID: 1, HexID: testHexID, HostName: "*", Port: 7777, MaxPlayers: 300}, LoginLinkHandlers{PlayerAuthResponse: validator.Resolve}, zerolog.Nop())
	if err != nil {
		t.Fatalf("DialLoginLink: %v", err)
	}
	t.Cleanup(func() { loginLink.Close() })
	state := world.New()
	templates := testTemplates(t)
	itemTemplates := testItemTemplates()
	ids := &sequentialIDs{next: 100}
	inventoryUpdates := task.NewInventoryUpdates()
	roster := gamemanager.NewRoster(chars, items, shortcuts, templates, itemTemplates, npc.NewTable(nil), ids, gamemanager.DefaultDeleteAfter, time.Now)
	gcl := NewGameClientLink(GameClientLinkConfig{
		Validator:        validator,
		LoginLink:        func() *LoginLink { return loginLink },
		Roster:           roster,
		Items:            items,
		Shortcuts:        shortcuts,
		Templates:        templates,
		ItemTemplates:    itemTemplates,
		HTML:             testHTMLCache(t, map[string]string{"help/tutorial.htm": "<html><body>tutorial</body></html>"}),
		Crests:           datacache.NewCrests(),
		Skills:           skills,
		World:            state,
		Geo:              testGeo{},
		IDs:              ids,
		GroundItems:      task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour, PlayerDroppedMultiplier: 1}, time.Now),
		Positions:        task.NewPositionUpdates(state),
		InventoryUpdates: inventoryUpdates,
		PlayerConfig:     PlayerConfig{RespawnRestoreHP: 0.7, SkillEnchantSPBookNeeded: true, KarmaPlayerCanTeleport: true, AllowWater: true},
		PetConfig:        petmodel.DefaultConfig(),
		Log:              zerolog.Nop(),
	})
	registerTestInventoryUpdates(t, state, inventoryUpdates)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var handlers struct {
		sync.Mutex
		count int
	}
	handlersDone := sync.NewCond(&handlers.Mutex)
	t.Cleanup(func() {
		cancel()
		ln.Close()
		handlers.Lock()
		defer handlers.Unlock()
		for handlers.count > 0 {
			handlersDone.Wait()
		}
	})
	go Serve(ctx, ln, func(ctx context.Context, conn *Conn) {
		handlers.Lock()
		handlers.count++
		handlers.Unlock()
		defer func() {
			handlers.Lock()
			handlers.count--
			handlersDone.Broadcast()
			handlers.Unlock()
		}()
		gcl.Handle(ctx, conn)
	}, zerolog.Nop())

	c := dialGameClient(t, ln.Addr().String())
	c.sendProtocolVersion(746)
	key := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}
	sessions.Put("player1", key)
	c.send(encodeAuthLogin("player1", key))
	if reply := c.read(); reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
	return c, chars, shortcuts, knownSkills
}
