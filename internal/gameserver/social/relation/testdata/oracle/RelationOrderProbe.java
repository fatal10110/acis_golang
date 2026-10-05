import java.nio.file.Files;
import java.nio.file.Path;
import java.util.HashSet;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

// Probe of aCis 409 RelationManager.java (data/manager/RelationManager.java)
// and PlayerPair.java (model/records/PlayerPair.java): the _relations store,
// the constructor's load, getBlockList, getFriendList, addToBlockList,
// removeFromBlockList, isInBlockList, areFriends, addToFriendList,
// removeFromFriendList and updateRelation copied verbatim except: a Player is
// its object id; packets, system messages and World lookups are dropped; the
// load reads the scenario's rows instead of SELECT * FROM character_relations.
// RequestAnswerFriendInvite's accept calls addToFriendList from both sides.
public class RelationOrderProbe
{
	record PlayerPair(int id1, int id2)
	{
		PlayerPair
		{
			if (id1 > id2)
			{
				final int temp = id1;
				id1 = id2;
				id2 = temp;
			}
		}

		boolean contains(int playerId)
		{
			return id1 == playerId || id2 == playerId;
		}
	}

	private static final int ARE_FRIENDS = 1;
	private static final int CHAR_BLOCKS_FRIEND = 2;
	private static final int FRIEND_BLOCKS_CHAR = 4;

	private final Map<PlayerPair, Integer> _relations = new ConcurrentHashMap<>();

	void load(int charId, int friendId, int relation)
	{
		_relations.putIfAbsent(new PlayerPair(charId, friendId), relation);
	}

	boolean isInBlockList(int player, int targetId)
	{
		final PlayerPair key = new PlayerPair(player, targetId);

		final Integer relation = _relations.get(key);
		if (relation == null || relation == 0)
			return false;

		return (player == key.id1() && (relation & CHAR_BLOCKS_FRIEND) != 0) || (player == key.id2() && (relation & FRIEND_BLOCKS_CHAR) != 0);
	}

	Set<Integer> getBlockList(int playerId)
	{
		final Set<Integer> blockList = new HashSet<>();

		_relations.forEach((playerPair, relation) ->
		{
			if (playerPair.contains(playerId))
			{
				if (playerId == playerPair.id1() && (relation & CHAR_BLOCKS_FRIEND) != 0)
					blockList.add(playerPair.id2());
				else if (playerId == playerPair.id2() && (relation & FRIEND_BLOCKS_CHAR) != 0)
					blockList.add(playerPair.id1());
			}
		});

		return blockList;
	}

	void addToBlockList(int player, int targetId)
	{
		final int playerId = player;
		if (playerId == targetId)
			return;

		updateRelation(playerId, targetId, (playerId < targetId ? CHAR_BLOCKS_FRIEND : FRIEND_BLOCKS_CHAR), true);
	}

	void removeFromBlockList(int player, int targetId)
	{
		if (!isInBlockList(player, targetId))
			return;

		updateRelation(player, targetId, (player < targetId ? CHAR_BLOCKS_FRIEND : FRIEND_BLOCKS_CHAR), false);
	}

	boolean areFriends(int playerId, int targetId)
	{
		final PlayerPair key = new PlayerPair(playerId, targetId);

		final Integer relation = _relations.get(key);
		if (relation == null || relation == 0)
			return false;

		return (relation & ARE_FRIENDS) != 0;
	}

	Set<Integer> getFriendList(int playerId)
	{
		final Set<Integer> friendList = new HashSet<>();

		_relations.forEach((playerPair, relation) ->
		{
			if (playerPair.contains(playerId) && (relation & ARE_FRIENDS) != 0)
				friendList.add((playerId == playerPair.id1()) ? playerPair.id2() : playerPair.id1());
		});

		return friendList;
	}

	void addToFriendList(int player, int targetId)
	{
		final int playerId = player;
		if (playerId == targetId)
			return;

		updateRelation(playerId, targetId, ARE_FRIENDS, true);
	}

	void removeFromFriendList(int player, int targetId)
	{
		if (!areFriends(player, targetId))
			return;

		updateRelation(player, targetId, ARE_FRIENDS, false);
	}

	private void updateRelation(int id1, int id2, int flag, boolean add)
	{
		if (id1 == id2)
			return;

		final PlayerPair key = new PlayerPair(id1, id2);

		_relations.compute(key, (k, oldRelation) ->
		{
			int relation = (oldRelation != null) ? oldRelation : 0;
			if (add)
				relation = relation | flag;
			else
				relation = relation & ~flag;

			return relation;
		});
	}

	// Scenario driver: see README.md for the line format.
	public static void main(String[] args) throws Exception
	{
		final StringBuilder out = new StringBuilder();
		RelationOrderProbe m = null;
		String name = null;
		for (String raw : Files.readAllLines(Path.of(args[0])))
		{
			final String line = raw.strip();
			if (line.isEmpty() || line.startsWith("#"))
				continue;

			final String[] f = line.split("\\s+");
			switch (f[0])
			{
				case "scenario":
					name = f[1];
					m = new RelationOrderProbe();
					continue;
				case "list":
					final int id = Integer.parseInt(f[1]);
					out.append(name).append(" friends ").append(id).append(':');
					for (int x : m.getFriendList(id))
						out.append(' ').append(x);
					out.append('\n');
					out.append(name).append(" blocks ").append(id).append(':');
					for (int x : m.getBlockList(id))
						out.append(' ').append(x);
					out.append('\n');
					continue;
			}

			final int a = Integer.parseInt(f[1]);
			final int b = Integer.parseInt(f[2]);
			int arg = 3;
			final int relation = f[0].equals("load") ? Integer.parseInt(f[arg++]) : 0;
			int stepA = 0, stepB = 0, count = 1;
			if (f.length > arg)
			{
				stepA = Integer.parseInt(f[arg]);
				stepB = Integer.parseInt(f[arg + 1]);
				count = Integer.parseInt(f[arg + 2]);
			}
			for (int i = 0; i < count; i++)
			{
				final int x = a + i * stepA;
				final int y = b + i * stepB;
				switch (f[0])
				{
					case "load" -> m.load(x, y, relation);
					case "friend" ->
					{
						m.addToFriendList(x, y);
						m.addToFriendList(y, x);
					}
					case "unfriend" -> m.removeFromFriendList(x, y);
					case "block" -> m.addToBlockList(x, y);
					case "unblock" -> m.removeFromBlockList(x, y);
					default -> throw new IllegalArgumentException(line);
				}
			}
		}
		System.out.print(out);
	}
}
