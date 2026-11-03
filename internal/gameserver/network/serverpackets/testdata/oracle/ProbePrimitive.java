import java.awt.Color;
import java.lang.reflect.Field;
import java.util.Collections;
import java.util.LinkedHashSet;
import java.util.List;

import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.location.Location;
import net.sf.l2j.gameserver.model.zone.ZoneForm;
import net.sf.l2j.gameserver.model.zone.form.ZoneCuboid;
import net.sf.l2j.gameserver.model.zone.form.ZoneCylinder;
import net.sf.l2j.gameserver.model.zone.form.ZoneNPoly;
import net.sf.l2j.gameserver.network.serverpackets.ExServerPrimitive;
import net.sf.l2j.gameserver.network.serverpackets.L2GameServerPacket;

/**
 * ExServerPrimitive, the debug drawing packet, and the zone outlines the reference forms
 * draw into it (packets_oracle_primitive_test.go mirrors each case by name).
 *
 * The packet keeps its points and lines in identity-hashed sets, so with more than one of
 * either its wire order is arbitrary. Every packet a case fills with more is made
 * insertion-ordered first, by swapping those sets for ordered ones: the bytes then follow
 * the order the forms add their strokes in, which is the order the Go drawing keeps.
 * Continuation packets the reference creates on its own hold at most one point and one
 * line here, so their order is fixed regardless.
 */
final class ProbePrimitive
{
	static final Location ANCHOR = new Location(-71338, 258271, -16000);

	static ExServerPrimitive ordered(ExServerPrimitive p) throws Exception
	{
		for (String name : new String[] { "_points", "_lines" })
		{
			final Field f = ExServerPrimitive.class.getDeclaredField(name);
			f.setAccessible(true);
			f.set(p, Collections.synchronizedSet(new LinkedHashSet<>()));
		}
		return p;
	}

	// Sends p as the handler does and emits every packet that went out, name/0, name/1...
	static void send(String name, ExServerPrimitive p) throws Exception
	{
		final Player player = new Player();
		p.sendTo(player);
		final List<L2GameServerPacket> sent = player.sent;
		for (int i = 0; i < sent.size(); i++)
			PacketProbe.emit(name + "/" + i, sent.get(i));
	}

	static void outline(String name, ZoneForm form, String info) throws Exception
	{
		final ExServerPrimitive p = ordered(new ExServerPrimitive("ZONE", ANCHOR));
		form.visualizeZone(info, p);
		send("ExServerPrimitive/" + name, p);
	}

	static void run() throws Exception
	{
		// An empty drawing: the white point at the anchor.
		send("ExServerPrimitive/empty", new ExServerPrimitive("ZONE", ANCHOR));

		// One point and one line, written points first.
		final ExServerPrimitive one = new ExServerPrimitive("dbg", new Location(1, -2, 3));
		one.addLine("", Color.YELLOW, false, -10, 20, -30, 40, -50, 60);
		one.addPoint("Pté", new Color(0x12, 0x34, 0x56), true, 7, -8, 9);
		send("ExServerPrimitive/point-line", one);

		// The zone outlines.
		outline("cuboid", new ZoneCuboid(200, -100, 400, -300, -3000, -3500), "PeaceZone[2]");
		outline("cylinder", new ZoneCylinder(-71338, 258271, -3700, -3100, 300), "TownZone[11]");
		outline("cylinder-small", new ZoneCylinder(10, 20, 0, 100, 7), "ArenaZone[3]");
		outline("npoly", new ZoneNPoly(new int[] { 0, 1000, 1500, -200 }, new int[] { 0, 0, 900, 700 }, 100, -100), "SwampZone[40]");

		// An outline past one packet's capacity continues in a second one, named ZONE1. Its
		// root is ordered; the continuation is made ordered before it fills.
		final ExServerPrimitive big = ordered(new ExServerPrimitive("ZONE", ANCHOR));
		final Field next = ExServerPrimitive.class.getDeclaredField("_next");
		next.setAccessible(true);
		next.set(big, ordered(new ExServerPrimitive(big)));
		final ZoneForm wide = new ZoneCylinder(1000, -2000, -500, 500, 1000);
		wide.visualizeZone("PeaceZone[2]", big);
		send("ExServerPrimitive/chain", big);

		// Emptied, both packets go out blank; the continuation is then dropped.
		big.reset();
		send("ExServerPrimitive/chain-reset", big);
		send("ExServerPrimitive/chain-reset-again", big);
	}
}
