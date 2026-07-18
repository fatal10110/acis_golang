package zone

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func statSet(kv map[string]string) *commons.StatSet {
	set := commons.NewStatSet()
	for k, v := range kv {
		set.Set(k, v)
	}
	return set
}

func TestFlagsCounterFloorsAtZero(t *testing.T) {
	var f Flags
	f.Set(FlagWater, false) // releasing a clear flag must not go negative
	f.Set(FlagWater, true)
	if !f.Has(FlagWater) {
		t.Fatal("flag not active after one hold")
	}
	f.Set(FlagWater, false)
	if f.Has(FlagWater) {
		t.Fatal("flag still active after its only hold was released")
	}
}

func TestPvPFlagYieldsToPeace(t *testing.T) {
	var f Flags
	f.Set(FlagPvP, true)
	if !f.Has(FlagPvP) {
		t.Fatal("pvp flag inactive without peace")
	}
	f.Set(FlagPeace, true)
	if f.Has(FlagPvP) {
		t.Fatal("pvp flag active inside a peace zone")
	}
	if !f.Has(FlagPeace) {
		t.Fatal("peace flag inactive")
	}
}

func TestDamageZoneUnlinkedTrap(t *testing.T) {
	z, err := NewDamage(1, testForm, statSet(map[string]string{"initialDelay": "1500", "reuseDelay": "2500"}))
	if err != nil {
		t.Fatal(err)
	}
	if z.HPDamage != 200 || z.InitialDelay != 1500*time.Millisecond || z.ReuseDelay != 2500*time.Millisecond {
		t.Fatalf("settings = %d hp, %v/%v, want 200 hp, 1.5s/2.5s", z.HPDamage, z.InitialDelay, z.ReuseDelay)
	}

	var pulses, notices int
	z.StartPulse = func() { pulses++ }
	z.DangerNotice = func(Actor) { notices++ }

	npc := &fakeActor{id: 1, pos: insideAt, class: ClassNPC}
	Revalidate(z, npc)
	if z.Inside(npc) {
		t.Fatal("damage zone affected a non-playable")
	}

	a := newFakePlayer(2, insideAt)
	b := newFakePlayer(3, insideAt)
	Revalidate(z, a)
	Revalidate(z, b)
	if pulses != 1 {
		t.Fatalf("pulse started %d times, want once", pulses)
	}
	if !a.flags.Has(FlagDanger) || notices != 2 {
		t.Fatalf("danger state: flag=%v notices=%d, want raised flag and 2 notices", a.flags.Has(FlagDanger), notices)
	}

	// After the pulse task reports it stopped, a fresh entry restarts it.
	z.PulseStopped()
	c := newFakePlayer(4, insideAt)
	Revalidate(z, c)
	if pulses != 2 {
		t.Fatalf("pulse not restarted after PulseStopped, count=%d", pulses)
	}
}

func TestDamageZoneStartsPulseOutsideLatch(t *testing.T) {
	z, err := NewDamage(1, testForm, commons.NewStatSet())
	if err != nil {
		t.Fatal(err)
	}
	a := newFakePlayer(2, insideAt)
	done := make(chan struct{})
	z.StartPulse = func() {
		z.PulseStopped()
		close(done)
	}

	Revalidate(z, a)
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("StartPulse could not call back into zone state")
	}
}

func TestDamageZoneDormantCastleTrap(t *testing.T) {
	z, err := NewDamage(1, testForm, statSet(map[string]string{"castleId": "3", "eventId": "10"}))
	if err != nil {
		t.Fatal(err)
	}
	var pulses int
	z.StartPulse = func() { pulses++ }

	a := newFakePlayer(2, insideAt)
	Revalidate(z, a)
	if pulses != 0 {
		t.Fatal("dormant castle trap started its pulse")
	}
	if a.flags.Has(FlagDanger) {
		t.Fatal("dormant castle trap raised the danger flag")
	}

	// Armed trap during a running siege is live again.
	z.Armed = true
	z.SiegeActive = func() bool { return true }
	b := newFakePlayer(3, insideAt)
	Revalidate(z, b)
	if pulses != 1 || !b.flags.Has(FlagDanger) {
		t.Fatal("armed trap under siege did not fire")
	}
}

