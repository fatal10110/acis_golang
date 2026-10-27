package main

import (
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// loadEventRules reads the events.properties settings the event quests
// read, with the shipped defaults for a missing key; NeededPartyMembers is
// held to 2 to 9, the Dimensional Rift settings are taken as written.
func loadEventRules(paths gameServerPaths) (script.EventRules, error) {
	props, err := config.LoadFile(paths.EventsConfigPath)
	if err != nil {
		return script.EventRules{}, err
	}
	f := config.NewFields(props, "events")
	def := script.DefaultEventRules()
	r := script.EventRules{
		FourGobletsParty: min(max(f.Int("NeededPartyMembers", def.FourGobletsParty), 2), 9),
		Rift: script.RiftRules{
			MinParty: f.Int("RiftMinPartySize", def.Rift.MinParty),
			Costs: [6]int{
				f.Int("RecruitCost", def.Rift.Costs[0]),
				f.Int("SoldierCost", def.Rift.Costs[1]),
				f.Int("OfficerCost", def.Rift.Costs[2]),
				f.Int("CaptainCost", def.Rift.Costs[3]),
				f.Int("CommanderCost", def.Rift.Costs[4]),
				f.Int("HeroCost", def.Rift.Costs[5]),
			},
			JumpDelayMin:       f.Int("AutoJumpsDelayMin", def.Rift.JumpDelayMin),
			JumpDelayRnd:       f.Int("AutoJumpsDelayRnd", def.Rift.JumpDelayRnd),
			AnakazelPortChance: f.Int("AnakazelPortChance", def.Rift.AnakazelPortChance),
		},
	}
	return r, f.Err()
}

// provideEventBook returns the event settings read from the events
// properties, in a book //reload config sets whole.
func provideEventBook(paths gameServerPaths) (*script.EventBook, error) {
	r, err := loadEventRules(paths)
	if err != nil {
		return nil, err
	}
	return script.NewEventBook(r), nil
}
