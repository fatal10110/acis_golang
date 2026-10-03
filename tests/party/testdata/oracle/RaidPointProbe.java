import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.Collections;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.TreeMap;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * Probe of the reference raid point record: RaidPointManager's in-memory
 * store, ranking and record list, and the ExGetBossRecord body it answers
 * RequestGetBossRecord with. See README.md.
 */
public class RaidPointProbe
{
	// RaidPointManager._entries, getList, addPoints (minus the REPLACE),
	// getPointsByOwnerId and calculateRanking, unchanged otherwise.
	private static final Map<Integer, Map<Integer, Integer>> _entries = new ConcurrentHashMap<>();

	static void load(int objectId, int bossId, int points)
	{
		final Map<Integer, Integer> playerData = _entries.computeIfAbsent(objectId, m -> new HashMap<>());
		playerData.put(bossId, points);
	}

	static Map<Integer, Integer> getList(int objectId)
	{
		return _entries.get(objectId);
	}

	static void addPoints(int objectId, int bossId, int points)
	{
		if (points < 0)
			return;

		final Map<Integer, Integer> playerData = _entries.computeIfAbsent(objectId, m -> new HashMap<>());
		points = playerData.merge(bossId, points, Integer::sum);
	}

	static int getPointsByOwnerId(int objectId)
	{
		final Map<Integer, Integer> playerData = _entries.get(objectId);
		if (playerData == null || playerData.isEmpty())
			return 0;

		return playerData.values().stream().mapToInt(Number::intValue).sum();
	}

	static int calculateRanking(int objectId)
	{
		final Map<Integer, Integer> playersData = new HashMap<>();
		for (int ownerId : _entries.keySet())
		{
			final int points = getPointsByOwnerId(ownerId);
			if (points > 0)
				playersData.put(ownerId, points);
		}

		final AtomicInteger counter = new AtomicInteger(1);
		final Map<Integer, Integer> rankMap = new LinkedHashMap<>();
		playersData.entrySet().stream().sorted(Collections.reverseOrder(Map.Entry.comparingByValue())).forEachOrdered(e -> rankMap.put(e.getKey(), counter.getAndIncrement()));

		final Integer rank = rankMap.get(objectId);
		return (rank == null) ? 0 : rank;
	}

	// ExGetBossRecord.writeImpl, written little-endian.
	static String exGetBossRecord(int ranking, int totalPoints, Map<Integer, Integer> bossRecordInfo)
	{
		final ByteBuffer buf = ByteBuffer.allocate(4096).order(ByteOrder.LITTLE_ENDIAN);
		buf.put((byte) 0xfe);
		buf.putShort((short) 0x33);
		buf.putInt(ranking);
		buf.putInt(totalPoints);
		if (bossRecordInfo == null)
		{
			buf.putInt(0x00);
			buf.putInt(0x00);
			buf.putInt(0x00);
			buf.putInt(0x00);
		}
		else
		{
			buf.putInt(bossRecordInfo.size());
			for (Map.Entry<Integer, Integer> bossEntry : bossRecordInfo.entrySet())
			{
				buf.putInt(bossEntry.getKey());
				buf.putInt(bossEntry.getValue());
				buf.putInt(0x00);
			}
		}
		final StringBuilder sb = new StringBuilder();
		for (int i = 0; i < buf.position(); i++)
			sb.append(String.format("%02x", buf.get(i)));
		return sb.toString();
	}

	// RequestGetBossRecord.runImpl.
	static String request(int objectId)
	{
		final int points = getPointsByOwnerId(objectId);
		final int ranking = calculateRanking(objectId);
		final Map<Integer, Integer> list = getList(objectId);
		return exGetBossRecord(ranking, points, list);
	}

	public static void main(String[] args)
	{
		// Stored rows of player 1, in the table's key order.
		final int[][] stored =
		{
			{25001, 10},
			{25002, 20},
			{25016, 5},
			{25017, 7},
			{25033, 1},
			{29001, 30},
		};
		for (int[] row : stored)
			load(1, row[0], row[1]);

		// Player 1: thirteen bosses, the thirteenth growing the table.
		addPoints(1, 25003, 4);
		addPoints(1, 25001, 3);
		addPoints(1, 25004, 2);
		addPoints(1, 25005, 2);
		addPoints(1, 25006, 2);
		addPoints(1, 25007, 2);
		addPoints(1, 25018, 1);
		addPoints(1, 25019, 1);

		// Player 2: nine bosses in one bucket, the ninth growing the table.
		for (int boss = 25008; boss <= 25136; boss += 16)
			addPoints(2, boss, 5);

		// Player 3: a boss worth nothing. Player 4: a negative amount.
		addPoints(3, 25001, 0);
		addPoints(4, 25001, -1);

		for (int objectId = 1; objectId <= 4; objectId++)
			System.out.println(objectId + " " + request(objectId));
		
		// Restart: the stored rows (the totals addPoints wrote) are read
		// back in the table's key order.
		final TreeMap<Long, int[]> rows = new TreeMap<>();
		for (Map.Entry<Integer, Map<Integer, Integer>> player : _entries.entrySet())
			for (Map.Entry<Integer, Integer> boss : player.getValue().entrySet())
				rows.put(((long) player.getKey() << 32) | boss.getKey(), new int[] {player.getKey(), boss.getKey(), boss.getValue()});
		_entries.clear();
		for (int[] row : rows.values())
			load(row[0], row[1], row[2]);
		
		for (int objectId = 1; objectId <= 4; objectId++)
			System.out.println("restart " + objectId + " " + request(objectId));
	}
}
