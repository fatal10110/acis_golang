"""#258 parity sweep orchestration: docs/parity-sweep/sweep.py.

Reads manifest.json (from slicer.py) and the prompt templates next to this file, and
  - creates the labels, the 12 epics, the 81 slice issues and the 20 domain issues, and links them as
    sub-issues (bootstrap), recording the numbers in issues.json;
  - renders issue bodies (body) and fills agent prompts (prompt, domain-prompt);
  - lists the domain runs due after a wave closes (runs).

GitHub writes happen only with --apply. Without it every command is a dry run that prints what it would do.
Run from anywhere: paths are resolved relative to this file. Requires an authenticated gh CLI (REST only).

  python3 sweep.py labels [--apply]
  python3 sweep.py bootstrap [--apply]
  python3 sweep.py body P23|D4|W3
  python3 sweep.py prompt P23 --sha <sha>
  python3 sweep.py domain-prompt D4-B --sha <sha>
  python3 sweep.py patch-body P23 [--apply]
  python3 sweep.py runs --wave 4 | --final
  python3 sweep.py wave-prepare 4 | wave-start 4 | wave-triage 4 | domain-triage 4
  python3 sweep.py delta-create P23 --wave 4 --files a.go,b.go [--apply]
  python3 sweep.py delta-prompt <issue> --wave 4 --sha <sha>
Global: --manifest PATH and --issues PATH select the files to use (default: next to this script).
"""
import argparse, json, os, subprocess, sys, time

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = 'fatal10110/acis_golang'
PARENT = 258
MILESTONE = 'M14 - Hardening, parity sign-off, release'
LABELS = {
    'parity-sweep': 'Issue #258 parity sweep: slice issues and their follow-ups',
    'parity-epic': 'Issue #258 parity sweep: wave and domain epics',
    'domain-review': 'Issue #258 domain review issues and their follow-ups',
    'parity-delta': 'Issue #258: review of files that changed after their slice closed',
    'oracle-gap': 'Behaviour without oracle, golden or behaviour-test evidence',
    'accepted-deferral': 'Owner accepted leaving this open when #258 closes',
    **{'wave-%d' % i: 'Issue #258 execution wave %d' % i for i in range(1, 12)},
    **{'sev-S%d' % i: d for i, d in enumerate([
        'Exploit, duplication, crash, disconnect, data loss, or client stuck until relog',
        'Wrong client-visible behaviour in normal play',
        'Drift or defect not seen in normal play',
        'Undocumented simplification, smell or cleanup'])},
}
SLEEP = 1.0  # seconds between GitHub writes (secondary rate limits)


PATHS = {'manifest': os.path.join(HERE, 'manifest.json'), 'issues': os.path.join(HERE, 'issues.json')}


def load(name):
    with open(os.path.join(HERE, name)) as fh:
        return fh.read()


def manifest():
    with open(PATHS['manifest']) as fh:
        return json.load(fh)


def issues_map():
    if not os.path.exists(PATHS['issues']):
        return {}
    with open(PATHS['issues']) as fh:
        return json.load(fh)


def save_issues(m):
    with open(PATHS['issues'], 'w') as fh:
        json.dump(m, fh, indent=1, sort_keys=True)
        fh.write('\n')


def gh(method, path, fields=None, apply=False, tolerate=()):
    """One REST call. Dry run prints it. A failure whose message contains a `tolerate` string is
    returned as None (for example an already-linked sub-issue); any other failure stops the run.
    State already written to issues.json survives, so a rerun resumes where this one stopped."""
    if not apply:
        print('DRY %s %s %s' % (method, path, json.dumps(fields or {})[:160]))
        return {'number': 0, 'id': 0}
    cmd = ['gh', 'api', '-X', method, path, '--input', '-']
    r = subprocess.run(cmd, input=json.dumps(fields or {}), capture_output=True, text=True)
    time.sleep(SLEEP)
    if r.returncode != 0:
        msg = (r.stderr + r.stdout).strip()
        if any(t in msg for t in tolerate):
            return None
        sys.exit('gh %s %s failed: %s\nRerun the same command: finished steps are recorded in %s.' % (
            method, path, msg, PATHS['issues']))
    return json.loads(r.stdout or '{}')


def existing_by_title(apply):
    """Issues already on GitHub with a sweep label, keyed by exact title (guards against duplicates
    when a create succeeded but its result was lost)."""
    if not apply:
        return {}
    found = {}
    for label in ('parity-epic', 'parity-sweep', 'domain-review', 'parity-delta'):
        out = subprocess.run(['gh', 'api', '--paginate', 'repos/%s/issues?state=all&per_page=100&labels=%s' % (REPO, label),
                              '--jq', '.[] | select(.pull_request == null) | [.title, .number, .id] | @tsv'],
                             capture_output=True, text=True)
        for line in out.stdout.splitlines():
            t, n, i = line.split('\t')
            found[t] = {'number': int(n), 'id': int(i)}
    return found


