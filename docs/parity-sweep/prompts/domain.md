Domain review run {RUN} for acis_golang #258: {DID} — {title}. Lenses in this run: {lenses}. This review is read-only: change no code, write no tests, open no PRs.

Pinned SHA: {sha}. First, post "started {RUN} at {sha}" on issue #{domain_issue}; launchers use that comment to avoid starting the same run twice. Work from the outer acis_public/ root, in the worktree created by `git -C acis_golang worktree add ../.worktrees/p258-{RUN} {sha}`, and follow docs/parity-sweep/README.md "Repo rules that apply to every run".

Scope: {scope}. For slice IDs, that means the owned files and routed handlers that `python3 <worktree>/docs/parity-sweep/sweep.py body P##` lists for each.
Scope note: {scope_note}
Lens weights for this domain: {weights} (H = main focus, M = check, L = skim). The checklists are in README "Lens checklists".
Start condition: {trigger}. For a bugs run, first read the parity follow-ups of the slices in scope (issues titled "[P##]" with the label parity-sweep), so you do not re-report drift.

Rules:
- Report what is wrong in Go even where it matches Java. Never report Go-vs-Java drift; link the parity finding instead.
- Ownership follows README "Domain reviews".
- Every finding needs a file and line, a concrete failure or cost, a fix size, and a severity on the README's domain scale.
- A performance finding needs a measurement (pprof per docs/profiling.md, tests/perf, cmd/botswarm). Without one, put it under "questions".
- Test runs follow README "Test runs": focused -run on one package at a time. Use -race only where this run's lens needs it, at most one DB-backed package at a time.
- Mark any fix that would change observable behaviour "needs parity check".

Output: post exactly one comment on issue #{domain_issue}, headed "{RUN} at {sha}". Give a findings table per lens (Sev | file:line | finding | failure or cost | fix size | owner), then the questions. Do not file issues. Remove your worktree when done.
