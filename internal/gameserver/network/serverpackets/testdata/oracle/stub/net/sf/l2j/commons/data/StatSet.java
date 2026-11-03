package net.sf.l2j.commons.data;

import java.util.HashMap;
import java.util.Map;

// Stand-in: a map-backed set with the two readers the value types use (the
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

	public boolean getBool(String key)
	{
		return (Boolean) _map.get(key);
	}
}
