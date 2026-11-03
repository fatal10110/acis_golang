package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// oracleCharacterCases are the packets that describe a character, an npc or a
// summon to the client (testdata/oracle/ProbeCharacters.java). Every field is
// a function of the shared row v, through rowField (the probe's f) and the
// shared doubles, so the Java and Go inputs are the same values.
func oracleCharacterCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	addSummonCases(m)
	addNpcCases(m)
	addPlayerCases(m)
	return m
}

// rowField is ProbeCharacters.f: field i of a row, the index wrapping.
func rowField(v [10]int32, i int) int32 { return v[i%10] }

// rowFloat is ProbeCharacters.fl: a shared double narrowed to the float the
// status getters return, widened back.
func rowFloat(r, k int) float64 { return float64(float32(oracleDoubles[(r+k)%len(oracleDoubles)])) }

// rowDouble is ProbeCharacters.dbl.
func rowDouble(r, k int) float64 { return oracleDoubles[(r+k)%len(oracleDoubles)] }

// oracleSummon is ProbeCharacters.summonBase: the snapshot the probe's summon
// reads as, for a pet or a servitor.
func oracleSummon(r int, v [10]int32) PetInfoSnapshot {
	f := func(i int) int { return int(rowField(v, i)) }
	return PetInfoSnapshot{
		SummonType: f(0), ObjectID: rowField(v, 1), TemplateID: f(2),
		X: f(3), Y: f(4), Z: f(5), Heading: f(6),
		MAtkSpd: f(7), PAtkSpd: f(8), RunSpd: f(9), WalkSpd: f(0) ^ f(5),
		MoveMultiplier: rowFloat(r, 0), AtkSpdMultiplier: rowFloat(r, 1),
		CollisionRadius: rowDouble(r, 2), CollisionHeight: rowDouble(r, 3),
		InCombat: f(1)&1 != 0, AlikeDead: f(2)&1 != 0,
		Name: oracleStrings[r].s, Title: oracleStrings[(r+1)%len(oracleStrings)].s,
		PvpFlag: abs(f(4) % 3), Karma: f(5),
		CurHP: f(6), MaxHP: f(7), CurMP: f(8), MaxMP: f(9),
		SP: f(0), Level: f(1), Exp: int64(f(2))*1000003 + int64(f(3)),
		ExpForThisLevel: int64(f(4))*1000003 + int64(f(5)), ExpForNextLevel: int64(f(6))*1000003 + int64(f(7)),
		WeightLimit: f(8),
		PAtk:        f(9), PDef: f(0) ^ f(1), MAtk: f(2) ^ f(3), MDef: f(4) ^ f(5),
		Accuracy: f(6) ^ f(7), EvasionRate: f(8) ^ f(9), CriticalHit: f(0) ^ f(9),
		MoveSpeed:      f(3) & 0x7fff,
		AbnormalEffect: f(5), Mountable: f(6)&1 != 0,
		MoveType: r % 3, Team: r % 3,
		SoulShotsPerHit: f(7), SpiritShotsPerHit: f(8),
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func addSummonCases(m map[string]func() wire.Frame) {
	for r, row := range oracleRows {
		v, n := row.v, "/"+row.name
		pet := func() PetInfoSnapshot {
			s := oracleSummon(r, v)
			s.CurFed, s.MaxFed, s.TotalWeight = int(rowField(v, 4)), int(rowField(v, 5)), int(rowField(v, 6))
			return s
		}
		servitor := func() PetInfoSnapshot {
			s := oracleSummon(r, v)
			s.CurFed, s.MaxFed = int(rowField(v, 4)), int(rowField(v, 5))
			return s
		}
		m["PetInfo/pet"+n] = func() wire.Frame { return FramePetInfo(pet()) }
		m["PetStatusUpdate/pet"+n] = func() wire.Frame { return FramePetStatusUpdate(pet()) }
		m["PetInfo/servitor"+n] = func() wire.Frame { return FramePetInfo(servitor()) }
		m["PetStatusUpdate/servitor"+n] = func() wire.Frame { return FramePetStatusUpdate(servitor()) }
		m["PetInfo/pet-worn"+n] = func() wire.Frame {
			s := pet()
			s.Weapon, s.Armor = int(rowField(v, 7)^rowField(v, 1)), int(rowField(v, 8)^rowField(v, 2))
			return FramePetInfo(s)
		}

		// SummonInfo is what everyone but the owner is shown of a summon.
		summonInfo := func(weapon, armor, animation int, title string) NPCInfoSnapshot {
			f := func(i int) int { return int(rowField(v, i)) }
			return NPCInfoSnapshot{
				ObjectID: rowField(v, 1), TemplateID: f(2), Attackable: f(9)&1 != 0,
				X: f(3), Y: f(4), Z: f(5), Heading: f(6),
				MAtkSpd: f(7), PAtkSpd: f(8), RunSpd: f(9), WalkSpd: f(0) ^ f(5),
				MoveMultiplier: rowFloat(r, 0), AtkSpdMultiplier: rowFloat(r, 1),
				CollisionRadius: rowDouble(r, 2), CollisionHeight: rowDouble(r, 3),
				RightHand: weapon, Chest: armor,
				Running: f(3)&1 != 0, InCombat: f(1)&1 != 0, AlikeDead: f(2)&1 != 0,
				SummonAnimation: animation, Title: title, Summon: true,
				PvpFlag: abs(f(4) % 3), Karma: f(5), AbnormalEffect: f(5),
				MoveType: r % 3, Team: r % 3,
			}
		}
		weapon, armor := int(rowField(v, 7)^rowField(v, 1)), int(rowField(v, 8)^rowField(v, 2))
		m["SummonInfo/pet"+n] = func() wire.Frame { return FrameNPCInfo(summonInfo(weapon, armor, 2, "Master")) }
		m["SummonInfo/pet-offline"+n] = func() wire.Frame { return FrameNPCInfo(summonInfo(weapon, armor, 2, "")) }
		m["SummonInfo/pet-val"+n] = func() wire.Frame {
			return FrameNPCInfo(summonInfo(weapon, armor, int(rowField(v, 3)&3), "Master"))
		}
		m["SummonInfo/servitor"+n] = func() wire.Frame { return FrameNPCInfo(summonInfo(0, 0, 2, "Master")) }
	}
}

// oracleNpc is ProbeCharacters.npc: the snapshot the probe's npc of row r
// reads as, attackable as the row's f(9) says.
func oracleNpc(r int, v [10]int32) npcinfo.Snapshot {
	f := func(i int) int { return int(rowField(v, i)) }
	return npcinfo.Snapshot{
		ObjectID: rowField(v, 1), TemplateID: f(2), Attackable: f(9)&1 != 0,
		X: f(3), Y: f(4), Z: f(5), Heading: f(6),
		MAtkSpd: f(7), PAtkSpd: f(8), RunSpd: f(9), WalkSpd: f(0) ^ f(5),
		MoveMultiplier: rowFloat(r, 0), AtkSpdMultiplier: rowFloat(r, 1),
		CollisionRadius: rowDouble(r, 2), CollisionHeight: rowDouble(r, 3),
		RightHand: f(4) ^ f(5), LeftHand: f(6) ^ f(7),
		Running: f(3)&1 != 0, InCombat: f(1)&1 != 0, AlikeDead: f(2)&1 != 0,
		SummonAnimation: 2,
		Name:            oracleStrings[r].s, Title: oracleStrings[(r+1)%len(oracleStrings)].s,
		AbnormalEffect: f(5), MoveType: r % 3, EnchantEffect: f(6), Flying: f(7)&1 != 0,
	}
}

func addNpcCases(m map[string]func() wire.Frame) {
	for r, row := range oracleRows {
		v, n := row.v, "/"+row.name
		f := func(i int) int { return int(rowField(v, i)) }
		snapshot := func(attackable bool) npcinfo.Snapshot {
			s := oracleNpc(r, v)
			s.Attackable = attackable
			return s
		}
		m["NpcInfo"+n] = func() wire.Frame { return FrameNPCInfo(oracleNpc(r, v)) }
		server := func(attackable bool) NPCInfoSnapshot {
			s := snapshot(attackable)
			s.CurrentHP, s.MaxHP = f(8), f(9)
			return s
		}
		m["ServerObjectInfo/attackable"+n] = func() wire.Frame { return FrameServerObjectInfo(server(true)) }
		m["ServerObjectInfo/peaceful"+n] = func() wire.Frame { return FrameServerObjectInfo(server(false)) }
	}
}

// oraclePlayer is ProbeCharacters.player: the character of row r and what
// the snapshots carry besides it.
type oraclePlayer struct {
	c        *player.Character
	tmpl     *player.Template
	items    []*item.Instance
	clan     ClanFields
	gm       bool
	heroAura bool
	// spawnProtected shows the blue team; hidden draws the character invisible
	// to the viewer; revealed shows an invisible character to a game master.
	spawnProtected, hidden, revealed bool
}

// oracleLive* are the values the live calculators answer for the template and
// base resources newOraclePlayer(0, ..).live gives a character; the probe
// (ProbeCharacters.live) holds the same numbers, and
// TestOracleCharacterInputsPinned fails when a calculator moves them.
var oracleLiveTemplate = player.Template{
	STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25,
	PAtk: 4, PDef: 30, MAtk: 3, MDef: 15,
	RunSpeed: 120, WalkSpeed: 80, SwimSpeed: 50,
}

func newOraclePlayer(r int, v [10]int32, live bool) *oraclePlayer {
	f := func(i int) int { return int(rowField(v, i)) }
	c := &player.Character{
		ID: v[1], Race: player.Race(r), CharLevel: 40,
		Exp: int64(f(2))*1000003 + int64(f(3)), SP: f(8),
		Location: loc(rowField(v, 3), rowField(v, 4), rowField(v, 5)), LastHeading: f(6),
		KarmaPoints: f(5), PKKills: f(6), PvPKills: f(7),
		Face: f(6), HairStyle: f(7), HairColor: f(8),
	}
	c.SetName(oracleStrings[r].s)
	c.SetTitle(oracleStrings[(r+1)%len(oracleStrings)].s)
	c.SetClanID(rowField(v, 2))
	c.SetSex(player.Sex(f(2) & 1))
	c.SetClassID(oracleClassIDs[r])
	c.SetColors(rowField(v, 9), rowField(v, 0)^rowField(v, 1))
	c.SetPledgeClass(f(8))
	c.SetRunning(false) // a character starts running; the probe's starts walking
	c.SetRecommendationCounts((f(6)&0x7fffffff)%256, (f(5)&0x7fffffff)%10)

	tmpl := &player.Template{}
	if live {
		*tmpl = oracleLiveTemplate
	}
	tmpl.CollisionRadius, tmpl.CollisionHeight = rowDouble(r, 2), rowDouble(r, 3)
	tmpl.CollisionRadiusFemale, tmpl.CollisionHeightFemale = rowDouble(r, 4), rowDouble(r, 5)

	maxHP, maxMP, maxCP := float64(f(7)), float64(f(9)), float64(f(0))
	if live {
		// The live maxima are the base values run through the calculators.
		maxHP, maxMP, maxCP = 777, 99, 55
	}
	c.SetResourceValues(player.Resources{
		MaxHP: maxHP, CurrentHP: float64(f(6)) + 0.5,
		MaxMP: maxMP, CurrentMP: float64(f(7)) + 0.25,
		MaxCP: maxCP, CurrentCP: float64(f(8)) + 0.75,
	})
	if live {
		c.AttachRuntime(tmpl, nil)
	}

	o := &oraclePlayer{c: c, tmpl: tmpl, clan: ClanFields{
		CrestID: rowField(v, 3), AllyID: rowField(v, 4), AllyCrestID: rowField(v, 5),
		CrestLargeID: rowField(v, 6), Privileges: rowField(v, 7), PledgeType: rowField(v, 9),
	}}
	enchant := (f(9) & 0x7fffffff) % 256
	for slot := 1; slot <= 16; slot++ {
		inst := &item.Instance{
			ObjectID: oracleSlotObjectID(v, slot), TemplateID: oracleSlotItemID(v, slot),
			Location: item.LocationPaperdoll, LocationData: slot,
		}
		switch slot {
		case rhandPaperdollIndex:
			inst.EnchantLevel = enchant
			inst.Augmentation = &item.Augmentation{Attributes: rowField(v, 8)}
		case lhandPaperdollIndex:
			inst.Augmentation = &item.Augmentation{Attributes: rowField(v, 9)}
		}
		o.items = append(o.items, inst)
	}
	return o
}

func (o *oraclePlayer) user() UserInfoSnapshot {
	return UserInfoSnapshot{
		Character: o.c, Template: o.tmpl, Items: o.items, Clan: o.clan,
		IsGM: o.gm, HeroAura: o.heroAura, SpawnProtectedTeam: o.spawnProtected,
	}
}

func (o *oraclePlayer) char() CharInfoSnapshot {
	return CharInfoSnapshot{
		Character: o.c, Template: o.tmpl, Items: o.items, Clan: o.clan,
		HeroAura: o.heroAura, SpawnProtectedTeam: o.spawnProtected,
		Hidden: o.hidden, Revealed: o.revealed,
	}
}

// invisible hides the character from a viewer who is no game master.
func (o *oraclePlayer) invisible() {
	o.c.SetInvisible(true)
	o.hidden = true
}

// oracleVessel is the boat a passenger rides: only its object id is read.
type oracleVessel int32

func (v oracleVessel) ObjectID() int32                 { return int32(v) }
func (v oracleVessel) OustLocation() location.Location { return location.Location{} }

// oracleVariants are ProbeCharacters.variants, in the same names: row "a"
// under one change at a time.
func oracleVariants() map[string]func(o *oraclePlayer) {
	a := oracleRows[0].v
	return map[string]func(o *oraclePlayer){
		"gm":           func(o *oraclePlayer) { o.gm = true },
		"gm-invisible": func(o *oraclePlayer) { o.gm = true; o.invisible() },
		"invisible":    func(o *oraclePlayer) { o.invisible() },
		"gm-hero-aura": func(o *oraclePlayer) { o.gm = true; o.heroAura = true },
		"hero":         func(o *oraclePlayer) { o.c.SetHero(true) },
		"noble":        func(o *oraclePlayer) { o.c.SetNoble(true) },
		"strider":      func(o *oraclePlayer) { o.c.Mount(12526, 0) },
		"wyvern":       func(o *oraclePlayer) { o.c.Mount(12621, 0) },
		"fishing": func(o *oraclePlayer) {
			o.c.SetFishing(true)
			o.c.SetFishingBait(loc(rowField(a, 3), rowField(a, 4), rowField(a, 5)))
		},
		"spawn-protected": func(o *oraclePlayer) { o.spawnProtected = true },
		"team-red":        func(o *oraclePlayer) { o.c.SetDuelTeam(duel.Team(2)) },
		"cubics": func(o *oraclePlayer) {
			o.c.SetSkillLevel(143, 5) // Cubic Mastery: room for more than one cubic
			o.c.AddOrRefreshCubic(1, false)
			o.c.AddOrRefreshCubic(3, false)
		},
		"party-room":     func(o *oraclePlayer) { o.c.SetPartyRoom(5) },
		"abnormal":       func(o *oraclePlayer) { o.c.StartAbnormalEffect(0x2040) },
		"operate-sell":   func(o *oraclePlayer) { o.c.SetOperateType(privatestore.OperatePackageSell) },
		"crystallize":    func(o *oraclePlayer) { o.c.SetSkillLevel(248, 1) },
		"clan-leader":    func(o *oraclePlayer) { o.clan.Leader = true },
		"siege-attacker": func(o *oraclePlayer) { o.c.SetSiegeState(player.SiegeStateAttacker) },
		"siege-defender": func(o *oraclePlayer) { o.c.SetSiegeState(player.SiegeStateDefender) },
		"cursed":         func(o *oraclePlayer) { o.c.SetCursedWeapon(8190, 2) },
		"pvp-flagged":    func(o *oraclePlayer) { o.c.UpdatePvPFlag(task.PvPFlagOn) },
		"pvp-blinking":   func(o *oraclePlayer) { o.c.UpdatePvPFlag(task.PvPFlagBlinking) },
		"running":        func(o *oraclePlayer) { o.c.SetRunning(true) },
		"sitting":        func(o *oraclePlayer) { o.c.SetStanding(false) },
		"combat":         func(o *oraclePlayer) { o.c.SetInCombat(true) },
		"dead":           func(o *oraclePlayer) { o.c.MarkDead() },
		"enchant-capped": func(o *oraclePlayer) { o.items[rhandPaperdollIndex-1].EnchantLevel = 200 },
		"boat":           func(o *oraclePlayer) { o.c.Board(oracleVessel(rowField(a, 4))) },
	}
}

func addPlayerCases(m map[string]func() wire.Frame) {
	for r, row := range oracleRows {
		v, n := row.v, "/"+row.name
		m["UserInfo"+n] = func() wire.Frame { return FrameUserInfo(newOraclePlayer(r, v, false).user()) }
		m["CharInfo"+n] = func() wire.Frame { return FrameCharInfo(newOraclePlayer(r, v, false).char()) }
	}
	a := oracleRows[0].v
	add := func(name string, live bool, apply func(o *oraclePlayer)) {
		build := func() *oraclePlayer {
			o := newOraclePlayer(0, a, live)
			apply(o)
			return o
		}
		m["UserInfo/"+name] = func() wire.Frame { return FrameUserInfo(build().user()) }
		m["CharInfo/"+name] = func() wire.Frame { return FrameCharInfo(build().char()) }
	}
	add("live", true, func(*oraclePlayer) {})
	add("live-flying", true, func(o *oraclePlayer) { o.c.SetFlying(true) })
	for name, apply := range oracleVariants() {
		add(name, false, apply)
	}
	// An invisible player is drawn to a viewer by who the viewer is.
	m["CharInfo/invisible-gm-viewer"] = func() wire.Frame {
		o := newOraclePlayer(0, a, false)
		o.c.SetInvisible(true)
		o.revealed = true
		return FrameCharInfo(o.char())
	}
	m["CharInfo/invisible-plain-viewer"] = func() wire.Frame {
		o := newOraclePlayer(0, a, false)
		o.invisible()
		return FrameCharInfo(o.char())
	}
}

// TestOracleCharacterInputsPinned holds the derived values the probe's player
// stand-in answers (ProbeCharacters.player and .live) to what the live
// calculators answer for the inputs the Go cases build: the oracle then
// compares layout and the plain fields, while the calculators have their own
// reference probes.
func TestOracleCharacterInputsPinned(t *testing.T) {
	type derived struct {
		str, dex, con, intl, wit, men               int
		pAtk, pDef, mAtk, mDef                      float64
		accuracy, evasion                           int
		crit                                        float64
		pAtkSpd, mAtkSpd, run, walk, swim, invLimit int
		move, attack                                float32
		maxHP, maxMP, maxCP                         float64
	}
	read := func(c *player.Character) derived {
		res := c.ResourceValues()
		return derived{
			c.STR(), c.DEX(), c.CON(), c.INT(), c.WIT(), c.MEN(),
			c.PAtk(), c.PDef(), c.MAtk(), c.MDef(),
			c.Accuracy(), c.Evasion(), c.CriticalRate(),
			c.AttackSpeed(), c.MagicAttackSpeed(), c.BaseRunSpeed(), c.BaseWalkSpeed(), c.BaseSwimSpeed(), c.InventoryLimit(),
			c.MovementSpeedMultiplier(), c.AttackSpeedMultiplier(),
			res.MaxHP, res.MaxMP, res.MaxCP,
		}
	}
	for r, row := range oracleRows {
		got := read(newOraclePlayer(r, row.v, false).c)
		maxima := [3]float64{float64(rowField(row.v, 7)), float64(rowField(row.v, 9)), float64(rowField(row.v, 0))}
		want := derived{1, 1, 1, 1, 1, 1, 0, 1, 0, 1, 46, 40, 34, 255, 133, 0, 0, 0, 80, 1, 0.935, maxima[0], maxima[1], maxima[2]}
		if got != want {
			t.Errorf("row %s: derived stats = %+v, want %+v (ProbeCharacters.player)", row.name, got, want)
		}
	}
	got := read(newOraclePlayer(0, oracleRows[0].v, true).c)
	want := derived{40, 30, 43, 21, 11, 25, 6, 38, 3, 24, 72, 72, 44, 330, 213, 120, 80, 50, 80, 1.1, 1.21, 1227, 126, 86}
	if got != want {
		t.Errorf("live: derived stats = %+v, want %+v (ProbeCharacters.live)", got, want)
	}
}
