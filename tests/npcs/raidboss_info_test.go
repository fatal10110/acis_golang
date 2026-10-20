package npcs

import (
	"bytes"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/raidbossinfo"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The raid boss radar of the raid information officers (RaidbossInfo),
// which a scenario cannot compare: the reference server holds the
// datapack's spawns, the scenario world none.

const (
	raidOfficerID = 31729
	// The level 20 page's first three raid bosses.
	greyclawKutus = 25001
	panDryad      = 25019
	tirak         = 25038
	raidInfoDir   = "script/feature/RaidbossInfo/"
)

// radarBossTemplate is a fixture raid boss template of npcID.
func radarBossTemplate(npcID int) *npc.Template {
	return &npc.Template{
		ID: npcID, TemplateID: npcID, Type: "RaidBoss", Name: "Raid", Level: 23,
		HPMax: 1000, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
}

// radarMarkerFrames are the frames marking x, y, z on the radar: the
// markers cleared, then the new one shown.
func radarMarkerFrames(x, y, z int32) [][]byte {
	var out [][]byte
	for _, r := range []serverpackets.Radar{{Show: 2, Type: 2, X: x, Y: y, Z: z}, {Show: 0, Type: 1, X: x, Y: y, Z: z}} {
		out = append(out, bytes.Clone(frameBody(serverpackets.FrameRadarControl(r))))
	}
	return out
}

// wantRadar requires frames to hold, of the dialog and radar packets, only
// want's RadarControl frames, in order.
func wantRadar(t *testing.T, frames, want [][]byte) {
	t.Helper()
	var got [][]byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeRadarControl, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeSystemMessage:
			got = append(got, f)
		}
	}
	if !slices.EqualFunc(got, want, bytes.Equal) {
		t.Fatalf("answer = %x, want %x", got, want)
	}
}

// TestRaidbossInfoRadar: an Adventurer Guildsman's link opens the raid
// levels, and the level 20 page's links mark a raid boss on the radar:
// Greyclaw Kutus at its spawn's point, not where it stands now; Pan Dryad,
// which no spawn places, where a live one stands; Tirak, with neither,
// marks nothing. No pick answers a page.
func TestRaidbossInfoRadar(t *testing.T) {
	t.Parallel()
	pages := featurePages(t, "RaidbossInfo")
	pages["test/any.htm"] = anyNpcPage
	officer := folkTemplate("Adventurer", raidOfficerID)
	kutus, dryad := radarBossTemplate(greyclawKutus), radarBossTemplate(panDryad)
	w := bootFolkWorld(t, pages,
		noBypassReuse,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{officer, kutus, dryad})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{raidOfficerID: script.KindFolk},
			[]script.Listing{{Path: "script.feature.RaidbossInfo"}},
			script.Catalog{"script.feature.RaidbossInfo": raidbossinfo.New}))
	guild := w.spawnFolk(t, officer, 40)

	if _, err := w.srv.NpcSpawns.SpawnFixedHostile(kutus, w.at.X+600, w.at.Y+300, w.at.Z, 0); err != nil {
		t.Fatal(err)
	}
	w.srv.RunScript(t, w.player, "RaidbossInfo", func(s *script.Script, _ *script.Player) {
		s.WorldNPC(greyclawKutus).SetXYZ(script.Loc{X: w.at.X + 900, Y: w.at.Y - 200, Z: w.at.Z})
	})
	live := location.Location{X: w.at.X - 700, Y: w.at.Y + 400, Z: w.at.Z}
	w.srv.SpawnHostileNPCTemplateAt(t, dryad, live)
	drainUntilQuiet(t, w.c)

	w.openAnyNpcPage(t)
	wantPage(t, w.bypass(t, npcCommand(guild, "Quest RaidbossInfo")), guild, scriptPage(t, raidInfoDir+"info.htm", guild), 2)
	wantPage(t, w.bypass(t, "Quest RaidbossInfo level20.htm"), guild, scriptPage(t, raidInfoDir+"level20.htm", guild), 1)

	wantRadar(t, w.bypass(t, "Quest RaidbossInfo 25001"), radarMarkerFrames(int32(w.at.X+600), int32(w.at.Y+300), int32(w.at.Z)))
	wantRadar(t, w.bypass(t, "Quest RaidbossInfo 25019"), radarMarkerFrames(int32(live.X), int32(live.Y), int32(live.Z)))
	wantRadar(t, w.bypass(t, "Quest RaidbossInfo 25038"), nil)
}
