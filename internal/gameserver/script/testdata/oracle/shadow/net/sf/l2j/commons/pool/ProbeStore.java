package net.sf.l2j.commons.pool;

import java.math.BigDecimal;
import java.math.RoundingMode;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.function.Predicate;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * The rows of the tables a scenario keeps, so a character stored at its logout is restored as
 * written. While tables are kept, every statement on one of them is run against its rows: an
 * insert (with or without an on-duplicate-key update), a replace, an update and a delete change
 * them, and a select reads them back. Only the statement shapes the reference's own SQL uses on
 * these tables are understood (single table, a where clause of column comparisons joined by AND
 * (or &&) and OR, an order by); any other statement on a kept table is a failure the scenario reports.
 * Values are kept as the database would print them: integer columns rounded, a boolean as 1 or
 * 0, strings compared without case.
 */
final class ProbeStore
{
	private ProbeStore()
	{
	}

	/** A table's columns in order, their defaults, which are integer typed, and its primary key. */
	record Schema(List<String> columns, Map<String, String> defaults, Map<String, Boolean> integer, List<String> key)
	{
	}

	private static final Map<String, Schema> SCHEMAS = new HashMap<>();

	/** The kept tables' rows; null while no table is kept. */
	private static Map<String, List<Map<String, String>>> _rows;

	private static final List<String> FAILURES = new ArrayList<>();

	private static final Pattern COLUMN = Pattern.compile("^\\s*`(\\w+)`\\s+(\\w+)(.*)$", Pattern.MULTILINE);
	private static final Pattern DEFAULT = Pattern.compile("\\bDEFAULT\\s+('[^']*'|\\S+?)(?:,|\\s|$)", Pattern.CASE_INSENSITIVE);
	private static final Pattern KEY = Pattern.compile("PRIMARY KEY\\s*\\(([^)]*)\\)", Pattern.CASE_INSENSITIVE);

	/** Records the schema of a CREATE TABLE body. */
	static void schema(String table, String body)
	{
		final List<String> columns = new ArrayList<>();
		final Map<String, String> defaults = new HashMap<>();
		final Map<String, Boolean> integer = new HashMap<>();
		final Matcher c = COLUMN.matcher(body);
		while (c.find())
		{
			final String name = c.group(1).toLowerCase();
			columns.add(name);
			integer.put(name, c.group(2).toUpperCase().endsWith("INT"));
			final Matcher d = DEFAULT.matcher(c.group(3));
			if (d.find())
			{
				final String v = d.group(1);
				defaults.put(name, v.equalsIgnoreCase("NULL") ? null : v.startsWith("'") ? v.substring(1, v.length() - 1) : v);
			}
		}
		final List<String> key = new ArrayList<>();
		final Matcher k = KEY.matcher(body);
		if (k.find())
			for (String col : k.group(1).split(","))
				key.add(col.replace("`", "").strip().toLowerCase());
		SCHEMAS.put(table, new Schema(columns, defaults, integer, key));
	}

	/** Starts keeping the rows of tables, none yet. */
	static void keep(String... tables)
	{
		_rows = new HashMap<>();
		for (String t : tables)
		{
			if (!SCHEMAS.containsKey(t))
				throw new IllegalStateException("no schema for kept table " + t);
			_rows.put(t, new ArrayList<>());
		}
		FAILURES.clear();
	}

	/** Stops keeping rows, dropping them; returns the failures since {@link #keep}. */
	static List<String> forget()
	{
		_rows = null;
		final List<String> out = List.copyOf(FAILURES);
		FAILURES.clear();
		return out;
	}

	/** A select's answer, or the count an update changed. */
	record Result(List<Map<String, String>> rows, int count)
	{
	}

