package net.sf.l2j.gameserver.data.xml;

import java.util.ArrayList;
import java.util.Collection;
import java.util.List;

import net.sf.l2j.gameserver.model.records.Henna;

// Stand-in for the symbol table.
public final class HennaData
{
	private static final HennaData INSTANCE = new HennaData();

	public final List<Henna> hennas = new ArrayList<>();

	public static HennaData getInstance()
	{
		return INSTANCE;
	}

	public Collection<Henna> getHennas()
	{
		return hennas;
	}
}
