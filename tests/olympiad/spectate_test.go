package olympiad

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The spectating scenarios add a second, idle stadium west of the class
// spawn; each stadium's third normal spawn is where its spectators watch.
var (
	spectatorSpot1 = location.Location{X: 5930, Y: 0, Z: gameservertest.SpawnZ}
	spectatorSpot2 = location.Location{X: -5930, Y: 0, Z: gameservertest.SpawnZ}
)

// smWaitingListCannotWatch refuses to let a registered noble watch.
const smWaitingListCannotWatch = 1693

// spectateStadiums are the match's stadium, stadium 0, then a second,
// stadium 1, both with a spectator spawn.
func spectateStadiums(t *testing.T) *zone.Index {
	t.Helper()
	ix := matchStadium(t)
	form, err := zone.NewCuboid(-7000, -5000, -1000, 1000, -1000, 1000)
	if err != nil {
		t.Fatalf("second stadium form: %v", err)
	}
	second := zone.NewOlympiad(2, form)
	second.AddSpawn(zone.SpawnNormal, location.Location{X: -5900, Y: 0, Z: gameservertest.SpawnZ})
	second.AddSpawn(zone.SpawnNormal, location.Location{X: -5960, Y: 0, Z: gameservertest.SpawnZ})
	second.AddSpawn(zone.SpawnNormal, spectatorSpot2)
	ix.Add(second)
	return ix
}

// spectatePages are the shipped pages a spectator goes through: the
// manager's page to a non-noble and both stadium lists.
func spectatePages(t *testing.T) map[string]string {
	t.Helper()
	pages := map[string]string{}
	for _, name := range []string{"noble.htm", "olympiad_observe_list.htm", "olympiad_arena_observe_list.htm"} {
		raw, err := os.ReadFile(datapack.Path(t, "data", "html", "olympiad", name))
		if err != nil {
			t.Fatal(err)
		}
		pages["olympiad/"+name] = string(raw)
	}
	return pages
}

// spectate is a running match with Charlie, neither noble nor registered,
// online beside a Grand Olympiad Manager.
type spectate struct {
	*match
	watcher *competitor
	manager *npc.Folk
	pages   map[string]string
}

func bootSpectate(t *testing.T) *spectate {
	t.Helper()
	pages := spectatePages(t)
	m := bootMatch(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 120_000 },
		gameservertest.WithZones(spectateStadiums(t)),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithReuseDelays(3*time.Second, 0),
	)
	id := m.srv.SeedCharacterFor(t, "charlie", "Charlie", 40, 0).ID
	w := &competitor{c: m.srv.DialClient(t, "charlie", 1), id: id, name: "Charlie"}
	startInWorld(t, w.c)
	s := &spectate{match: m, watcher: w, pages: pages}
	home := s.char(t, w).CurrentLocation()
	s.manager = m.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("OlympiadManagerNpc", 31688), location.Location{X: home.X + 30, Y: home.Y, Z: home.Z})
	m.srv.ReadQueued(t, w.c)
	m.ones = append(m.ones, w)
	return s
}

// char is p's online character.
func (s *spectate) char(t *testing.T, p *competitor) *player.Character {
	t.Helper()
	obj, ok := s.srv.State.Player(p.id)
	if !ok {
		t.Fatalf("%s not in world", p.name)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%s is %T, not an online character", p.name, obj)
	}
	return c
}

// send has p send payload and returns what p was sent for it, its own
// teleports answered with Appearing.
func (s *spectate) send(t *testing.T, p *competitor, payload []byte) [][]byte {
	t.Helper()
	from := len(p.frames)
	p.c.Send(payload)
	s.srv.Advance(t, 50*time.Millisecond)
	s.read(t)
	s.srv.Advance(t, 50*time.Millisecond)
	s.read(t)
	return p.frames[from:]
}

