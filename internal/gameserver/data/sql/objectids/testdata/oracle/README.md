# IdFactory statement oracle

`../statements.golden` is the expected output of `TestBootStatementsMatchReferenceProbe`
(`../../objectids_oracle_test.go`): the SQL the reference IdFactory runs at boot, in
order, one line per statement (`update <sql>`, `prepare <sql>` for the expired
timestamp delete, `query <sql>` for the used id scans).

`IdFactoryProbe.java` compiles the reference `IdFactory.java` unmodified against
the stubs in `stub/` (a recording connection, a silent logger, no thread pool) and
prints what the constructor ran.

Regenerate with `run.sh`.