	/**
	 * Runs sql with params against the kept rows; null when sql touches no kept table (or no table
	 * is kept), so the caller answers as before.
	 */
	static Result run(String sql, List<Object> params) throws SQLException
	{
		if (_rows == null || sql == null)
			return null;
		final Parser p = new Parser(sql, params);
		final String table = p.table();
		if (table == null || !_rows.containsKey(table))
			return null;
		try
		{
			return p.run(table, _rows.get(table), SCHEMAS.get(table));
		}
		catch (RuntimeException e)
		{
			FAILURES.add(e.getMessage() + ": " + sql);
			throw new SQLException("probe store: " + e.getMessage(), e);
		}
	}

	/** The database's text of a bound value for a column. */
	static String text(Object v, boolean integer)
	{
		if (v == null)
			return null;
		if (v instanceof Boolean b)
			return b ? "1" : "0";
		final String s = v instanceof Enum<?> e ? e.name() : String.valueOf(v);
		if (integer)
		{
			try
			{
				return new BigDecimal(s.strip()).setScale(0, RoundingMode.HALF_UP).toPlainString();
			}
			catch (NumberFormatException e)
			{
				return s;
			}
		}
		return s;
	}

	/** Compares two stored values as the database does: numbers by value, strings without case. */
	static int compare(String a, String b)
	{
		try
		{
			return new BigDecimal(a.strip()).compareTo(new BigDecimal(b.strip()));
		}
		catch (NumberFormatException e)
		{
			return a.compareToIgnoreCase(b);
		}
	}

	private static final Pattern TOKEN = Pattern.compile("\\s*(`[^`]*`|'(?:[^']|'')*'|<>|!=|<=|>=|&&|[(),=<>*?;+]|-?\\d+(?:\\.\\d+)?|[A-Za-z_][\\w.]*)");

	/** One statement's tokens, read in order; each ? takes the next bound value. */
	private static final class Parser
	{
		private final String _sql;
		private final List<String> _toks = new ArrayList<>();
		private final List<Object> _params;
		private int _at;
		private int _param;

		Parser(String sql, List<Object> params)
		{
			_sql = sql.strip();
			_params = params;
			final Matcher m = TOKEN.matcher(_sql);
			int end = 0;
			while (m.find() && m.start() == end)
			{
				_toks.add(m.group(1));
				end = m.end();
			}
			if (!_sql.substring(end).isBlank())
				_toks.add("\u0000unreadable:" + _sql.substring(end));
		}

		/** The table the statement is on, null for a statement the store never reads. */
		String table()
		{
			if (_toks.isEmpty())
				return null;
			final String verb = _toks.get(0).toUpperCase();
			final int from = switch (verb)
			{
				case "SELECT", "DELETE" -> indexOf("FROM");
				case "INSERT", "REPLACE" -> indexOf("INTO");
				case "UPDATE" -> 0;
				default -> -1;
			};
			if (from < 0 || from + 1 >= _toks.size())
				return null;
			return name(_toks.get(from + 1));
		}

		private int indexOf(String kw)
		{
			for (int i = 0; i < _toks.size(); i++)
				if (_toks.get(i).equalsIgnoreCase(kw))
					return i;
			return -1;
		}

		private static String name(String tok)
		{
			String n = tok.replace("`", "");
			final int dot = n.lastIndexOf('.');
			if (dot >= 0)
				n = n.substring(dot + 1);
			return n.toLowerCase();
		}

		private String peek()
		{
			return _at < _toks.size() ? _toks.get(_at) : "";
		}

		private String next()
		{
			if (_at >= _toks.size())
				throw new IllegalStateException("statement ends early");
			return _toks.get(_at++);
		}

		private boolean accept(String kw)
		{
			if (peek().equalsIgnoreCase(kw))
			{
				_at++;
				return true;
			}
			return false;
		}

		private void expect(String kw)
		{
			if (!accept(kw))
				throw new IllegalStateException("expected " + kw + " at " + peek());
		}

		private String column(Schema s)
		{
			final String tok = next();
			final String c = name(tok);
			if (!s.columns().contains(c))
				throw new IllegalStateException("no column " + tok);
			return c;
		}

