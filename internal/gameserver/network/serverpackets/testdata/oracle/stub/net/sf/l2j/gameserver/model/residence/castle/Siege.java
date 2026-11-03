package net.sf.l2j.gameserver.model.residence.castle;

import net.sf.l2j.gameserver.enums.SiegeSide;
import net.sf.l2j.gameserver.model.pledge.Clan;

// Stand-in for a castle siege: the side it reports for every clan.
public final class Siege
{
	public SiegeSide side = SiegeSide.PENDING;

	public SiegeSide getSide(Clan clan)
	{
		return side;
	}
}
