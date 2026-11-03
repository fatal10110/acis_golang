package net.sf.l2j.commons.data;

import java.util.HashMap;
import java.util.Map;

// Stand-in: a map-backed set with the readers the value types use (the
// location type and the manor seed build themselves from one).
public final class StatSet
{
	private final Map<String, Object> _map = new HashMap<>();

	public StatSet set(String key, Object value)
	{
		_map.put(key, value);
		return this;
	}

	public int getInteger(String key)
	{
		return ((Number) _map.get(key)).intValue();
	}

	public String getString(String key)
	{
		return (String) _map.get(key);
	}

	public String getString(String key, String defaultValue)
	{
		final Object val = _map.get(key);
		return (val == null) ? defaultValue : (String) val;
	}

	public int getInteger(String key, int defaultValue)
	{
		final Object val = _map.get(key);
		return (val == null) ? defaultValue : ((Number) val).intValue();
	}

	// The pair a node names under key (an item the node needs); unread unless containsKey.
	public net.sf.l2j.gameserver.model.holder.IntIntHolder getIntIntHolder(String key)
	{
		return (net.sf.l2j.gameserver.model.holder.IntIntHolder) _map.get(key);
	}

	public boolean containsKey(String key)
	{
		return _map.containsKey(key);
	}

	public boolean getBool(String key, boolean defaultValue)
	{
		final Object val = _map.get(key);
		return (val == null) ? defaultValue : (Boolean) val;
	}

	public boolean getBool(String key)
	{
		return (Boolean) _map.get(key);
	}
}