// atManager has p select the manager, then talk to it.
func (s *spectate) atManager(t *testing.T, p *competitor) {
	t.Helper()
	at := s.char(t, p).CurrentLocation()
	s.send(t, p, encodeSelect(s.manager.ObjectID(), at))
	s.send(t, p, encodeSelect(s.manager.ObjectID(), at))
}

func encodeSelect(objectID int32, at location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteUint8(0)
	return w.Bytes()
}

func encodeExRequest(sub uint16) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(sub)
	return w.Bytes()
}

// spectatorLines reduces frames to what spectating shows: its system
// messages, Olympiad packets, own teleports, pages and walks of id.
func spectatorLines(id int32, frames [][]byte) []string {
	var out []string
	for _, f := range frames {
		r := wire.NewReader(f[1:])
		switch f[0] {
		case serverpackets.OpcodeSystemMessage:
			out = append(out, "SM "+strconv.Itoa(int(r.ReadInt32())))
		case serverpackets.OpcodeTeleportToLocation:
			if r.ReadInt32() == id {
				out = append(out, fmt.Sprintf("Teleport %d %d", r.ReadInt32(), r.ReadInt32()))
			}
		case serverpackets.OpcodeMoveToLocation:
			if r.ReadInt32() == id {
				out = append(out, "Move")
			}
		case serverpackets.OpcodeNpcHtmlMessage:
			out = append(out, fmt.Sprintf("Html %d", r.ReadInt32()))
		case serverpackets.OpcodeExtended:
			switch r.ReadUint16() {
			case serverpackets.OpcodeExOlympiadMode:
				out = append(out, fmt.Sprintf("Mode %d", r.ReadUint8()))
			case serverpackets.OpcodeExOlympiadMatchEnd:
				out = append(out, "MatchEnd")
			case serverpackets.OpcodeExOlympiadUserInfo:
				side := r.ReadUint8()
				out = append(out, fmt.Sprintf("UserInfo %d %d", side, r.ReadInt32()))
			case serverpackets.OpcodeExOlympiadSpelledInfo:
				out = append(out, fmt.Sprintf("Spelled %d", r.ReadInt32()))
			}
		}
	}
	return out
}

// htmlOf is the one page among frames: its object and its text.
func htmlOf(t *testing.T, frames [][]byte) (int32, string) {
	t.Helper()
	var pages [][]byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeNpcHtmlMessage {
			pages = append(pages, f)
		}
	}
	if len(pages) != 1 {
		t.Fatalf("%d pages sent, want 1", len(pages))
	}
	r := wire.NewReader(pages[0][1:])
	return r.ReadInt32(), r.ReadString()
}

// wantPage is the shipped page name as the page cache holds it, each pair
// of fill replaced.
func (s *spectate) wantPage(t *testing.T, name string, fill ...string) string {
	t.Helper()
	key := "olympiad/" + name
	page, _ := gameservertest.HTMLCache(t, map[string]string{key: s.pages[key]}).Get(key)
	for i := 0; i+1 < len(fill); i += 2 {
		page = strings.ReplaceAll(page, fill[i], fill[i+1])
	}
	return page
}

