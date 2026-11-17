import net.sf.l2j.commons.pool.ConnectionPool;
import net.sf.l2j.gameserver.idfactory.IdFactory;

/**
 * Boots the reference IdFactory (compiled unmodified) against a connection that
 * records every statement, and prints them in order: the online reset, the
 * orphan cleanup and repair, the expired timestamps and the used id queries.
 */
public final class IdFactoryProbe
{
	public static void main(String[] args)
	{
		IdFactory.getInstance();
		ConnectionPool.LOG.forEach(System.out::println);
	}
}
