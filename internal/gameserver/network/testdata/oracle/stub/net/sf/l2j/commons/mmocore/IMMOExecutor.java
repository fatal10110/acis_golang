package net.sf.l2j.commons.mmocore;

public interface IMMOExecutor<T>
{
	void execute(ReceivablePacket<T> rp);
}
