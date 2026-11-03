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
