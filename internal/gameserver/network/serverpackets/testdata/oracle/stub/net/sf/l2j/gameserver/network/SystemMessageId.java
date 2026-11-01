package net.sf.l2j.gameserver.network;

import net.sf.l2j.gameserver.network.serverpackets.SystemMessage;

// Stand-in for the system message id table: a numeric id and the parameter
// count the message declares. The shared no-parameter message is never cached,
// so every probe builds a fresh message.
public final class SystemMessageId
{
	public static final SystemMessageId S1 = new SystemMessageId(109, 1);
	public static final SystemMessageId LOOTING_FINDERS_KEEPERS = new SystemMessageId(1031);
	public static final SystemMessageId LOOTING_RANDOM = new SystemMessageId(1032);
	public static final SystemMessageId LOOTING_RANDOM_INCLUDE_SPOIL = new SystemMessageId(1033);
	public static final SystemMessageId LOOTING_BY_TURN = new SystemMessageId(1034);
	public static final SystemMessageId LOOTING_BY_TURN_INCLUDE_SPOIL = new SystemMessageId(1035);

	private final int _id;
	private int _paramCount;

	public SystemMessageId(int id)
	{
		this(id, 0);
	}

	public SystemMessageId(int id, int paramCount)
	{
		_id = id;
		_paramCount = paramCount;
	}

	public int getId()
	{
		return _id;
	}

	public int getParamCount()
	{
		return _paramCount;
	}

	public void setParamCount(int paramCount)
	{
		_paramCount = paramCount;
	}

	public SystemMessage getStaticSystemMessage()
	{
		return null;
	}

	public void setStaticSystemMessage(SystemMessage sm)
	{
	}

	public static SystemMessageId getSystemMessageId(int id)
	{
		return new SystemMessageId(id);
	}
}
