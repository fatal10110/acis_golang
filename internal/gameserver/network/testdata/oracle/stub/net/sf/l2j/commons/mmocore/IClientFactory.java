package net.sf.l2j.commons.mmocore;

public interface IClientFactory<T>
{
	T create(MMOConnection<T> con);
}
