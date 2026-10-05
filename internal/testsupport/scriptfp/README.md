# Script fingerprints and the engine call census

This package is slice V1 of `docs/script-engine-plan.md` (section 11, item 2; section 12,
V1 and the proof batch). It holds:

- `reference.golden`: a static fingerprint of every class file under the reference
  server's script trees (`scripting/quest`, `scripting/script`, `scripting/task`): its
  distinctive literals, the engine calls it makes and the parent-call shape of each hook.
- `census.golden`: every engine call those classes make, one row per call target, with
  the Go name `apimap.txt` gives it.
- `apimap.txt`: the hand-kept Go name of each census row. It starts empty; a slice that
  builds the Go call for a row adds the row. The script-facing API is frozen when the
  census has no unmapped row.
- `Check`: compares a Go script package with the fingerprints of the classes it ports.

Fingerprints are read from source text. Nothing is compiled or run.

## Regenerating

```bash
go run ./cmd/scriptfp -java <aCis_gameserver>/java -o internal/testsupport/scriptfp
```

Run it after a reference change, and after editing `apimap.txt` (the census carries the
Go names). `TestCommittedGoldensMatchReference` rebuilds both goldens from the reference
checkout found above the module (as the datapack is found; `ACIS_REQUIRE_DATAPACK=1` turns
a missing checkout into a failure) and requires the committed copies byte for byte.
`TestCommittedGoldensAreConsistent` needs no checkout: the census must be the census of
the committed fingerprints with the committed map.

Committed outputs were generated from the reference at workspace commit
`55ff8a4ec7e186d9816cd549246b4cf1f59c9f12`.

## reference.golden

```
classes <n>
class <key> [extends <parent>]      key: path below scripting/, '.'-separated, as scripts.xml
                                    names it; parent: a script key, else its simple name
  numbers <n> ...                   distinct number literals, ascending
  string "<value>"                  one per distinct string literal, Go-quoted, sorted
  char "<value>"                    one per distinct char literal
  call <row> <sites>                engine call sites per census row
  hook <method> none|direct <args>|helper <args>
```

- **Literals**: every number, string and char literal of the file outside comments and
  annotations. Numbers are canonical decimals with no sign (`0x1F` is `31`, `30_048` is
  `30048`, `3000L` is `3000`, `0.5f` is `0.5`, `2.0` is `2`). `0`, `1` and the empty
  string are left out.
- **Calls**: a call whose declaration is in a reference class outside the script trees.
  Receivers are typed statically: parameters, locals, fields, casts, pattern variables,
  the return types of reference methods, and the elements of library collections and maps
  (for-each variables and lambda parameters included). A row is `Owner.method`, with the
  declaring class (`WorldObject.getObjectId` for a player's object id); `Owner.new` for a
  constructor. A method inherited from a library class is counted on the reference class
  that extends it (`MemoSet.get` for a quest state's variable). Calls to library (JDK)
  types, calls between script classes and parent calls (`super.x(...)`, recorded in the
  hook shape) are left out. A call whose target is not found is `?.method`.
- **Hooks**: every non-static, non-private method of the class's top-level type that
  overrides an ancestor method. `direct` when its body calls `super.<same method>(...)`,
  `helper` when it does not but calls a method of its class that does, else `none`.
  `<args>` lists each such parent call in source order: `same` when it passes the hook's
  (or helper's) parameters unchanged and in order, else `changed`.

`TestHooksMatchProbeManifest` checks the hooks against the bytecode record of the
reference probe (`internal/gameserver/script/testdata/oracle/manifest.golden`): every
script class there has the same overriding methods with the same parent calls and argument
shapes.

## census.golden

```
classes <n> sites <n> rows <n> mapped <n> inline <n> unmapped <n> unresolved <n>
call <row> classes <n> sites <n> go <Go name>|inline|-
```

`-` marks an unmapped row; `?.` rows are the calls the extractor could not resolve.

## Check

```go
problems, err := scriptfp.Check("internal/gameserver/script/quest/q001",
	[]string{"quest.Q001_LettersOfLove"})
```

A family package names every class it ports; their fingerprints are compared as one.
`Check` reads the package's non-test files and reports, naming the classes and the item:

- a number, string or char literal on one side only;
- a call of the classes with no `apimap.txt` row, or whose Go name the package never
  calls (matched on the name after its last `.`); a call mapped from a `Quest` helper row
  that none of the classes makes;
- an `on*` hook whose name or shape differs. The Go hook is the `OnX` field of a `Hooks`
  composite literal (a function literal or a package function); its parent call is a call
  to `X` on any receiver but the hook's first parameter (which is the script: `s.X` is a
  re-entry), with `same` when the arguments are the hook's parameters in order.

So a port keeps the reference's literal values as written (delays in the reference's
unit, page names, counts and chances) and calls the parent as `base.X(s, e)` where the
reference calls `super.onX(...)`. Differences a port cannot avoid (a page name built with
a format verb, an SQL text) are passed as exemptions, written as the problem starts:
`number 906`, `string "x"`, `char "x"`, `call Quest.giveItems`, `hook onTalk none`. An
exemption that matches nothing is reported.
