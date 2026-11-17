import java.nio.ByteBuffer;
import java.nio.ByteOrder;

import net.sf.l2j.commons.mmocore.ReceivablePacket;
import net.sf.l2j.gameserver.network.GameClient;
import net.sf.l2j.gameserver.network.GameClient.GameClientState;
import net.sf.l2j.gameserver.network.GamePacketHandler;

/**
 * Feeds every first opcode (and, behind 0xd0, every second opcode) to the
 * reference GamePacketHandler in each client state and prints the packet class
 * it builds: "<state> <opcode hex> <class>" and "<state> d0:<second hex> <class>".
 * An opcode the handler takes but builds no packet for (and does not count as an
 * unknown packet) prints "none"; an opcode it counts as unknown prints nothing.
 */
public final class OpcodeProbe
{
	private static ReceivablePacket<GameClient> handle(GamePacketHandler handler, GameClient client, int... bytes)
	{
		final ByteBuffer buf = ByteBuffer.allocate(16).order(ByteOrder.LITTLE_ENDIAN);
		for (int b : bytes)
			buf.put((byte) b);
		buf.rewind();
		return handler.handlePacket(buf, client);
	}

	public static void main(String[] args)
	{
		final GamePacketHandler handler = new GamePacketHandler();
		for (GameClientState state : GameClientState.values())
		{
			final GameClient client = new GameClient(null);
			client.setState(state);
			for (int op = 0; op <= 0xff; op++)
			{
				final int unknown = client.unknown;
				final ReceivablePacket<GameClient> msg = handle(handler, client, op, 0, 0, 0, 0, 0, 0, 0, 0, 0);
				if (msg != null || client.unknown == unknown)
					System.out.println(state + " " + String.format("%02x", op) + " " + (msg == null ? "none" : msg.getClass().getSimpleName()));
			}
			for (int second = 0; second <= 0xffff; second++)
			{
				final int unknown = client.unknown;
				final ReceivablePacket<GameClient> msg = handle(handler, client, 0xd0, second & 0xff, second >> 8, 0, 0, 0, 0, 0, 0, 0);
				if (msg != null || client.unknown == unknown)
					System.out.println(state + " d0:" + String.format("%04x", second) + " " + (msg == null ? "none" : msg.getClass().getSimpleName()));
			}
		}

		// with the warehouse deposit disabled by the server, the opcode builds nothing
		net.sf.l2j.Config.ALLOW_WAREHOUSE = false;
		final GameClient client = new GameClient(null);
		client.setState(GameClientState.IN_GAME);
		final ReceivablePacket<GameClient> msg = handle(handler, client, 0x31, 0, 0, 0, 0, 0, 0, 0, 0, 0);
		System.out.println("IN_GAME-no-warehouse 31 " + (msg == null ? "none" : msg.getClass().getSimpleName()));
	}
}
