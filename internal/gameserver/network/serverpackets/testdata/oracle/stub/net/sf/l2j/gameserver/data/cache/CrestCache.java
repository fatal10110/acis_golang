package net.sf.l2j.gameserver.data.cache;

import java.util.HashMap;
import java.util.Map;

import net.sf.l2j.gameserver.enums.CrestType;

// Stand-in for the crest cache: the crests a probe registers, read back with the
// reference size check.
public final class CrestCache
{
	private static final CrestCache INSTANCE = new CrestCache();

	public final Map<Integer, byte[]> crests = new HashMap<>();

	public static CrestCache getInstance()
	{
		return INSTANCE;
	}

	public final byte[] getCrest(CrestType type, int id)
	{
		byte[] data = crests.get(id);

		if (data == null || data.length != type.getSize())
			return null;

		return data;
	}
}
