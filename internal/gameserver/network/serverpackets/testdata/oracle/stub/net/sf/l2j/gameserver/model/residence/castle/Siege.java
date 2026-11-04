package net.sf.l2j.gameserver.model.residence.castle;

import java.util.ArrayList;
import java.util.Calendar;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import net.sf.l2j.gameserver.enums.SiegeSide;
import net.sf.l2j.gameserver.model.pledge.Clan;

// Stand-in for a castle siege: the registered clans in the order a probe lists them, the side
// it reports for a clan (one of sides, else side) and the siege date.
public final class Siege
{
	public SiegeSide side = SiegeSide.PENDING;
	public boolean inProgress;

	public boolean isInProgress()
	{
		return inProgress;
	}

	public SiegeSide getSide(Clan clan)
	{
		return sides.getOrDefault(clan, side);
	}

	public final Map<Clan, SiegeSide> sides = new HashMap<>();
	public final List<Clan> attackers = new ArrayList<>();
	public final List<Clan> defenders = new ArrayList<>();
	public final List<Clan> pending = new ArrayList<>();
	public final Calendar siegeDate = Calendar.getInstance();

	public List<Clan> getAttackerClans()
	{
		return attackers;
	}

	public List<Clan> getDefenderClans()
	{
		return defenders;
	}

	public List<Clan> getPendingClans()
	{
		return pending;
	}

	public Calendar getSiegeDate()
	{
		return siegeDate;
	}
}
