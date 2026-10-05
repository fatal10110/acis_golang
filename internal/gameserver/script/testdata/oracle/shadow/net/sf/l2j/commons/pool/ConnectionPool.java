package net.sf.l2j.commons.pool;

import java.io.IOException;
import java.io.UncheckedIOException;
import java.lang.reflect.InvocationHandler;
import java.lang.reflect.Proxy;
import java.sql.CallableStatement;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.Statement;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.TreeMap;
import java.util.function.Consumer;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import java.util.stream.Stream;

/**
 * Probe shadow of the reference connection pool: no database. Every connection is a
 * stub whose updates touch no row and whose queries answer as a freshly installed
 * database would: a SELECT from a table the datapack's install scripts seed returns
 * every seeded row, any other query returns no row. While a sink is set, every executed
 * statement is reported to it as its SQL text followed by its bound parameters in index
 * order.
 */
public final class ConnectionPool
{
	private ConnectionPool()
	{
		throw new IllegalStateException("Utility class");
	}

	private static Consumer<String> _sink;

	public static void setSink(Consumer<String> sink)
	{
		_sink = sink;
	}

	/** Seed rows per table, read from the install scripts in the directory named by the probe.sql property. */
	private static final Map<String, List<Map<String, String>>> SEEDS = new HashMap<>();
	
	public static void init()
	{
		final String dir = System.getProperty("probe.sql");
		if (dir == null)
			return;
		try (Stream<Path> files = Files.list(Path.of(dir)))
		{
			for (Path f : files.filter(f -> f.toString().endsWith(".sql")).sorted().toList())
				seed(Files.readString(f));
		}
		catch (IOException e)
		{
			throw new UncheckedIOException(e);
		}
	}
	
	private static final Pattern CREATE = Pattern.compile("CREATE TABLE[^`]*`(\\w+)`\\s*\\((.*?)\\n\\)", Pattern.DOTALL | Pattern.CASE_INSENSITIVE);
	private static final Pattern COLUMN = Pattern.compile("^\\s*`(\\w+)`", Pattern.MULTILINE);
	private static final Pattern INSERT = Pattern.compile("INSERT[^`]*`(\\w+)`\\s*VALUES(.*?);", Pattern.DOTALL | Pattern.CASE_INSENSITIVE);
	
	private static void seed(String sql)
	{
		final Map<String, List<String>> columns = new HashMap<>();
		final Matcher c = CREATE.matcher(sql);
		while (c.find())
		{
			final List<String> cols = new ArrayList<>();
			final Matcher col = COLUMN.matcher(c.group(2));
			while (col.find())
				cols.add(col.group(1).toLowerCase());
			columns.put(c.group(1).toLowerCase(), cols);
		}
		final Matcher m = INSERT.matcher(sql);
		while (m.find())
		{
			final String table = m.group(1).toLowerCase();
			final List<String> cols = columns.get(table);
			if (cols == null)
				throw new IllegalStateException("seed rows for " + table + " without its columns");
			final List<Map<String, String>> rows = SEEDS.computeIfAbsent(table, k -> new ArrayList<>());
			for (List<String> values : tuples(m.group(2)))
			{
				final Map<String, String> row = new LinkedHashMap<>();
				for (int i = 0; i < cols.size(); i++)
					row.put(cols.get(i), values.get(i));
				rows.add(row);
			}
		}
	}
	
	/** Splits {@code (a,'b',"c"),(...)} into value lists; quotes are removed. */
	private static List<List<String>> tuples(String s)
	{
		final List<List<String>> out = new ArrayList<>();
		List<String> cur = null;
		StringBuilder tok = null;
		char quote = 0;
		for (int i = 0; i < s.length(); i++)
		{
			final char ch = s.charAt(i);
			if (quote != 0)
			{
				if (ch == quote)
					quote = 0;
				else
					tok.append(ch);
				continue;
			}
			switch (ch)
			{
				case '(' ->
				{
					cur = new ArrayList<>();
					tok = new StringBuilder();
				}
				case '\'', '"' -> quote = ch;
				case ',' ->
				{
					if (cur != null)
					{
						cur.add(tok.toString().trim());
						tok = new StringBuilder();
					}
				}
				case ')' ->
				{
					cur.add(tok.toString().trim());
					out.add(cur);
					cur = null;
				}
				default ->
				{
					if (cur != null)
						tok.append(ch);
				}
			}
		}
		return out;
	}
	
	private static final Pattern FROM = Pattern.compile("^\\s*SELECT\\b.*?\\bFROM\\s+`?(\\w+)", Pattern.DOTALL | Pattern.CASE_INSENSITIVE);
	
	private static List<Map<String, String>> rowsFor(String sql)
	{
		if (sql == null)
			return List.of();
		final Matcher m = FROM.matcher(sql);
		return m.find() ? SEEDS.getOrDefault(m.group(1).toLowerCase(), List.of()) : List.of();
	}

	public static void shutdown()
	{
	}

	public static Connection getConnection()
	{
		return (Connection) Proxy.newProxyInstance(ConnectionPool.class.getClassLoader(), new Class<?>[]
		{
			Connection.class
		}, ConnectionPool::connection);
	}

