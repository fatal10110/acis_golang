Parity review for acis_golang #258, slice {ID} — {title}. This review is read-only: change no code, write no tests, open no PRs.

Pinned SHA: {sha} (recorded on wave epic #{wave_epic}). Work from the outer acis_public/ root, in your own worktree:
`git -C acis_golang worktree add ../.worktrees/p258-{ID} {sha}`, so WT=.worktrees/p258-{ID}. Use `rtk go -C $WT`. Follow docs/parity-sweep/README.md "Repo rules that apply to every run".

Scope: run `python3 $WT/docs/parity-sweep/sweep.py body {ID}`. It lists, at the pinned SHA:
- the owned files;
- the routed handlers, which are your entry points;
- the secondary handlers: entry points owned elsewhere that reach your code;
- the read-only context files.
Java starting point: {java_start}. Evidence packages: {evidence_packages}.

Steps:
1. Follow README "Parity slice method", steps 1–7. Treat each secondary handler as an entry point for the branch that reaches your code.
2. Report only Go-vs-Java drift, rated S0–S3 with the README's severity table.
3. Evidence: cite the CI run and the local evidence comment on #{wave_epic}. Any extra test run is a focused -run on one package with ACIS_REQUIRE_DATAPACK=1 -v. Do not run -race on DB-backed packages (README "Test runs"). A SKIP counts as not run. For a FAIL, apply README's FAIL rule.
4. If you find drift in a context file, report it with `owner: P##`.

Output: post exactly one comment on #{slice_issue}, in the README's report format. Do not file issues. Remove your worktree when done.
