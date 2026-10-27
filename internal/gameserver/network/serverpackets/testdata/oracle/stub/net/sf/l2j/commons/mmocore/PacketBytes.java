package net.sf.l2j.commons.mmocore;

import java.lang.reflect.Method;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.HexFormat;

/**
 * Probe-side serializer: runs a packet's own writeImpl into a little-endian byte
 * buffer and returns the hex of the bytes written (opcode first). It lives in the
 * packet base package to reach the buffer slot without changing any reference class.
 */
public final class PacketBytes
{
	private PacketBytes()
	{
	}

	@SuppressWarnings("rawtypes")
	public static String hex(SendablePacket packet) throws Exception
	{
		final ByteBuffer buf = ByteBuffer.allocate(64 * 1024).order(ByteOrder.LITTLE_ENDIAN);
		packet._buf = buf;

		// writeImpl is invoked directly: the packet base class's write() swallows
		// exceptions, which would turn a failing probe into silent short bytes.
		Class<?> c = packet.getClass();
		while (c != Object.class)
		{
			try
			{
				final Method m = c.getDeclaredMethod("writeImpl");
				m.setAccessible(true);
				m.invoke(packet);
				break;
			}
			catch (NoSuchMethodException e)
			{
				c = c.getSuperclass();
			}
		}
		if (buf.position() == 0)
			throw new IllegalStateException("packet wrote nothing: " + packet.getClass());

		final byte[] body = new byte[buf.position()];
		buf.flip();
		buf.get(body);
		return HexFormat.of().formatHex(body);
	}
}
