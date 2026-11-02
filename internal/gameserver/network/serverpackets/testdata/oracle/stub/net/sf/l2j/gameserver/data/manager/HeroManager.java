package net.sf.l2j.gameserver.data.manager;

import java.util.HashMap;
import java.util.Map;

import net.sf.l2j.commons.data.StatSet;

// Stand-in for the hero manager: the era's heroes a probe registers, in the same kind of
// table as the reference, and the key names of a hero's set.
public final class HeroManager
{
	public static final String CLASS_ID = "class_id";
	public static final String CHAR_NAME = "char_name";
	public static final String COUNT = "count";
	public static final String CLAN_NAME = "clan_name";
	public static final String CLAN_CREST = "clan_crest";
	public static final String ALLY_NAME = "ally_name";
	public static final String ALLY_CREST = "ally_crest";

	private static final HeroManager INSTANCE = new HeroManager();

	public final Map<Integer, StatSet> heroes = new HashMap<>();

	public static HeroManager getInstance()
	{
		return INSTANCE;
	}

	public Map<Integer, StatSet> getHeroes()
	{
		return heroes;
	}
}
