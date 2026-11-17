# Legacy string hash oracle

`../stringhash.golden` is the expected output of
`TestLegacyStringHashMatchesReferenceProbe` (`../../oracle_test.go`). Each
line is `<hash> <UTF-8 bytes of the string as hex>` (`-` for the empty
string).

`StringHashProbe.java` prints `String.hashCode()` (the reference runtime's,
not a copy) for fixed strings, including supplementary-plane characters that
hash as surrogate pairs, and for every multisell list name in the datapack,
derived the way aCis 409
`aCis_gameserver/java/net/sf/l2j/gameserver/data/xml/MultisellData.java:49`
derives a list id: `file.getName().replaceAll(".xml", "").hashCode()`.

## Regenerating

From the repository root, with the datapack next to the Go repository (or
`ACIS_DATAPACK` set to the datapack root):

```
internal/commons/testdata/oracle/run.sh
```

## Number and placeholder formatting oracle

`../format.golden` is the expected output of `TestFormatMatchesReferenceProbe`
(`../../format_oracle_test.go`), written by `FormatProbe.java` (same `run.sh`).
Lines (strings as hex of UTF-8, `-` for the empty string):

- `double <raw bits hex> <Double.toString>`
- `float <raw bits hex> <Float.toString>`
- `fixed <raw bits hex> <decimals> <String.format(Locale.ENGLISH, "%.Nf")>`
- `html <value hex> <hex of "%a%".replaceAll("%a%", value.replaceAll("\\$", "\\\\\\$"))>`, the
  expression of `NpcHtmlMessage.replace(String, String)`, or `EXC` when the
  replacement grammar rejects the value (a backslash ending it)

Subnormal values are not probed: the JDK prints a subnormal whose shortest
decimal has one digit with the closest two-digit decimal (`Double.MIN_VALUE`
is `4.9E-324`, Go writes `5.0E-324`); no game value reaches them.