def prompt_file(sid):
    if sid == 'P68':
        return 'prompts/quest-triage.md'
    if 'P69' <= sid <= 'P79':
        return 'prompts/quest-band.md'
    return 'prompts/slice.md'


def slice_by_id(m, sid):
    return next(s for s in m['slices'] if s['id'] == sid)


def bullet(items):
    return '\n'.join('- `%s`' % x for x in items) if items else '- none'


def slice_body(m, s, im):
    wave_epic = im.get('W%d' % s['wave'], '?')
    extra = ''
    if s['id'] == 'P05':
        extra = ('\n**Kept handlers (review these):**\n%s\n\n<details><summary>Handlers in P05 files that other slices '
                 'review (do not review)</summary>\n\n%s\n</details>\n' % (bullet(s['kept_handlers']), bullet(s['routed_out_handlers'])))
    prompt = prompt_file(s['id'])
    return f"""Parity slice **{s['id']} — {s['title']}** of #{PARENT}. Wave {s['wave']} (epic #{wave_epic}).

Generated by `docs/parity-sweep/sweep.py` from `manifest.json` (slicer {m['slicer']}, {m['sha'] or 'unknown sha'}). Procedure: `docs/parity-sweep/README.md`; agent prompt: `docs/parity-sweep/{prompt}`. The review runs at the SHA pinned on the wave epic.

**Java starting point:** {s['java_start']}
**Evidence packages:** {', '.join('`%s`' % p for p in s['evidence_packages']) or 'none'}

**Routed handlers (review the function, not its host file):**
{bullet(s['routed_handlers'])}
{extra}
<details><summary>Owned files ({len(s['files'])}, {s['prod_lines']:,} lines)</summary>

{bullet(s['files'])}
</details>

<details><summary>Read-only context files ({len(s['context_files'])})</summary>

{bullet(s['context_files'])}
</details>

Closes when the wave triage has filed or linked every finding (follow-ups are sub-issues of this issue).
"""


def domain_body(m, d):
    runs = [r for r in m['domain_runs'] if r['domain'] == d['id']]
    w = ', '.join('%s %s' % (k, v) for k, v in d['weights'].items())
    scope = d['slices'] if isinstance(d['slices'], str) else ', '.join(d['slices'])
    run_lines = '\n'.join('- [ ] `%s`: %s (start: %s)' % (r['run'], ', '.join(r['lenses']), r['trigger']) for r in runs)
    return f"""Domain review **{d['id']} — {d['title']}** of #{PARENT}.

**Slices:** {scope}
**Lens weights:** {w}
{('**Scope note:** ' + d['scope_note']) if d['scope_note'] else ''}

Procedure: `docs/parity-sweep/README.md` (domain sections); agent prompt: `docs/parity-sweep/prompts/domain.md`. Findings are filed by the domain triage as sub-issues of this issue.

**Runs:**
{run_lines}
"""


def wave_body(m, wv):
    return f"""Execution wave {wv['wave']} of #{PARENT}: {wv['title']}.

Slices: {', '.join(wv['slices'])}.

Run `docs/parity-sweep/prompts/wave-start.md`, then one workflow per slice, then `prompts/wave-triage.md`.
- [ ] Manifest refreshed and pinned SHA recorded
- [ ] Evidence comment posted
- [ ] All slice reviews posted
- [ ] Triage done; slice and delta issues closed
"""


def cmd_labels(apply):
    existing = set()
    if apply:
        out = subprocess.run(['gh', 'api', '--paginate', 'repos/%s/labels?per_page=100' % REPO, '--jq', '.[].name'],
                             capture_output=True, text=True)
        existing = set(out.stdout.split('\n'))
    for name, desc in LABELS.items():
        if name not in existing:
            gh('POST', 'repos/%s/labels' % REPO, {'name': name, 'description': desc[:100], 'color': 'c5def5'}, apply)


def milestone_number(apply):
    if not apply:
        return 0
    out = subprocess.run(['gh', 'api', 'repos/%s/milestones?state=all&per_page=100' % REPO], capture_output=True, text=True)
    return next(x['number'] for x in json.loads(out.stdout) if x['title'] == MILESTONE)


def create(key, title, body, labels, im, ms, apply, existing):
    """Create the issue once: reuse the issues.json entry, else an existing issue with the same title."""
    if key in im:
        return im[key]
    if title in existing:
        im[key] = dict(existing[title], linked=False)
    else:
        r = gh('POST', 'repos/%s/issues' % REPO, {'title': title, 'body': body, 'labels': labels,
                                                  'milestone': ms or None}, apply)
        im[key] = {'number': r['number'], 'id': r['id'], 'linked': False}
    if apply:
        save_issues(im)
    return im[key]


