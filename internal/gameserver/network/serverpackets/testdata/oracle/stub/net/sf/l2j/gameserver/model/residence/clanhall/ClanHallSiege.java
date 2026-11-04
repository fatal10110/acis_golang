package net.sf.l2j.gameserver.model.residence.clanhall;

import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.gameserver.enums.SiegeSide;
import net.sf.l2j.gameserver.model.pledge.Clan;

// Stand-in for a clan hall siege: the side it reports for every clan and the attackers
// in the order a probe lists them.
public final class ClanHallSiege
{
	public SiegeSide side = SiegeSide.PENDING;

	public boolean checkSide(Clan clan, SiegeSide wanted)
	{
		return side == wanted;
	}

	public final List<Clan> attackers = new ArrayList<>();

	public List<Clan> getAttackerClans()
	{
		return attackers;
	}
}
