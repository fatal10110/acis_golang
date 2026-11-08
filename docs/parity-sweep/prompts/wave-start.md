Start wave {N} of the acis_golang #258 parity sweep (epic #{wave_epic}). This is phase B; it runs after the wave {N} manifest PR has merged, or after phase A reported "manifest unchanged". Do not change Go code. Work from the outer acis_public/ root.

1. Pin:
   - The pinned SHA is the manifest PR's merge commit, or the SHA phase A reported as unchanged.
   - Run `git -C acis_golang fetch origin main` and create the worktree WT=.worktrees/p258-wave{N}: `git -C acis_golang worktree add ../.worktrees/p258-wave{N} <sha>`.
   - Every command below uses WT's copy of sweep.py, manifest.json and issues.json: `S="python3 $WT/docs/parity-sweep/sweep.py"`.
2. Check the manifest at the pinned SHA:
   - Run `python3 $WT/docs/parity-sweep/slicer.py $WT /tmp/m{N}.json`.
   - Compare /tmp/m{N}.json with $WT/docs/parity-sweep/manifest.json, ignoring `sha`, `prod_lines` and `generated_lines`.
   - If file ownership, routed handlers or context differ (main moved between phase A and the merge), stop and rerun phase A.
3. CI: find the go.yml run for the pinned SHA with `gh api "repos/fatal10110/acis_golang/actions/runs?head_sha=<sha>"`.
   - It must have succeeded. If it is still running, wait for it.
   - If it failed, post that on #{wave_epic} and stop. The wave needs a green pinned SHA, so the next phase A picks a newer commit.
4. Changed files since the last pin:
   - Run `git -C $WT diff --name-only <wave {prev} pinned sha>..<sha> -- internal cmd`. For wave 1 there is nothing to compare.
   - For every changed, new, renamed or deleted file that belongs to a slice whose issue is already closed, run `$S delta-create P## --wave {N} --files <comma list> --apply`. The command records the new delta issue in WT's issues.json; phase A of the next wave commits it.
5. Issue bodies: for every slice in this wave, run `$S patch-body P## --apply`, so each body matches the pinned manifest.
6. Post the pinned SHA, the CI run link and the list of delta issues on #{wave_epic}.
7. Local-only evidence. Run only the local-only tests listed in docs/agents/verification.md whose packages are in this wave's evidence_packages:
   - geodata: `geo/probe` and `geo/pathfind`, which find geodata in the outer aCis_datapack/ or $ACIS_GEODATA_DIR;
   - reference: the tests that need the outer aCis_gameserver/.
   Command: `ACIS_REQUIRE_DATAPACK=1 ACIS_REQUIRE_REFERENCE=1 rtk go -C $WT test -count=1 -v <package> -run '<listed tests>'`.
   Post one evidence comment on #{wave_epic}: the CI run link, then for each local test its result. A SKIP counts as not run; give the reason.
8. Launch, with at most 9 agents at once, of which at most 3 may run DB-backed tests at a time:
   - one workflow per slice: `$S prompt P## --sha <sha>`;
   - one per delta issue: `$S delta-prompt <issue> --wave {N} --sha <sha>`;
   - the domain runs due now: `$S runs --wave {N}`, then `$S domain-prompt <run> --sha <sha>`. Also relaunch any earlier run that has a "started" comment but never reported.