func TestDangerNoticeOnExitWaitsForLastHold(t *testing.T) {
	z1, _ := NewDamage(1, testForm, commons.NewStatSet())
	z2, _ := NewDamage(2, testForm, commons.NewStatSet())
	var notices int
	z1.DangerNotice = func(Actor) { notices++ }
	z1.StartPulse = func() {}
	z2.StartPulse = func() {}

	a := newFakePlayer(7, insideAt)
	Revalidate(z1, a)
	Revalidate(z2, a)
	notices = 0

	// Leaving z1 while z2 still holds the danger flag: no refresh yet.
	Remove(z1, a)
	if notices != 0 {
		t.Fatal("danger display refreshed while another danger zone still held the flag")
	}
	Remove(z2, a)
	if a.flags.Has(FlagDanger) {
		t.Fatal("danger flag still raised after leaving both zones")
	}
}

func TestEffectZoneSettingsAndScope(t *testing.T) {
	set := statSet(map[string]string{
		"skill":      "4070-1;4150-3",
		"chance":     "50",
		"reuseDelay": "6000",
		"targetType": "Npc",
	})
	z, err := NewEffect(1, testForm, set)
	if err != nil {
		t.Fatal(err)
	}
	want := []SkillRef{{ID: 4070, Level: 1}, {ID: 4150, Level: 3}}
	if len(z.Skills) != 2 || z.Skills[0] != want[0] || z.Skills[1] != want[1] {
		t.Fatalf("skills = %v, want %v", z.Skills, want)
	}
	if z.Chance != 50 || z.ReuseDelay != 6*time.Second || !z.Enabled() {
		t.Fatalf("settings = chance %d, reuse %v, enabled %v", z.Chance, z.ReuseDelay, z.Enabled())
	}

	// Npc scope: players are not even tracked.
	p := newFakePlayer(2, insideAt)
	Revalidate(z, p)
	if z.Inside(p) {
		t.Fatal("Npc-scoped effect zone tracked a player")
	}
	npc := &fakeActor{id: 3, pos: insideAt, class: ClassNPC}
	var pulses int
	z.StartPulse = func() { pulses++ }
	Revalidate(z, npc)
	if !z.Inside(npc) || pulses != 1 {
		t.Fatal("Npc-scoped effect zone ignored an NPC")
	}
}

func TestEffectZoneStartsPulseOutsideLatch(t *testing.T) {
	z, err := NewEffect(1, testForm, commons.NewStatSet())
	if err != nil {
		t.Fatal(err)
	}
	npc := &fakeActor{id: 3, pos: insideAt, class: ClassNPC}
	z.Target = ScopeNPC
	done := make(chan struct{})
	z.StartPulse = func() {
		z.Enabled()
		close(done)
	}

	Revalidate(z, npc)
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("StartPulse could not call back into zone state")
	}
}

func TestEffectZoneRejectsMalformedSkill(t *testing.T) {
	for _, raw := range []string{"4070", "a-1", "4070-b"} {
		if _, err := NewEffect(1, testForm, statSet(map[string]string{"skill": raw})); err == nil {
			t.Errorf("skill %q accepted, want error", raw)
		}
	}
}

