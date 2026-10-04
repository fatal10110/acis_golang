package main

import (
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/classmaster"
)

// loadClassMasterConfig reads the npcs.properties class manager settings:
// AllowEntireTree (default false) and ConfigClassMaster, the occupation
// changes offered with their prices and rewards (none when the key is
// absent). A value that does not parse fails boot.
func loadClassMasterConfig(paths gameServerPaths) (classmaster.Config, error) {
	props, err := config.LoadFile(paths.NpcsConfigPath)
	if err != nil {
		return classmaster.Config{}, err
	}
	f := config.NewFields(props, "class master")
	entireTree := f.Bool("AllowEntireTree", false)
	if err := f.Err(); err != nil {
		return classmaster.Config{}, err
	}
	line, _ := props.Lookup("ConfigClassMaster")
	jobs, err := classmaster.ParseJobs(line)
	if err != nil {
		return classmaster.Config{}, err
	}
	return classmaster.NewConfig(entireTree, jobs), nil
}