def link(key, parent_number, im, apply):
    """Link im[key] under parent_number once; 'already' answers count as done."""
    child = im[key]
    if child.get('linked'):
        return
    gh('POST', 'repos/%s/issues/%d/sub_issues' % (REPO, parent_number), {'sub_issue_id': child['id']}, apply,
       tolerate=('already', 'duplicate sub-issue', 'may only have one parent'))
    child['linked'] = True
    if apply:
        save_issues(im)


def cmd_bootstrap(apply):
    """Idempotent: rerunning after a failure creates only what is missing and links only what is unlinked.
    Also creates issues for slices or domains added to the manifest later (P82, ...)."""
    m, im = manifest(), issues_map()
    ms = milestone_number(apply)
    cmd_labels(apply)
    existing = existing_by_title(apply)
    count = 0
    for wv in m['waves']:
        k = 'W%d' % wv['wave']
        new = k not in im
        create(k, '#%d parity wave %d — %s' % (PARENT, wv['wave'], wv['title']), wave_body(m, wv),
               ['parity-epic', 'wave-%d' % wv['wave']], im, ms, apply, existing)
        link(k, PARENT, im, apply)
        count += new
    new = 'DOMAINS' not in im
    create('DOMAINS', '#%d domain reviews' % PARENT,
           'Domain review epic of #%d: D1–D20. See docs/parity-sweep/README.md.' % PARENT,
           ['parity-epic', 'domain-review'], im, ms, apply, existing)
    link('DOMAINS', PARENT, im, apply)
    count += new
    numbers = {k: v['number'] for k, v in im.items()}
    for s in m['slices']:
        new = s['id'] not in im
        create(s['id'], '#%d parity %s — %s' % (PARENT, s['id'], s['title']), slice_body(m, s, numbers),
               ['parity-sweep', 'wave-%d' % s['wave']], im, ms, apply, existing)
        link(s['id'], im['W%d' % s['wave']]['number'], im, apply)
        count += new
    for d in m['domains']:
        new = d['id'] not in im
        create(d['id'], '#%d domain %s — %s' % (PARENT, d['id'], d['title']), domain_body(m, d),
               ['domain-review'], im, ms, apply, existing)
        link(d['id'], im['DOMAINS']['number'], im, apply)
        count += new
    print('%s %d new issues; map in %s' % ('created' if apply else 'would create', count, PATHS['issues']))


def need(im, *keys):
    missing = [k for k in keys if k not in im]
    if missing:
        sys.exit('%s not in %s: run bootstrap (with --apply) first, or pass --issues' % (', '.join(missing), PATHS['issues']))


def cmd_patch_body(target, apply):
    m, im = manifest(), issues_map()
    need(im, target)
    numbers = {k: v['number'] for k, v in im.items()}
    s = slice_by_id(m, target)
    gh('PATCH', 'repos/%s/issues/%d' % (REPO, im[target]['number']), {'body': slice_body(m, s, numbers)}, apply)


def cmd_delta_create(target, files, wave, apply):
    """Open 'P## delta' under the wave epic for files that changed after slice `target` closed."""
    m, im = manifest(), issues_map()
    key = '%s-delta-w%d' % (target, wave)
    need(im, target, 'W%d' % wave)
    if key in im:
        print('exists: #%d' % im[key]['number'])
        return
    s = slice_by_id(m, target)
    body = (f"Delta review for **{target} — {s['title']}** (slice issue #{im[target]['number']}), wave {wave}.\n\n"
            f"These files changed after the slice review. Review only them, with `docs/parity-sweep/prompts/delta.md`.\n\n"
            + bullet(files) + "\n\nFollow-ups are filed as sub-issues of this issue.\n")
    create(key, '#%d parity %s delta — wave %d' % (PARENT, target, wave), body,
           ['parity-sweep', 'parity-delta', 'wave-%d' % wave], im, milestone_number(apply), apply, existing_by_title(apply))
    link(key, im['W%d' % wave]['number'], im, apply)


def fill(template, values):
    class Keep(dict):
        def __missing__(self, k):
            return '{' + k + '}'
    return template.format_map(Keep(values))


def cmd_prompt(sid, sha):
    m, im = manifest(), issues_map()
    s = slice_by_id(m, sid)
    num = lambda k: im.get(k, {}).get('number', '?')
    tpl = load(prompt_file(sid))
    print(fill(tpl, dict(ID=sid, title=s['title'], sha=sha, wave_epic=num('W%d' % s['wave']), slice_issue=num(sid),
                         p68_issue=num('P68'), java_start=s['java_start'],
                         evidence_packages=' '.join(s['evidence_packages']) or 'none')))


