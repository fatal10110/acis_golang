package main

import "github.com/fatal10110/acis_golang/internal/config"

// adminConfig holds the game-master settings: whether admin commands are
// audited (server.properties GMAudit) and the modes a game master logs in
// with (players.properties GMStartup*): invulnerable, invisible, blocking
// everything, and on the /gmlist list; and whether a game master shows the
// hero aura (players.properties GMHeroAura).
type adminConfig struct {
	GMAudit               bool
	GMStartupInvulnerable bool
	GMStartupInvisible    bool
	GMStartupBlockAll     bool
	GMStartupAutoList     bool
	GMHeroAura            bool
}

func loadAdminConfig(paths gameServerPaths) (adminConfig, error) {
	server, err := config.LoadFile(paths.ConfigPath)
	if err != nil {
		return adminConfig{}, err
	}
	players, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return adminConfig{}, err
	}
	sf := config.NewFields(server, "gm audit")
	pf := config.NewFields(players, "gm startup")
	cfg := adminConfig{
		GMAudit:               sf.Bool("GMAudit", false),
		GMStartupInvulnerable: pf.Bool("GMStartupInvulnerable", false),
		GMStartupInvisible:    pf.Bool("GMStartupInvisible", false),
		GMStartupBlockAll:     pf.Bool("GMStartupBlockAll", false),
		GMStartupAutoList:     pf.Bool("GMStartupAutoList", true),
		GMHeroAura:            pf.Bool("GMHeroAura", false),
	}
	if err := sf.Err(); err != nil {
		return adminConfig{}, err
	}
	return cfg, pf.Err()
}
