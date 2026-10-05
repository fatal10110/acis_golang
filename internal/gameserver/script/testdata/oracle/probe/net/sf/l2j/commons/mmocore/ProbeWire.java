package net.sf.l2j.commons.mmocore;

import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.HexFormat;
import java.util.function.BiConsumer;

/**
 * The probe's connection path. {@link #receive} runs a client packet from its body
 * bytes (opcode excluded) the way the selector and the packet executor would.
 * <p>
 * The probe's send path. The build replaces the socket write in the game
 * client's packet send with {@link #capture}, which serializes the packet body exactly
 * as the selector would before encryption (little-endian, opcode first) and hands the
 * client and the hex body to the sink.
 */
public final class ProbeWire
{
	private ProbeWire()
	{
	}

	private static BiConsumer<MMOClient<?>, String> _sink;

	public static void setSink(BiConsumer<MMOClient<?>, String> sink)
	{
		_sink = sink;
	}

	@SuppressWarnings(
	{
		"rawtypes",
		"unchecked"
	})
	public static void capture(MMOClient<?> client, SendablePacket packet)
	{
		final ByteBuffer buf = ByteBuffer.allocate(64 * 1024).order(ByteOrder.LITTLE_ENDIAN);
		packet._buf = buf;
		packet._client = client;
		packet.write();
		packet._buf = null;

		if (_sink == null)
			return;

		final byte[] body = new byte[buf.position()];
		buf.flip();
		buf.get(body);
		_sink.accept(client, packet.getClass().getSimpleName() + " " + HexFormat.of().formatHex(body));
	}
	
	@SuppressWarnings(
	{
		"rawtypes",
		"unchecked"
	})
	public static void receive(MMOClient<?> client, ReceivablePacket packet, byte[] body)
	{
		packet._buf = ByteBuffer.wrap(body).order(ByteOrder.LITTLE_ENDIAN);
		packet._client = client;
		packet._sbuf = new NioNetStringBuffer(64 * 1024);
		if (packet.read())
			packet.run();
		packet._buf = null;
	}
}
