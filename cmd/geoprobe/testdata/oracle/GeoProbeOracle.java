import java.io.BufferedReader;
import java.io.FileWriter;
import java.io.IOException;
import java.io.InputStreamReader;
import java.io.PrintWriter;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.TreeMap;

import net.sf.l2j.Config;
import net.sf.l2j.gameserver.geoengine.GeoEngine;
import net.sf.l2j.gameserver.geoengine.pathfinding.Node;
import net.sf.l2j.gameserver.model.location.Location;

/**
 * Standalone oracle-dump generator: reads geoprobe query IDs (one per line, the
 * first tab-separated column of a geoprobe dump, or a bare ID) and re-evaluates
 * each against the reference GeoEngine, writing answers in the same dump format
 * geoprobe's -expected-dump flag consumes.
 *
 * Usage: java -cp <gameserver classes> GeoProbeOracle <geodataPath> <queriesFile> <outFile>
 */
public final class GeoProbeOracle
{
	public static void main(String[] args) throws Exception
	{
		if (args.length != 3)
		{
			System.err.println("usage: GeoProbeOracle <geodataPath> <queriesFile> <outFile>");
			System.exit(2);
		}

		Config.GEODATA_PATH = args[0];
		Config.GEODATA_TYPE = net.sf.l2j.gameserver.enums.GeoType.L2OFF;
		Config.MAX_GEOPATH_FAIL_COUNT = 50;
		Config.PART_OF_CHARACTER_HEIGHT = 75;
		Config.MAX_OBSTACLE_HEIGHT = 32;
		Config.MOVE_WEIGHT = 10;
		Config.MOVE_WEIGHT_DIAG = 14;
		Config.OBSTACLE_WEIGHT = 30;
		Config.OBSTACLE_WEIGHT_DIAG = (int) (Config.OBSTACLE_WEIGHT * Math.sqrt(2));
		Config.HEURISTIC_WEIGHT = 12;
		Config.MAX_ITERATIONS = 10000;

		List<String> ids = new ArrayList<>();
		try (BufferedReader r = new BufferedReader(new InputStreamReader(Files.newInputStream(Paths.get(args[1])), StandardCharsets.UTF_8)))
		{
			String line;
			while ((line = r.readLine()) != null)
			{
				if (line.isEmpty())
					continue;
				int tab = line.indexOf('\t');
				ids.add(tab < 0 ? line : line.substring(0, tab));
			}
		}

		// Trigger geodata load.
		GeoEngine engine = GeoEngine.getInstance();

		List<String> lines = new ArrayList<>(ids.size());
		for (String id : ids)
			lines.add(formatRecordLine(id, evaluate(engine, id)));

		lines.sort(String::compareTo);

		try (PrintWriter w = new PrintWriter(new FileWriter(args[2], StandardCharsets.UTF_8)))
		{
			for (String line : lines)
				w.print(line);
		}

		System.err.println("evaluated " + ids.size() + " queries -> " + args[2]);
	}

	private static Map<String, String> evaluate(GeoEngine engine, String id)
	{
		Map<String, String> fields = new TreeMap<>();
		int colon = id.indexOf(':');
		String category = id.substring(0, colon);
		String rest = id.substring(colon + 1);

		if (category.equals("height"))
		{
			int[] p = parsePoint(rest);
			fields.put("height", Integer.toString(engine.getHeight(p[0], p[1], p[2])));
			return fields;
		}

		String[] parts = rest.split("->", 2);
		int[] from = parsePoint(parts[0]);
		int[] to = parsePoint(parts[1]);

		switch (category)
		{
			case "canmove":
				fields.put("result", Boolean.toString(engine.canMoveToTarget(from[0], from[1], from[2], to[0], to[1], to[2])));
				break;
			case "los":
				boolean result = engine.canSee(from[0], from[1], from[2], 0, to[0], to[1], to[2], 0, null, null)
					&& engine.canSee(to[0], to[1], to[2], 0, from[0], from[1], from[2], 0, null, null);
				fields.put("result", Boolean.toString(result));
				break;
			case "validlocation":
				Location v = engine.getValidLocation(from[0], from[1], from[2], to[0], to[1], to[2], null);
				fields.put("x", Integer.toString(v.getX()));
				fields.put("y", Integer.toString(v.getY()));
				fields.put("z", Integer.toString(v.getZ()));
				break;
			case "path":
				List<Location> path = engine.findPath(from[0], from[1], from[2], to[0], to[1], to[2], false, null);
				fields.put("found", Boolean.toString(!path.isEmpty()));
				if (!path.isEmpty())
				{
					// The last corner point is the target node the A* search
					// actually reached; its accumulated getCostG() is the
					// reference PathFinder's own total path cost, read
					// straight off the search result rather than recomputed.
					Node target = (Node) path.get(path.size() - 1);
					fields.put("cost", Integer.toString(target.getCostG()));
					fields.put("points", formatPath(path));
				}
				break;
			default:
				throw new IllegalArgumentException("unknown category " + category);
		}
		return fields;
	}

	private static String formatPath(List<Location> path)
	{
		StringBuilder sb = new StringBuilder();
		for (int i = 0; i < path.size(); i++)
		{
			if (i > 0)
				sb.append(';');
			Location p = path.get(i);
			sb.append(p.getX()).append(',').append(p.getY()).append(',').append(p.getZ());
		}
		return sb.toString();
	}

	private static int[] parsePoint(String s)
	{
		String[] parts = s.split(",");
		return new int[] { Integer.parseInt(parts[0]), Integer.parseInt(parts[1]), Integer.parseInt(parts[2]) };
	}

	private static String formatRecordLine(String id, Map<String, String> fields)
	{
		StringBuilder sb = new StringBuilder(id);
		for (Map.Entry<String, String> e : fields.entrySet())
			sb.append('\t').append(e.getKey()).append('=').append(e.getValue());
		sb.append('\n');
		return sb.toString();
	}
}
