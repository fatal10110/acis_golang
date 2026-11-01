# Clan registry order oracle

`../order.golden` is the expected output of `TestRegistryOrderMatchesReferenceProbe`
(`../../order_oracle_test.go`).

`run.sh` cuts two field declarations **verbatim** out of
`aCis_gameserver/java/net/sf/l2j/gameserver/model/pledge/Clan.java` (only CRLF line ends are
normalized) and compiles them unchanged inside the harness `ClanCopy.java.tmpl`:

- `_atWarWith = ConcurrentHashMap.newKeySet()` (the war set; the attacker set is declared
  the same way),
- `_subPledges = new ConcurrentHashMap<>()` (the sub-unit registry).

`OrderProbe.java` fills them and prints their iteration order on the real JDK 21 collections.

Line formats:

- `war <ids, ascending> => <iteration order>`: war sets of 1 to 100 clans, with ids spread
  over 64 to 4 million values from `0x10000000`, so tables of 16 to 256 buckets and every
  doubling are crossed. Ascending insertion is how boot restores the stored rows.
- `ops <+offset|-offset, ...> => <iteration order>`: offsets from `0x10000000`. The first adds
  ascend (the boot restore); later `+` are wars declared at runtime (random order, so a war
  goes to the tail of its bucket) and `-` are wars ended (the table never shrinks). Strides of
  16 to 4096 between the ids crowd buckets: past eight ids a bucket makes a table of fewer than
  64 buckets grow early, and from 64 buckets it becomes a tree bucket (newest id first). The
  random lines of `ops(...)` stop removing once a tree bucket exists (read through
  reflection, hence `--add-opens` in `run.sh`); the `treeOps(...)` lines and the two fixed
  lines at the end then end and declare wars in tree buckets. A tree bucket (a JDK 21
  `ConcurrentHashMap.TreeBin`) losing an id turns back into a chain when
  `root.right == null || root.left == null || root.left.left == null` holds before the
  red-black unlink (`TreeBin.removeTreeNode` returning true, then `untreeify(first)`), so the
  shape of the tree (insertion order, `balanceInsertion`/`balanceDeletion`) decides it. A war
  declared after that lands at the front of the bucket (still a tree) or its tail (a chain).
  The two fixed lines grow a 256-bucket table past a tree bucket whose ids all stay in one
  half: `transfer` keeps that `TreeBin` as it is (`(hc != 0) ? new TreeBin<K,V>(lo) : t`),
  and the shrink then chains (first line) or keeps the tree (second) where a tree rebuilt in
  listing order would not.
- `sub <types, ascending> => <iteration order of the registry>`: every subset of the seven
  sub-unit types.

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/clan/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`.
