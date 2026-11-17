package net.sf.l2j.commons.mmocore;

import java.nio.ByteBuffer;

public interface IPacketHandler<T>
{
	ReceivablePacket<T> handlePacket(ByteBuffer buf, T client);
}