// countPrefix counts the lines starting with prefix.
func countPrefix(lines []string, prefix string) int {
	n := 0
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

// subsequence reports whether want appears in got in order.
func subsequence(got, want []string) bool {
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	return i == len(want)
}

// TestOlympiadSpectateWatchSwitchAndLeave pins watching a stadium
// (OlympiadManagerNpc "Olympiad 3", RequestBypassToServer arenachange,
// Player.enterOlympiadObserverMode, OlympiadStadiumZone.onEnter,
// EffectList.updateEffectIcons, RequestOlympiadMatchList,
// RequestOlympiadObserverEnd, Player.leaveOlympiadObserverMode): the
// manager lists the stadiums, the fighting one with its competitors; a
// link lands the player hidden and unhurtable on the stadium's spectator
// spawn in spectating mode, not expelled, shown the competitors' gauges
// and their effect icons; a spectator walks, lists the stadiums itself,
// jumps to another stadium keeping where it left, and leaves back there.
func TestOlympiadSpectateWatchSwitchAndLeave(t *testing.T) {
	t.Parallel()
	s := bootSpectate(t)
	a, b, w := s.ones[0], s.ones[1], s.watcher
	s.shown(t)

	s.atManager(t, w)
	mgr := strconv.Itoa(int(s.manager.ObjectID()))
	objectID, html := htmlOf(t, s.send(t, w, encodeHeroBypass("npc_"+mgr+"_Olympiad 3")))
	wantList := `<a action="bypass arenachange 0">Arena 1&nbsp;(&$829;)&nbsp;Alpha&nbsp; : &nbsp;Bravo</a><br>` +
		`<a action="bypass arenachange 1">Arena 2&nbsp;(&$906;)</td><td>&nbsp;</a><br>`
	if want := s.wantPage(t, "olympiad_observe_list.htm", "%list%", wantList, "%objectId%", mgr); objectID != s.manager.ObjectID() || html != want {
		t.Fatalf("manager stadium list from %d =\n%s\nwant from %d\n%s", objectID, html, s.manager.ObjectID(), want)
	}

	c := s.char(t, w)
	left := c.CurrentLocation()
	got := spectatorLines(w.id, s.send(t, w, encodeHeroBypass("arenachange 0")))
	enter := []string{
		fmt.Sprintf("Teleport %d %d", spectatorSpot1.X, spectatorSpot1.Y), "Mode 3", "SM 283",
		fmt.Sprintf("UserInfo 1 %d", a.id), fmt.Sprintf("UserInfo 2 %d", b.id),
	}
	if !subsequence(got, enter) || !slices.Contains(got, fmt.Sprintf("Spelled %d", a.id)) || !slices.Contains(got, fmt.Sprintf("Spelled %d", b.id)) {
		t.Fatalf("Charlie's entry = %q, want %q in order and both competitors' effect icons", got, enter)
	}
	if n := countPrefix(got, "Teleport"); n != 1 {
		t.Fatalf("Charlie teleported %d times on its entry, want once (not expelled): %q", n, got)
	}
	if saved, ok := c.SavedLocation(); !c.ObserverMode() || !c.Invisible() || !c.Invul() || c.Paralyzed() || c.OlympiadGameID() != 0 || !ok || saved != left {
		t.Fatalf("watching: observer=%v invisible=%v invul=%v paralyzed=%v game=%d saved=%+v/%v, want watching stadium 0 from %+v",
			c.ObserverMode(), c.Invisible(), c.Invul(), c.Paralyzed(), c.OlympiadGameID(), saved, ok, left)
	}
	for _, p := range []*competitor{a, b} {
		if slices.ContainsFunc(spectatorLines(w.id, p.frames), func(l string) bool { return strings.HasPrefix(l, "Spelled") }) {
			t.Fatalf("competitor %s was sent a spectator's effect icons", p.name)
		}
	}

	// A spectator is not held: once the stand-up of its entry is over, it
	// walks.
	s.srv.Advance(t, 3*time.Second)
	s.read(t)
	at := c.CurrentLocation()
	if got := spectatorLines(w.id, s.send(t, w, encodeMove(at.X+40, at.Y, at.Z, at.X, at.Y, at.Z))); !slices.Contains(got, "Move") {
		t.Fatalf("spectator's walk answered %q, want its MoveToLocation", got)
	}

	objectID, html = htmlOf(t, s.send(t, w, encodeExRequest(clientpackets.OpcodeRequestOlympiadMatchList)))
	wantRows := `<tr><td fixwidth=10><a action="bypass arenachange 0">1</a></td><td fixwidth=80>&$829;</td><td>Alpha&nbsp; / &nbsp;Bravo</td><td><font color="aaccff"></font></td></tr>` +
		`<tr><td fixwidth=10><a action="bypass arenachange 1">2</a></td><td fixwidth=80>&$906;</td><td>&nbsp;</td><td><font color="aaccff"></font></td></tr>`
	if want := s.wantPage(t, "olympiad_arena_observe_list.htm", "%list%", wantRows); objectID != 0 || html != want {
		t.Fatalf("spectator stadium list from %d =\n%s\nwant from 0\n%s", objectID, html, want)
	}

	got = spectatorLines(w.id, s.send(t, w, encodeHeroBypass("arenachange 1")))
	jump := []string{fmt.Sprintf("Teleport %d %d", spectatorSpot2.X, spectatorSpot2.Y), "Mode 3"}
	if !subsequence(got, jump) || !subsequence(got, []string{"SM 284", "MatchEnd"}) {
		t.Fatalf("Charlie's jump = %q, want %q and the fighting stadium left", got, jump)
	}
	if saved, _ := c.SavedLocation(); !c.ObserverMode() || c.OlympiadGameID() != 1 || saved != left {
		t.Fatalf("after the jump: observer=%v game=%d saved=%+v, want watching stadium 1 from %+v", c.ObserverMode(), c.OlympiadGameID(), saved, left)
	}

	got = spectatorLines(w.id, s.send(t, w, encodeExRequest(clientpackets.OpcodeRequestOlympiadObserverEnd)))
	leave := []string{"Mode 0", fmt.Sprintf("Teleport %d %d", left.X, left.Y)}
	if !subsequence(got, leave) {
		t.Fatalf("Charlie's leave = %q, want %q", got, leave)
	}
	if _, ok := c.SavedLocation(); c.ObserverMode() || c.Invisible() || c.Invul() || c.OlympiadGameID() != -1 || ok {
		t.Fatalf("after leaving: observer=%v invisible=%v invul=%v game=%d saved=%v, want none",
			c.ObserverMode(), c.Invisible(), c.Invul(), c.OlympiadGameID(), ok)
	}
	if at := c.CurrentLocation(); at.X != left.X || at.Y != left.Y {
		t.Fatalf("after leaving Charlie stands at %+v, want back at %+v", at, left)
	}
}

// TestOlympiadSpectateRefusals pins who may not watch (RequestBypassToServer
// arenachange, RequestOlympiadObserverEnd, RequestOlympiadMatchList): a
// registered noble at the manager is told it may not watch from the
// waiting list; a player neither at the manager nor watching, a stadium
// that is not and a number that does not parse answer nothing, and so do
// leaving and listing from a player who watches nothing.
func TestOlympiadSpectateRefusals(t *testing.T) {
	t.Parallel()
	s := bootSpectate(t)
	a, w := s.ones[0], s.watcher

	s.atManager(t, a)
	if got := spectatorLines(a.id, s.send(t, a, encodeHeroBypass("arenachange 0"))); !slices.Equal(got, []string{"SM " + strconv.Itoa(smWaitingListCannotWatch)}) {
		t.Fatalf("registered noble's arenachange = %q, want only the waiting-list refusal", got)
	}

	for _, payload := range [][]byte{
		encodeHeroBypass("arenachange 0"),
		encodeExRequest(clientpackets.OpcodeRequestOlympiadObserverEnd),
		encodeExRequest(clientpackets.OpcodeRequestOlympiadMatchList),
	} {
		if got := s.send(t, w, payload); len(got) != 0 {
			t.Fatalf("Charlie, away from the manager, answered %d frames, want none: %q", len(got), spectatorLines(w.id, got))
		}
	}

	s.atManager(t, w)
	for _, command := range []string{"arenachange 2", "arenachange -1", "arenachange x", "arenachange"} {
		if got := s.send(t, w, encodeHeroBypass(command)); len(got) != 0 {
			t.Fatalf("%q answered %q, want nothing", command, spectatorLines(w.id, got))
		}
	}
	if c := s.char(t, w); c.ObserverMode() || c.Invisible() || c.OlympiadGameID() != -1 {
		t.Fatalf("after the refusals: observer=%v invisible=%v game=%d, want not watching", c.ObserverMode(), c.Invisible(), c.OlympiadGameID())
	}
}
