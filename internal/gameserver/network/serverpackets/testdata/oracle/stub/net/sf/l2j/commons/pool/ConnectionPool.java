package net.sf.l2j.commons.pool;

import java.sql.Connection;
import java.sql.SQLException;

// Stand-in for the connection pool: hands out the in-memory database the
// probe fills. The character list reads its rows through the reference JDBC
// calls, so the queries run unchanged.
public final class ConnectionPool
{
	private ConnectionPool()
	{
	}

	public static Connection getConnection() throws SQLException
	{
		return FakeDb.connection();
	}
}
