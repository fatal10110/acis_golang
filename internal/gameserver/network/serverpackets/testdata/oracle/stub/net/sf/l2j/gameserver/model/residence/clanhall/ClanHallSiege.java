package net.sf.l2j.gameserver.model.residence.clanhall;

import net.sf.l2j.gameserver.enums.SiegeSide;
import net.sf.l2j.gameserver.model.pledge.Clan;

// Stand-in for a clan hall siege: the side it reports for every clan.
public final class ClanHallSiege
{
	public SiegeSide side = SiegeSide.PENDING;

	public boolean checkSide(Clan clan, SiegeSide wanted)
	{
		return side == wanted;
	}
}
