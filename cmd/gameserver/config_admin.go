package main

import "github.com/fatal10110/acis_golang/internal/config"

// adminConfig holds the game-master settings: whether admin commands are
// audited (server.properties GMAudit) and whether a game master logs in on
// the /gmlist list (players.properties GMStartupAutoList).
type adminConfig struct {
	GMAudit           bool
	GMStartupAutoList bool
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
	pf := config.NewFields(players, "gm startup auto list")
	cfg := adminConfig{
		GMAudit:           sf.Bool("GMAudit", false),
		GMStartupAutoList: pf.Bool("GMStartupAutoList", true),
	}
	if err := sf.Err(); err != nil {
		return adminConfig{}, err
	}
	return cfg, pf.Err()
}
