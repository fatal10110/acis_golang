package net.sf.l2j.commons.pool;

import java.lang.reflect.InvocationHandler;
import java.lang.reflect.Proxy;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.Iterator;
import java.util.List;
import java.util.Locale;
import java.util.Map;

/**
 * In-memory tables behind the stand-in connection pool. A row is a map from
 * lower-case column name to value; a query is routed by the table its text
 * names and filtered by the one bound parameter against the column the query
 * compares (the owner of the rows it selects).
 */
public final class FakeDb
{
	public static final List<Map<String, Object>> CHARACTERS = new ArrayList<>();
	public static final List<Map<String, Object>> SUBCLASSES = new ArrayList<>();
	public static final List<Map<String, Object>> ITEMS = new ArrayList<>();
	public static final List<Map<String, Object>> AUGMENTATIONS = new ArrayList<>();

	private FakeDb()
	{
	}

	public static void reset()
	{
		CHARACTERS.clear();
		SUBCLASSES.clear();
		ITEMS.clear();
		AUGMENTATIONS.clear();
	}

	/** Builds a row from alternating column names and values. */
	public static Map<String, Object> row(Object... kv)
	{
		final Map<String, Object> m = new HashMap<>();
		for (int i = 0; i < kv.length; i += 2)
			m.put(((String) kv[i]).toLowerCase(Locale.ROOT), kv[i + 1]);
		return m;
	}

	static Connection connection()
	{
		return (Connection) Proxy.newProxyInstance(FakeDb.class.getClassLoader(), new Class<?>[]
		{
			Connection.class
		}, (proxy, method, args) ->
		{
			switch (method.getName())
			{
				case "prepareStatement":
					return statement((String) args[0]);
				case "close":
					return null;
			}
			throw new UnsupportedOperationException(method.getName());
		});
	}

	private static PreparedStatement statement(String sql)
	{
		final List<Map<String, Object>> table;
		final String column;
		if (sql.contains("FROM characters"))
		{
			table = CHARACTERS;
			column = "account_name";
		}
		else if (sql.contains("FROM character_subclasses"))
		{
			table = SUBCLASSES;
			column = "char_obj_id";
		}
		else if (sql.contains("FROM augmentations"))
		{
			table = AUGMENTATIONS;
			column = "item_oid";
		}
		else if (sql.contains("FROM items"))
		{
			table = ITEMS;
			column = "owner_id";
		}
		else
			throw new UnsupportedOperationException(sql);

		final Object[] params = new Object[3];
		final InvocationHandler h = (proxy, method, args) ->
		{
			switch (method.getName())
			{
				case "setInt":
				case "setString":
					params[(Integer) args[0]] = args[1];
					return null;
				case "executeQuery":
				{
					final List<Map<String, Object>> rows = new ArrayList<>();
					for (Map<String, Object> r : table)
						if (params[1].equals(r.get(column)))
							rows.add(r);
					// The subclass query also compares the class id.
					if (table == SUBCLASSES)
						rows.removeIf(r -> !params[2].equals(r.get("class_id")));
					return results(rows);
				}
				case "close":
					return null;
			}
			throw new UnsupportedOperationException(method.getName());
		};
		return (PreparedStatement) Proxy.newProxyInstance(FakeDb.class.getClassLoader(), new Class<?>[]
		{
			PreparedStatement.class
		}, h);
	}

	private static ResultSet results(List<Map<String, Object>> rows)
	{
		final Iterator<Map<String, Object>> it = rows.iterator();
		final Map<String, Object>[] current = new Map[1];
		return (ResultSet) Proxy.newProxyInstance(FakeDb.class.getClassLoader(), new Class<?>[]
		{
			ResultSet.class
		}, (proxy, method, args) ->
		{
			switch (method.getName())
			{
				case "next":
					if (!it.hasNext())
						return false;
					current[0] = it.next();
					return true;
				case "getInt":
					return ((Number) value(current[0], (String) args[0])).intValue();
				case "getLong":
					return ((Number) value(current[0], (String) args[0])).longValue();
				case "getDouble":
					return ((Number) value(current[0], (String) args[0])).doubleValue();
				case "getString":
					return (String) value(current[0], (String) args[0]);
				case "close":
					return null;
			}
			throw new UnsupportedOperationException(method.getName());
		});
	}

	private static Object value(Map<String, Object> row, String column)
	{
		final Object v = row.get(column.toLowerCase(Locale.ROOT));
		if (v == null)
			throw new IllegalStateException("no column " + column);
		return v;
	}
}