func TestBossZoneEntryPolicing(t *testing.T) {
	set := statSet(map[string]string{"InvadeTime": "600000", "oustX": "1", "oustY": "2", "oustZ": "3"})
	z, err := NewBoss(1, testForm, set)
	if err != nil {
		t.Fatal(err)
	}
	if z.InvadeWindow != 10*time.Minute {
		t.Fatalf("invade window = %v, want 10m", z.InvadeWindow)
	}
	clock := time.Unix(1000, 0)
	z.now = func() time.Time { return clock }

	var ejected, unsummoned []int32
	z.Eject = func(a Actor) { ejected = append(ejected, a.ObjectID()) }
	z.Unsummon = func(a Actor) { unsummoned = append(unsummoned, a.ObjectID()) }

	// Uninvited player: boss flag still raises, then thrown out.
	uninvited := newFakePlayer(10, insideAt)
	Revalidate(z, uninvited)
	if !uninvited.flags.Has(FlagBoss) || !uninvited.flags.Has(FlagNoSummonFriend) {
		t.Fatal("entry flags missing")
	}
	if len(ejected) != 1 || ejected[0] != 10 {
		t.Fatalf("uninvited player not ejected: %v", ejected)
	}

	// Permitted player inside the window enters freely.
	invited := newFakePlayer(11, insideAt)
	z.AllowEntry(11, time.Minute)
	Revalidate(z, invited)
	if len(ejected) != 1 {
		t.Fatalf("permitted player was ejected: %v", ejected)
	}

	// Its deadline was consumed: walking out online revokes the permission.
	invited.pos = outside
	Revalidate(z, invited)
	if z.isAllowed(11) {
		t.Fatal("walk-out kept the entry permission")
	}

	// Disconnect inside grants a re-entry deadline instead.
	dc := newFakePlayer(12, insideAt)
	z.AllowEntry(12, time.Minute)
	Revalidate(z, dc)
	dc.online = false
	Remove(z, dc)
	if !z.isAllowed(12) {
		t.Fatal("disconnect revoked the permission")
	}
	dc.online = true
	dc.pos = insideAt
	Revalidate(z, dc) // within the invade window: back in
	if len(ejected) != 1 {
		t.Fatalf("re-entry within the window was rejected: %v", ejected)
	}

	// Expired permission: thrown out and revoked.
	late := newFakePlayer(13, insideAt)
	z.AllowEntry(13, time.Minute)
	clock = clock.Add(2 * time.Minute)
	Revalidate(z, late)
	if len(ejected) != 2 || ejected[1] != 13 || z.isAllowed(13) {
		t.Fatalf("expired permission not enforced: ejected=%v allowed=%v", ejected, z.isAllowed(13))
	}

	// GM walks in regardless.
	gm := newFakePlayer(14, insideAt)
	gm.gm = true
	Revalidate(z, gm)
	if len(ejected) != 2 {
		t.Fatal("GM was ejected")
	}

	// Summon of an unpermitted owner is dismissed; permitted owner keeps it.
	owner := newFakePlayer(20, insideAt)
	pet := newFakeSummon(21, insideAt, owner)
	Revalidate(z, pet)
	if len(unsummoned) != 1 || unsummoned[0] != 21 {
		t.Fatalf("summon of unpermitted owner kept: %v", unsummoned)
	}
	z.AllowEntry(20, time.Minute)
	pet2 := newFakeSummon(22, insideAt, owner)
	Revalidate(z, pet2)
	if len(unsummoned) != 1 {
		t.Fatal("summon of permitted owner dismissed")
	}
}

func TestBossZoneRaidRecall(t *testing.T) {
	// The recall check only runs for policed zones: an unpoliced exit (no
	// invade window) stops player exit handling right after the flags.
	z, err := NewBoss(1, testForm, statSet(map[string]string{"InvadeTime": "600000"}))
	if err != nil {
		t.Fatal(err)
	}
	var recalls int
	var strays []int32
	z.RecallRaids = func() { recalls++ }
	z.RecallNPC = func(a Actor) { strays = append(strays, a.ObjectID()) }

	raid := &fakeActor{id: 1, pos: insideAt, class: ClassNPC}
	p := newFakePlayer(2, insideAt)
	z.AllowEntry(2, time.Minute)
	Revalidate(z, raid)
	Revalidate(z, p)

	// Last playable leaves while the raid stays: recall fires.
	p.pos = outside
	Revalidate(z, p)
	if recalls != 1 {
		t.Fatalf("raid recall fired %d times, want once", recalls)
	}

	// The raid monster itself wandering out goes through the stray hook.
	raid.pos = outside
	Revalidate(z, raid)
	if len(strays) != 1 || strays[0] != 1 {
		t.Fatalf("stray recall = %v, want [1]", strays)
	}
}

func TestSiegeZoneLifecycle(t *testing.T) {
	z, err := NewSiege(1, testForm, statSet(map[string]string{"castleId": "3"}))
	if err != nil {
		t.Fatal(err)
	}
	if z.ResidenceID != 3 {
		t.Fatalf("residence = %d, want 3", z.ResidenceID)
	}

	var pvpFlags, notices int
	z.FlagPvP = func(Actor) { pvpFlags++ }
	z.CombatNotice = func(_ Actor, _ bool) { notices++ }

	// Inactive siege: entering imposes nothing.
	a := newFakePlayer(7, insideAt)
	Revalidate(z, a)
	if a.flags.Has(FlagSiege) || a.flags.Has(FlagPvP) {
		t.Fatal("inactive siege imposed combat state")
	}

	// Activation replays the entry rules for everyone inside.
	z.SetActive(true)
	if !a.flags.Has(FlagSiege) || !a.flags.Has(FlagPvP) || !a.flags.Has(FlagNoSummonFriend) {
		t.Fatal("activation did not impose combat state on occupants")
	}

	// Leaving an active siege puts the player on the timed pvp flag.
	a.pos = outside
	Revalidate(z, a)
	if pvpFlags != 1 {
		t.Fatalf("leave-battlefield pvp flag fired %d times, want once", pvpFlags)
	}
	if a.flags.Has(FlagSiege) {
		t.Fatal("siege flag survived the exit")
	}

	// Deactivation strips combat state without the pvp flag.
	b := newFakePlayer(8, insideAt)
	Revalidate(z, b)
	z.SetActive(false)
	if b.flags.Has(FlagSiege) || b.flags.Has(FlagPvP) {
		t.Fatal("deactivation left combat state on occupants")
	}
	if pvpFlags != 1 {
		t.Fatal("deactivation applied the leave-battlefield pvp flag")
	}
}

