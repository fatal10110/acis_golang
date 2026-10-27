package net.sf.l2j.gameserver.model.records;

// Stand-in: the packet base class only names this record type; the probed
// packets never write an effect.
public record EffectHolder(int id, int level, int duration)
{
}
