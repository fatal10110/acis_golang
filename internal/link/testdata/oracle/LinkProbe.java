import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.commons.network.AttributeType;
import net.sf.l2j.gameserver.network.gameserverpackets.PlayerInGame;
import net.sf.l2j.gameserver.network.gameserverpackets.ServerStatus;

/**
 * Prints the reference's game server to login server packets (compiled
 * unmodified) as golden lines; formats in README.md.
 */
public final class LinkProbe
{
	private static String hex(byte[] b)
	{
		final StringBuilder s = new StringBuilder();
		for (byte c : b)
			s.append(String.format("%02x", c));
		return s.toString();
	}

	private static void status(ServerStatus ss, List<String> attrs, AttributeType type, int value)
	{
		ss.addAttribute(type, value);
		attrs.add(type.getId() + ":" + value);
	}

	private static void print(ServerStatus ss, List<String> attrs)
	{
		System.out.println("serverstatus " + String.join(",", attrs) + " " + hex(ss.getContent()));
	}

	public static void main(String[] args)
	{
		// one attribute alone: every status, every flag, ages and a player cap
		// (setServerType, setMaxPlayer and sendServerStatus send one at a time)
		for (int status = 0; status <= 5; status++)
			one(AttributeType.STATUS, status);
		for (AttributeType flag : new AttributeType[] { AttributeType.CLOCK, AttributeType.BRACKETS, AttributeType.TEST_SERVER, AttributeType.PVP_SERVER })
			for (int on = 0; on <= 1; on++)
				one(flag, on);
		for (int age : new int[] { 0, 15, 18 })
			one(AttributeType.AGE_LIMIT, age);
		for (int max : new int[] { 0, 1, 300, 5000, Integer.MAX_VALUE })
			one(AttributeType.MAX_PLAYERS, max);

		// the packet sent on AuthResponse: status (GM only or auto), clock, brackets,
		// age limit, test server and pvp server, in this order
		for (int gm = 0; gm <= 1; gm++)
			for (int clock = 0; clock <= 1; clock++)
				for (int brackets = 0; brackets <= 1; brackets++)
					for (int age : new int[] { 0, 15, 18 })
						for (int test = 0; test <= 1; test++)
							for (int pvp = 0; pvp <= 1; pvp++)
							{
								final ServerStatus ss = new ServerStatus();
								final List<String> attrs = new ArrayList<>();
								status(ss, attrs, AttributeType.STATUS, gm == 1 ? 5 : 0);
								ss.addAttribute(AttributeType.CLOCK, clock == 1);
								attrs.add("2:" + clock);
								ss.addAttribute(AttributeType.BRACKETS, brackets == 1);
								attrs.add("3:" + brackets);
								status(ss, attrs, AttributeType.AGE_LIMIT, age);
								ss.addAttribute(AttributeType.TEST_SERVER, test == 1);
								attrs.add("5:" + test);
								ss.addAttribute(AttributeType.PVP_SERVER, pvp == 1);
								attrs.add("6:" + pvp);
								print(ss, attrs);
							}

		// the roster sent after it: every account online, and one account entering
		System.out.println("playeringame alice,bob " + hex(new PlayerInGame(List.of("alice", "bob")).getContent()));
		System.out.println("playeringame carol " + hex(new PlayerInGame("carol").getContent()));
	}

	private static void one(AttributeType type, int value)
	{
		final ServerStatus ss = new ServerStatus();
		final List<String> attrs = new ArrayList<>();
		status(ss, attrs, type, value);
		print(ss, attrs);
	}
}
