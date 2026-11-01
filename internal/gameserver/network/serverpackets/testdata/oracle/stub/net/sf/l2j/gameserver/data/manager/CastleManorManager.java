package net.sf.l2j.gameserver.data.manager;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.stream.Collectors;

import net.sf.l2j.gameserver.model.manor.CropProcure;
import net.sf.l2j.gameserver.model.manor.Seed;
import net.sf.l2j.gameserver.model.manor.SeedProduction;

// Stand-in for the manor: the same lookups over plain tables the probe fills,
// without the database, the schedule or the periods. The queries are the
// reference's own.
public final class CastleManorManager
{
	private static final CastleManorManager INSTANCE = new CastleManorManager();

	public final Map<Integer, Seed> seeds = new LinkedHashMap<>();
	public final Map<Integer, List<SeedProduction>> production = new HashMap<>();
	public final Map<Integer, List<SeedProduction>> productionNext = new HashMap<>();
	public final Map<Integer, List<CropProcure>> procure = new HashMap<>();
	public final Map<Integer, List<CropProcure>> procureNext = new HashMap<>();
	public boolean approved;

	public static CastleManorManager getInstance()
	{
		return INSTANCE;
	}

	public List<SeedProduction> getSeedProduction(int castleId, boolean nextPeriod)
	{
		return (nextPeriod) ? productionNext.get(castleId) : production.get(castleId);
	}

	public SeedProduction getSeedProduct(int castleId, int seedId, boolean nextPeriod)
	{
		for (SeedProduction sp : getSeedProduction(castleId, nextPeriod))
		{
			if (sp.getId() == seedId)
				return sp;
		}
		return null;
	}

	public List<CropProcure> getCropProcure(int castleId, boolean nextPeriod)
	{
		return (nextPeriod) ? procureNext.get(castleId) : procure.get(castleId);
	}

	public CropProcure getCropProcure(int castleId, int cropId, boolean nextPeriod)
	{
		for (CropProcure cp : getCropProcure(castleId, nextPeriod))
		{
			if (cp.getId() == cropId)
				return cp;
		}
		return null;
	}

	public boolean isManorApproved()
	{
		return approved;
	}

	public List<Seed> getCrops()
	{
		final List<Seed> list = new ArrayList<>();
		final List<Integer> cropIds = new ArrayList<>();
		for (Seed seed : seeds.values())
		{
			if (!cropIds.contains(seed.getCropId()))
			{
				list.add(seed);
				cropIds.add(seed.getCropId());
			}
		}
		return list;
	}

	public Set<Seed> getSeedsForCastle(int castleId)
	{
		return seeds.values().stream().filter(s -> s.getCastleId() == castleId).collect(Collectors.toSet());
	}

	public Set<Integer> getCropIds()
	{
		return seeds.values().stream().map(Seed::getCropId).collect(Collectors.toSet());
	}

	public Seed getSeed(int seedId)
	{
		return seeds.get(seedId);
	}

	public Seed getSeedByCrop(int cropId)
	{
		return seeds.values().stream().filter(s -> s.getCropId() == cropId).findFirst().orElse(null);
	}
}
