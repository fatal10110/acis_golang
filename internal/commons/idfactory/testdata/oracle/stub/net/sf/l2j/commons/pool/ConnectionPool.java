package net.sf.l2j.commons.pool;

import java.lang.reflect.InvocationHandler;
import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.List;

/**
 * Probe stub: a connection whose statements do nothing and record the SQL text
 * they are given, in order, as "update", "query" or "prepare" lines.
 */
public final class ConnectionPool
{
	public static final List<String> LOG = new ArrayList<>();
	
	private static Object defaultFor(Class<?> type)
	{
		if (type == boolean.class)
			return false;
		if (type == int.class)
			return 0;
		if (type == long.class)
			return 0L;
		return null;
	}
	
	private static <T> T proxy(Class<T> type, InvocationHandler h)
	{
		return type.cast(Proxy.newProxyInstance(ConnectionPool.class.getClassLoader(), new Class<?>[]
		{
			type
		}, h));
	}
	
	private static final ResultSet EMPTY = proxy(ResultSet.class, (p, m, a) -> defaultFor(m.getReturnType()));
	
	private static Object statement(Method m, Object[] a)
	{
		switch (m.getName())
		{
			case "executeUpdate":
				if (a != null && a.length > 0)
					LOG.add("update " + a[0]);
				return 0;
			case "executeQuery":
				LOG.add("query " + a[0]);
				return EMPTY;
			default:
				return defaultFor(m.getReturnType());
		}
	}
	
	public static Connection getConnection()
	{
		return proxy(Connection.class, (p, m, a) ->
		{
			switch (m.getName())
			{
				case "createStatement":
					return proxy(Statement.class, (p2, m2, a2) -> statement(m2, a2));
				case "prepareStatement":
					LOG.add("prepare " + a[0]);
					return proxy(PreparedStatement.class, (p2, m2, a2) -> "executeUpdate".equals(m2.getName()) ? 0 : statement(m2, a2));
				default:
					return defaultFor(m.getReturnType());
			}
		});
	}
}