		/** A value: a bound one, a quoted string, a number or NULL, as the column keeps it. */
		private String value(boolean integer)
		{
			final String tok = next();
			if (tok.equals("?"))
			{
				if (_param >= _params.size())
					throw new IllegalStateException("too few bound values");
				return text(_params.get(_param++), integer);
			}
			if (tok.startsWith("'"))
				return tok.substring(1, tok.length() - 1).replace("''", "'");
			if (tok.equalsIgnoreCase("NULL"))
				return null;
			if (tok.matches("-?\\d+(?:\\.\\d+)?"))
				return text(tok, integer);
			throw new IllegalStateException("unsupported value " + tok);
		}

		private void end()
		{
			accept(";");
			if (_at != _toks.size())
				throw new IllegalStateException("unsupported clause at " + peek());
			if (_param != _params.size())
				throw new IllegalStateException(_params.size() + " bound values, " + _param + " used");
		}

		Result run(String table, List<Map<String, String>> rows, Schema s)
		{
			final String verb = next().toUpperCase();
			return switch (verb)
			{
				case "SELECT" -> select(rows, s);
				case "INSERT", "REPLACE" -> insert(verb.equals("REPLACE"), rows, s);
				case "UPDATE" -> update(rows, s);
				case "DELETE" -> delete(rows, s);
				default -> throw new IllegalStateException("unsupported statement");
			};
		}

		private Result select(List<Map<String, String>> rows, Schema s)
		{
			final List<String> cols = new ArrayList<>();
			if (accept("*"))
				cols.addAll(s.columns());
			else
			{
				do
					cols.add(column(s));
				while (accept(","));
			}
			expect("FROM");
			next();
			final Predicate<Map<String, String>> where = where(s);
			final List<Map<String, String>> out = new ArrayList<>();
			for (Map<String, String> r : rows)
				if (where.test(r))
					out.add(r);
			if (accept("ORDER"))
			{
				expect("BY");
				final String by = column(s);
				final boolean desc = accept("DESC");
				if (!desc)
					accept("ASC");
				Comparator<Map<String, String>> cmp = Comparator.comparing(r -> r.get(by), Comparator.nullsFirst(ProbeStore::compare));
				out.sort(desc ? cmp.reversed() : cmp);
			}
			end();
			final List<Map<String, String>> answer = new ArrayList<>();
			for (Map<String, String> r : out)
			{
				final Map<String, String> a = new LinkedHashMap<>();
				for (String c : cols)
					a.put(c, r.get(c));
				answer.add(a);
			}
			return new Result(answer, answer.size());
		}

		private Result insert(boolean replace, List<Map<String, String>> rows, Schema s)
		{
			accept("IGNORE");
			expect("INTO");
			next();
			final List<String> cols = new ArrayList<>();
			expect("(");
			do
				cols.add(column(s));
			while (accept(","));
			expect(")");
			expect("VALUES");
			final List<Map<String, String>> fresh = new ArrayList<>();
			do
			{
				final Map<String, String> row = new LinkedHashMap<>();
				for (String c : s.columns())
					row.put(c, s.defaults().get(c));
				expect("(");
				for (int i = 0; i < cols.size(); i++)
				{
					if (i > 0)
						expect(",");
					row.put(cols.get(i), value(s.integer().get(cols.get(i))));
				}
				expect(")");
				fresh.add(row);
			}
			while (accept(","));
			// The on-duplicate-key assignments: the column and the inserted column it takes, or
			// null with the literal value in the second map.
			final Map<String, String> fromInsert = new LinkedHashMap<>();
			final Map<String, String> literal = new LinkedHashMap<>();
			boolean upsert = false;
			if (accept("ON"))
			{
				expect("DUPLICATE");
				expect("KEY");
				expect("UPDATE");
				upsert = true;
				do
				{
					final String c = column(s);
					expect("=");
					if (accept("VALUES"))
					{
						expect("(");
						fromInsert.put(c, column(s));
						expect(")");
					}
					else
						literal.put(c, value(s.integer().get(c)));
				}
				while (accept(","));
			}
			end();
			int count = 0;
			for (Map<String, String> row : fresh)
			{
				final Map<String, String> old = s.key().isEmpty() ? null : rows.stream().filter(r -> sameKey(r, row, s)).findFirst().orElse(null);
				if (old == null)
				{
					rows.add(row);
					count++;
				}
				else if (replace)
				{
					rows.set(rows.indexOf(old), row);
					count += 2;
				}
				else if (upsert)
				{
					for (Map.Entry<String, String> e : fromInsert.entrySet())
						old.put(e.getKey(), row.get(e.getValue()));
					old.putAll(literal);
					count += 2;
				}
				else
					throw new IllegalStateException("duplicate key " + s.key());
			}
			return new Result(null, count);
		}