func TestOlympiadZoneMatchState(t *testing.T) {
	z := NewOlympiad(1, testForm)
	battle := false
	z.BattleStarted = func() bool { return battle }
	var expelled []int32
	z.ExpelUninvited = func(a Actor) { expelled = append(expelled, a.ObjectID()) }

	a := newFakePlayer(7, insideAt)
	Revalidate(z, a)
	if a.flags.Has(FlagPvP) {
		t.Fatal("idle stadium imposed combat state")
	}
	if !a.flags.Has(FlagNoRestart) || !a.flags.Has(FlagNoSummonFriend) {
		t.Fatal("stadium entry flags missing")
	}
	if len(expelled) != 1 {
		t.Fatal("gate check skipped for an entering player")
	}

	battle = true
	z.UpdateCombatStatus()
	if !a.flags.Has(FlagPvP) {
		t.Fatal("match start did not impose combat state")
	}
	battle = false
	z.UpdateCombatStatus()
	if a.flags.Has(FlagPvP) {
		t.Fatal("match end left combat state")
	}
}

func TestCastleTeleportOustAll(t *testing.T) {
	set := statSet(map[string]string{
		"castleId": "3", "spawnMinX": "100", "spawnMaxX": "200",
		"spawnMinY": "-50", "spawnMaxY": "-10", "spawnZ": "-3000",
	})
	z, err := NewCastleTeleport(1, testForm, set)
	if err != nil {
		t.Fatal(err)
	}
	a := newFakePlayer(7, insideAt)
	Revalidate(z, a)
	if !a.flags.Has(FlagNoSummonFriend) {
		t.Fatal("summon block missing")
	}

	var got []location.Location
	z.Eject = func(_ Actor, to location.Location) { got = append(got, to) }
	z.OustAll()
	if len(got) != 1 {
		t.Fatalf("ejected %d players, want 1", len(got))
	}
	to := got[0]
	if to.X < 100 || to.X > 200 || to.Y < -50 || to.Y > -10 || to.Z != -3000 {
		t.Fatalf("exit point %+v outside the configured box", to)
	}
}

func TestBanishForeigners(t *testing.T) {
	z, err := NewCastle(1, testForm, statSet(map[string]string{"castleId": "3"}))
	if err != nil {
		t.Fatal(err)
	}
	owner := newFakePlayer(1, insideAt)
	owner.clan = 5
	intruder := newFakePlayer(2, insideAt)
	intruder.clan = 9
	Revalidate(z, owner)
	Revalidate(z, intruder)

	var banished []int32
	z.Banish = func(a Actor) { banished = append(banished, a.ObjectID()) }
	z.BanishForeigners(5)
	if len(banished) != 1 || banished[0] != 2 {
		t.Fatalf("banished = %v, want just the intruder (2)", banished)
	}
}

func TestSpawnsFallBackToNormal(t *testing.T) {
	var s Spawns
	if _, ok := s.RandomSpawn(SpawnChaotic); ok {
		t.Fatal("empty spawn set produced a location")
	}
	normal := location.Location{X: 1, Y: 2, Z: 3}
	s.AddSpawn(SpawnNormal, normal)
	if got := s.Spawn(SpawnChaotic); len(got) != 1 || got[0] != normal {
		t.Fatalf("chaotic spawns = %v, want fallback to the normal group", got)
	}
	chaotic := location.Location{X: 9, Y: 9, Z: 9}
	s.AddSpawn(SpawnChaotic, chaotic)
	if got := s.Spawn(SpawnChaotic); len(got) != 1 || got[0] != chaotic {
		t.Fatalf("chaotic spawns = %v, want the dedicated group", got)
	}
	if loc, ok := s.RandomSpawn(SpawnOwner); !ok || loc != normal {
		t.Fatalf("owner random spawn = %v/%v, want normal fallback", loc, ok)
	}
}
