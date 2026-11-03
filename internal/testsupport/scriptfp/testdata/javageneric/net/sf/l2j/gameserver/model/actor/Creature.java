package net.sf.l2j.gameserver.model.actor;

import java.util.List;
import java.util.function.Consumer;
import java.util.function.Predicate;

public abstract class Creature
{
	public int getZ()
	{
		return 0;
	}
	
	public final <A> void forEachKnownTypeInRadius(Class<A> type, int radius, Consumer<A> action)
	{
	}
	
	public final <A> List<A> getKnownTypeInside(Class<A> type, Predicate<A> filter)
	{
		return List.of();
	}
	
	public <T extends Creature> T getAs(int x, Class<T> type)
	{
		return null;
	}
}
