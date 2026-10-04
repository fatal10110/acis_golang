package network

// adminEndOlympiad answers //endoly: the running Olympiad ends at once and
// its heroes are elected (olympiad.Olympiad.SelectHeroes), and gm is told.
func (l *GameClientLink) adminEndOlympiad(gm *livePlayer, _ string) {
	if l.olympiad != nil {
		l.olympiad.SelectHeroes()
	}
	sendText(gm, "Heroes have been formed.")
}

// adminSetHero answers //sethero: the selected player, gm itself without
// one, gains hero status or loses it, on its own queue, with the hero
// skills and its look refreshed around it; gm is told. The status is not
// stored: the next login restores the elected one.
func (l *GameClientLink) adminSetHero(gm *livePlayer, _ string) {
	target := adminTargetPlayer(gm, true)
	onPlayer(gm, target, func() {
		l.setHero(target, !target.IsHero())
		l.broadcastCharacterInfo(target)
		sendText(gm, "You have modified "+target.Name+"'s hero status.")
	})
}