def cmd_domain_prompt(run, sha):
    m, im = manifest(), issues_map()
    r = next(x for x in m['domain_runs'] if x['run'] == run)
    d = next(x for x in m['domains'] if x['id'] == r['domain'])
    scope = r['scope'] if isinstance(r['scope'], str) else ', '.join(r['scope'])
    print(fill(load('prompts/domain.md'), dict(
        RUN=run, DID=d['id'], title=d['title'], lenses=', '.join(r['lenses']), sha=sha, scope=scope,
        weights=', '.join('%s %s' % kv for kv in d['weights'].items()), scope_note=d['scope_note'] or 'none',
        domain_issue=im.get(d['id'], {}).get('number', '?'), trigger=r['trigger'])))


def cmd_wave_prompt(kind, wave, sha, delta_issue=None):
    im = issues_map()
    num = lambda k: im.get(k, {}).get('number', '?')
    tpl = {'prepare': 'prompts/wave-prepare.md', 'start': 'prompts/wave-start.md', 'triage': 'prompts/wave-triage.md',
           'domain-triage': 'prompts/domain-triage.md', 'delta': 'prompts/delta.md'}[kind]
    last = max(w['wave'] for w in manifest()['waves'])
    print(fill(load(tpl), dict(N=wave, prev=wave - 1, last=last, wave_epic=num('W%d' % wave), domain_epic=num('DOMAINS'),
                               d20_issue=num('D20'),
                               sha=sha, delta_issue=delta_issue or '{delta_issue}')))


def cmd_runs(wave, final):
    """Runs that become due at the start of `wave`: wave 1 starts every 'any' run; wave N starts the runs
    due after wave N-1. --final lists the runs due after the last wave."""
    m = manifest()
    last = max(w['wave'] for w in m['waves'])
    due_after = last if final else wave - 1
    for r in m['domain_runs']:
        t = r['trigger']
        if (t == 'any' and wave == 1 and not final) or t == 'after-wave-%d' % due_after:
            print(r['run'], ','.join(r['lenses']), t)


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument('cmd', choices=['labels', 'bootstrap', 'body', 'patch-body', 'prompt', 'domain-prompt', 'runs',
                                   'wave-prepare', 'wave-start', 'wave-triage', 'domain-triage', 'delta-prompt',
                                   'delta-create'])
    p.add_argument('target', nargs='?')
    p.add_argument('--apply', action='store_true')
    p.add_argument('--sha', default='{sha}')
    p.add_argument('--wave', type=int, default=0)
    p.add_argument('--final', action='store_true')
    p.add_argument('--files', default='')
    p.add_argument('--manifest', help='manifest.json to read (default: next to this script)')
    p.add_argument('--issues', help='issues.json to read and write (default: next to this script)')
    a = p.parse_args()
    if a.manifest:
        PATHS['manifest'] = os.path.abspath(a.manifest)
    if a.issues:
        PATHS['issues'] = os.path.abspath(a.issues)
    if a.cmd == 'labels':
        cmd_labels(a.apply)
    elif a.cmd == 'bootstrap':
        cmd_bootstrap(a.apply)
    elif a.cmd == 'body':
        m, im = manifest(), {k: v['number'] for k, v in issues_map().items()}
        t = a.target
        if t.startswith('P'):
            print(slice_body(m, slice_by_id(m, t), im))
        elif t.startswith('D'):
            print(domain_body(m, next(d for d in m['domains'] if d['id'] == t)))
        else:
            print(wave_body(m, m['waves'][int(t[1:]) - 1]))
    elif a.cmd == 'prompt':
        cmd_prompt(a.target, a.sha)
    elif a.cmd == 'domain-prompt':
        cmd_domain_prompt(a.target, a.sha)
    elif a.cmd == 'runs':
        cmd_runs(a.wave, a.final)
    elif a.cmd == 'patch-body':
        cmd_patch_body(a.target, a.apply)
    elif a.cmd == 'delta-create':
        if not a.wave or not a.files:
            sys.exit('delta-create needs --wave and --files a.go,b.go')
        cmd_delta_create(a.target, a.files.split(','), a.wave, a.apply)
    elif a.cmd == 'delta-prompt':
        if not a.wave:
            sys.exit('delta-prompt needs --wave')
        cmd_wave_prompt('delta', a.wave, a.sha, a.target)
    else:
        cmd_wave_prompt(a.cmd.replace('wave-', ''), int(a.target), a.sha)


if __name__ == '__main__':
    main()
