package net.sf.l2j;

// Stand-in for the reference configuration class: the packet base class reads
// the debug flag, the manor seed limits the drop rate, and the player and
// creature info packets read the display switches (all off, as the shipped
// defaults).
public final class Config
{
	public static boolean PACKET_HANDLER_DEBUG = false;
	public static int RATE_DROP_MANOR = 2;
	public static boolean SHOW_NPC_LVL = false;
	public static boolean SHOW_NPC_CREST = false;
	public static boolean SHOW_SUMMON_CREST = false;
	public static boolean GM_HERO_AURA = false;
	public static int PLAYER_SPAWN_PROTECTION = 0;
	public static int WEAR_PRICE = 10;
	public static double RATE_SIEGE_GUARDS_PRICE = 1.0;
}