	private static Object connection(Object proxy, java.lang.reflect.Method m, Object[] args)
	{
		switch (m.getName())
		{
			case "prepareStatement", "prepareCall":
				return statement((String) args[0], m.getReturnType());
			case "createStatement":
				return statement(null, Statement.class);
			case "toString":
				return "probe-connection";
			case "hashCode":
				return System.identityHashCode(proxy);
			case "equals":
				return proxy == args[0];
			default:
				return zero(m.getReturnType());
		}
	}

	private static Object statement(String sql, Class<?> type)
	{
		final TreeMap<Integer, Object> params = new TreeMap<>();
		final List<String> batch = new ArrayList<>();
		final InvocationHandler h = (proxy, m, args) ->
		{
			final String name = m.getName();
			if (name.startsWith("set") && args != null && args.length >= 2 && args[0] instanceof Integer i)
			{
				params.put(i, name.equals("setNull") ? null : args[1]);
				return null;
			}
			switch (name)
			{
				case "clearParameters":
					params.clear();
					return null;
				case "addBatch":
					batch.add(render(args != null && args.length == 1 ? (String) args[0] : sql, params));
					return null;
				case "executeBatch":
					batch.forEach(ConnectionPool::report);
					final int[] counts = new int[batch.size()];
					batch.clear();
					return counts;
				case "executeQuery":
					final String query = args != null && args.length >= 1 ? (String) args[0] : sql;
					report(render(query, params));
					return resultSet(rowsFor(query));
				case "executeUpdate", "execute", "executeLargeUpdate":
					report(render(args != null && args.length >= 1 ? (String) args[0] : sql, params));
					return zero(m.getReturnType());
				case "getGeneratedKeys", "getResultSet":
					return resultSet(List.of());
				case "toString":
					return "probe-statement";
				case "hashCode":
					return System.identityHashCode(proxy);
				case "equals":
					return proxy == args[0];
				default:
					return zero(m.getReturnType());
			}
		};
		final Class<?> iface = type == CallableStatement.class ? CallableStatement.class : (type == PreparedStatement.class ? PreparedStatement.class : Statement.class);
		return Proxy.newProxyInstance(ConnectionPool.class.getClassLoader(), new Class<?>[]
		{
			iface
		}, h);
	}

	private static String render(String sql, TreeMap<Integer, Object> params)
	{
		final StringBuilder sb = new StringBuilder(sql == null ? "" : sql.trim());
		for (Object v : params.values())
			sb.append(" | ").append(v);
		return sb.toString();
	}

	private static void report(String line)
	{
		if (_sink != null)
			_sink.accept(line);
	}

	private static ResultSet resultSet(List<Map<String, String>> rows)
	{
		final int[] at =
		{
			-1
		};
		final boolean[] wasNull =
		{
			false
		};
		return (ResultSet) Proxy.newProxyInstance(ConnectionPool.class.getClassLoader(), new Class<?>[]
		{
			ResultSet.class
		}, (proxy, m, args) ->
		{
			final String name = m.getName();
			switch (name)
			{
				case "next":
					return ++at[0] < rows.size();
				case "wasNull":
					return wasNull[0];
				case "toString":
					return "probe-resultset";
				case "hashCode":
					return System.identityHashCode(proxy);
				case "equals":
					return proxy == args[0];
			}
			if (name.startsWith("get") && args != null && args.length >= 1 && at[0] >= 0 && at[0] < rows.size())
			{
				final Map<String, String> row = rows.get(at[0]);
				final String v = args[0] instanceof Integer i ? new ArrayList<>(row.values()).get(i - 1) : row.get(((String) args[0]).toLowerCase());
				wasNull[0] = v == null || v.equalsIgnoreCase("null");
				return convert(wasNull[0] ? null : v, m.getReturnType());
			}
			return zero(m.getReturnType());
		});
	}
	
	private static Object convert(String v, Class<?> t)
	{
		if (v == null)
			return zero(t);
		if (t == String.class || t == Object.class)
			return v;
		if (t == int.class)
			return v.isEmpty() ? 0 : Integer.parseInt(v);
		if (t == long.class)
			return v.isEmpty() ? 0L : Long.parseLong(v);
		if (t == short.class)
			return v.isEmpty() ? (short) 0 : Short.parseShort(v);
		if (t == byte.class)
			return v.isEmpty() ? (byte) 0 : Byte.parseByte(v);
		if (t == double.class)
			return v.isEmpty() ? 0d : Double.parseDouble(v);
		if (t == float.class)
			return v.isEmpty() ? 0f : Float.parseFloat(v);
		if (t == boolean.class)
			return v.equals("1") || v.equalsIgnoreCase("true");
		if (t == java.math.BigDecimal.class)
			return new java.math.BigDecimal(v);
		throw new IllegalStateException("probe result set cannot answer " + t + " for " + v);
	}
	
	private static Object zero(Class<?> t)
	{
		if (!t.isPrimitive() || t == void.class)
			return null;
		if (t == boolean.class)
			return false;
		if (t == long.class)
			return 0L;
		if (t == double.class)
			return 0d;
		if (t == float.class)
			return 0f;
		if (t == short.class)
			return (short) 0;
		if (t == byte.class)
			return (byte) 0;
		if (t == char.class)
			return (char) 0;
		return 0;
	}
}
