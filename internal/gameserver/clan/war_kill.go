package clan

// WarKill is the reputation one kill between clans at war moved.
type WarKill struct {
	// Killer is the killer's clan, Victim the victim's.
	Killer, Victim *Clan
	// Gain is the killer's clan's new score, when Gained.
	Gain   ReputationChanged
	Gained bool
	// Loss is the victim's clan's new score, when Lost.
	Loss ReputationChanged
	Lost bool
}

// CreditWarKill moves the reputation of victimID's death, in clan
// victimClanID, to killerID, in clan killerClanID. It moves only when each
// clan has declared war on the other and neither player is an academy
// member: the killer's clan gains 1 while the victim's clan holds a
// positive score, then the victim's clan loses 1 while the killer's clan
// (its gain counted) holds one. A clan below level 5 neither gains nor
// loses.
func (s *Service) CreditWarKill(victimID, victimClanID, killerID, killerClanID int32) WarKill {
	victimClan, ok := s.table.Get(victimClanID)
	if !ok {
		return WarKill{}
	}
	killerClan, ok := s.table.Get(killerClanID)
	if !ok || killerClan == victimClan {
		return WarKill{}
	}
	unlock := lockPair(victimClan, killerClan)
	defer unlock()
	victim, ok := victimClan.members[victimID]
	if !ok || victim.LvlJoinedAcademy > 0 {
		return WarKill{}
	}
	killer, ok := killerClan.members[killerID]
	if !ok || killer.LvlJoinedAcademy > 0 {
		return WarKill{}
	}
	_, victimAtWar := victimClan.wars[killerClan.id]
	_, killerAtWar := killerClan.wars[victimClan.id]
	if !victimAtWar || !killerAtWar {
		return WarKill{}
	}
	kill := WarKill{Killer: killerClan, Victim: victimClan}
	if victimClan.reputation > 0 {
		kill.Gain, kill.Gained = s.addReputationLocked(killerClan, 1)
	}
	if killerClan.reputation > 0 {
		kill.Loss, kill.Lost = s.addReputationLocked(victimClan, -1)
	}
	return kill
}
