import java.lang.reflect.Field;

import net.sf.l2j.commons.geometry.basic.Line2D;
import net.sf.l2j.gameserver.enums.boats.BoatDock;
import net.sf.l2j.gameserver.model.location.Location;
import net.sf.l2j.gameserver.model.location.Point2D;

/**
 * Prints the reference boat docks' data and geometry, one golden line per
 * observation; formats in README.md.
 */
public final class DockProbe
{
	private static long _state = 0x9E3779B97F4A7C15L;

	private static int next(int bound)
	{
		_state ^= _state << 13;
		_state ^= _state >>> 7;
		_state ^= _state << 17;
		return (int) Long.remainderUnsigned(_state, bound);
	}

	private static String line(Line2D l)
	{
		return l.getP1x() + " " + l.getP1y() + " " + l.getP2x() + " " + l.getP2y();
	}

	private static String point(Point2D p)
	{
		return p == null ? "none" : p.getX() + " " + p.getY();
	}

	private static Object field(BoatDock d, String name) throws Exception
	{
		final Field f = BoatDock.class.getDeclaredField(name);
		f.setAccessible(true);
		return f.get(d);
	}

	public static void main(String[] args) throws Exception
	{
		for (BoatDock d : BoatDock.values())
		{
			final Location at = (Location) field(d, "_dockLoc");
			final Location oust = d.getOustLoc();
			System.out.println("dock " + d.name() + " at " + at.getX() + " " + at.getY() + " " + at.getZ()
				+ " oust " + oust.getX() + " " + oust.getY() + " " + oust.getZ()
				+ " entrance " + line(d.getBoatEntrance()) + " exit " + line(d.getBoatExit())
				+ " axis " + line((Line2D) field(d, "_axisLine")) + " exclusive " + d.isBusyOnStart());

			// Walks starting and ending around the entrance, from and to
			// the deck: boarding, exit and adjusted boarding points, from
			// aboard and from the shore, and both coordinate conversions.
			final Line2D in = d.getBoatEntrance();
			final Line2D out = d.getBoatExit();
			for (int i = 0; i < 80; i++)
			{
				final Line2D line = i % 2 == 0 ? in : out;
				// Half the walks cross the line (from a point beside it to a point
				// across it); the rest are free walks around it.
				final int f = next(101);
				final int cx = line.getP1x() + (line.getP2x() - line.getP1x()) * f / 100;
				final int cy = line.getP1y() + (line.getP2y() - line.getP1y()) * f / 100;
				final int rx = next(601) - 300;
				final int ry = next(601) - 300;
				final boolean crossing = i % 4 < 2;
				final int ox = crossing ? cx + rx : in.getP1x() + next(801) - 400;
				final int oy = crossing ? cy + ry : in.getP1y() + next(801) - 400;
				final int dx = crossing ? cx - rx + next(61) - 30 : in.getP2x() + next(801) - 400;
				final int dy = crossing ? cy - ry + next(61) - 30 : in.getP2y() + next(801) - 400;
				sample(d, ox, oy, dx, dy);
			}
		}

		// The Rune walks of the boat passenger tests.
		final BoatDock rune = BoatDock.RUNE;
		final Location tie = (Location) field(rune, "_dockLoc");
		for (int[] w : new int[][]
		{
			{34600, -38100, 34335, -37764},
			{34479, -37947, 34332, -37894},
			{34600, -38100, tie.getX(), tie.getY()},
			{tie.getX(), tie.getY(), 34600, -38100},
			{34506, -37922, 34600, -38100},
			{34519, -37946, 34600, -38100}
		})
			sample(rune, w[0], w[1], w[2], w[3]);
		for (int[] c : new int[][]
		{
			{100, -200},
			{0, -100},
			{281, -92}
		})
			System.out.println("convert RUNE " + c[0] + " " + c[1] + " " + c[0] + " " + c[1] + " => "
				+ point(rune.convertBoatToWorldCoordinates(c[0], c[1])) + " | "
				+ point(rune.convertWorldToBoatCoordinates(c[0], c[1])));
		for (int[] c : new int[][]
		{
			{34519, -37946},
			{34332, -37894},
			{34335, -37764}
		})
			System.out.println("convert RUNE " + c[0] + " " + c[1] + " " + c[0] + " " + c[1] + " => "
				+ point(rune.convertBoatToWorldCoordinates(c[0], c[1])) + " | "
				+ point(rune.convertWorldToBoatCoordinates(c[0], c[1])));
	}

	// board <dock> <inBoat> <ox> <oy> <dx> <dy> => <entrance point> | <exit point> | <adjusted entrance point>
	// convert <dock> <deck x> <deck y> <world x> <world y> => <deck point to world> | <world point to deck>
	private static void sample(BoatDock d, int ox, int oy, int dx, int dy)
	{
		for (int boat = 0; boat <= 1; boat++)
		{
			final Point2D o = new Point2D(ox, oy);
			final Point2D t = new Point2D(dx, dy);
			System.out.println("board " + d.name() + " " + boat + " " + ox + " " + oy + " " + dx + " " + dy + " => "
				+ point(d.getBoardingPoint(o, t, boat == 1))
				+ " | " + point(BoatDock.getBoardingPoint(d.getBoatExit(), o, t, boat == 1))
				+ " | " + point(d.getAdjustedBoardingPoint(o, t, boat == 1)));
		}
		System.out.println("convert " + d.name() + " " + ox % 400 + " " + oy % 400 + " " + dx + " " + dy + " => "
			+ point(d.convertBoatToWorldCoordinates(ox % 400, oy % 400)) + " | "
			+ point(d.convertWorldToBoatCoordinates(dx, dy)));
	}
}