		private static boolean sameKey(Map<String, String> a, Map<String, String> b, Schema s)
		{
			for (String k : s.key())
			{
				final String x = a.get(k);
				final String y = b.get(k);
				if (x == null || y == null || compare(x, y) != 0)
					return false;
			}
			return true;
		}

		private Result update(List<Map<String, String>> rows, Schema s)
		{
			next();
			expect("SET");
			final Map<String, String> set = new LinkedHashMap<>();
			do
			{
				final String c = column(s);
				expect("=");
				set.put(c, value(s.integer().get(c)));
			}
			while (accept(","));
			final Predicate<Map<String, String>> where = where(s);
			end();
			int count = 0;
			for (Map<String, String> r : rows)
			{
				if (where.test(r))
				{
					r.putAll(set);
					count++;
				}
			}
			return new Result(null, count);
		}

		private Result delete(List<Map<String, String>> rows, Schema s)
		{
			expect("FROM");
			next();
			final Predicate<Map<String, String>> where = where(s);
			end();
			final int before = rows.size();
			rows.removeIf(where);
			return new Result(null, before - rows.size());
		}

		/** An optional where clause; every row when there is none. */
		private Predicate<Map<String, String>> where(Schema s)
		{
			return accept("WHERE") ? or(s) : r -> true;
		}

		private Predicate<Map<String, String>> or(Schema s)
		{
			Predicate<Map<String, String>> p = and(s);
			while (accept("OR"))
				p = p.or(and(s));
			return p;
		}

		private Predicate<Map<String, String>> and(Schema s)
		{
			Predicate<Map<String, String>> p = atom(s);
			// MySQL reads && as AND.
			while (accept("AND") || accept("&&"))
				p = p.and(atom(s));
			return p;
		}

		private Predicate<Map<String, String>> atom(Schema s)
		{
			if (accept("("))
			{
				final Predicate<Map<String, String>> p = or(s);
				expect(")");
				return p;
			}
			final String c = column(s);
			final boolean integer = s.integer().get(c);
			if (accept("IS"))
			{
				final boolean not = accept("NOT");
				expect("NULL");
				return r -> (r.get(c) == null) != not;
			}
			if (accept("IN"))
			{
				expect("(");
				final List<String> vals = new ArrayList<>();
				do
					vals.add(value(integer));
				while (accept(","));
				expect(")");
				return r -> r.get(c) != null && vals.stream().anyMatch(v -> v != null && compare(r.get(c), v) == 0);
			}
			final String op = next();
			if (!List.of("=", "<>", "!=", "<", ">", "<=", ">=").contains(op))
				throw new IllegalStateException("unsupported operator " + op);
			final String v = value(integer);
			return r ->
			{
				final String have = r.get(c);
				if (have == null || v == null)
					return false;
				final int cmp = compare(have, v);
				return switch (op)
				{
					case "=" -> cmp == 0;
					case "<>", "!=" -> cmp != 0;
					case "<" -> cmp < 0;
					case ">" -> cmp > 0;
					case "<=" -> cmp <= 0;
					default -> cmp >= 0;
				};
			};
		}
	}
}
