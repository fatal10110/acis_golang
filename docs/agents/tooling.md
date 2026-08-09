# Agent tooling policy

Use the smallest local tool that answers the question and bound its output. The command forms below
were verified against the installed tools on 2026-07-26.

Run every example from the outer `acis_public/` root. Examples use `acis_golang` for the primary
checkout; in a linked worktree, substitute its outer-root-relative path as `<go-root>` in
`rtk go -C <go-root>` and `git -C <go-root>`. The datapack remains `aCis_datapack/` at the outer root;
never resolve it as a sibling of `<go-root>`. Do not change the persistent shell directory.

Choose the search primitive before writing a command. Do not default to `find`, POSIX `grep`, or
`sed` for repository discovery, code search, or file reading:

- filenames or path patterns: `rg --files` with bounded globs;
- known exact text: `rg` in a constrained path;
- behavior with unknown names or locations: grepai search;
- Go definitions, references, implementations, diagnostics, or calls: `gopls`;
- caller/callee graphs across Go or Java: grepai trace;
- syntax-shaped matches or repetitive structural rewrites: ast-grep;
- known file and range: the native file-read tool.

After one exact `rg` miss, change tool categories; do not retry variants of `find`, `grep`, or `sed`.
Use `find` only for filesystem metadata that `rg --files` cannot express, POSIX `grep` only when `rg`
is unavailable, and `sed` only for a tiny known range when no native read tool exists. RTK is for
compressing noisy tests, builds, vet, lint, Git, and logs—not for choosing search tools.

| Tool | Use it for | Do not use it for | Fallback and output bound |
| --- | --- | --- | --- |
| `rg` | exact identifiers, opcodes, strings, constants, errors; `rg --files` for filenames | semantic guesses or call graphs | constrain path/glob and line range; use grepai when names are unknown |
| grepai | semantic location and caller/callee relationships across Go and Java | known exact text or whole-file dumping | `rg`, LSP, targeted reads; limit results to about five |
| `gopls` | Go definitions, references, implementations, diagnostics, call hierarchy | Java research or bulk text replacement | `rg` plus targeted reads; request one symbol/file at a time |
| JDT LS | Java definitions, references, implementations, type and call hierarchy when installed | semantic discovery or raw protocol experimentation | grepai graph, `rg`, targeted reads; it is currently optional and absent |
| `rtk` | noisy tests, builds, vet, lint, git, and CLI output | repository discovery, code search, navigation, or source reading | use the purpose-built search/read tool; preserve native semantics |
| `jq` | compact JSON selection and transformation | unstructured text | a short script only when the query is genuinely complex |
| ast-grep | structural search and safe repetitive rewrites | ordinary text search or one-off edits | `rg` and manual targeted edits; do not require ast-grep |

## Code search decision

1. Filename or path pattern: use `rg --files` with a constrained glob.
2. Exact identifier, opcode, string, constant, or error: use `rg` once.
3. Unknown implementation location or behavior described semantically: use grepai directly.
4. Caller or callee relationship: use `grepai trace` with depth one or two.
5. Go definition, implementation, diagnostic, or field/state references: use `gopls`.
6. Syntax-shaped match or structural rewrite: use ast-grep.
7. Java definition or hierarchy: use JDT LS only through a verified local client; otherwise use
   grepai trace, `rg`, and targeted reads. grepai 0.35.0 has no `refs` command.
8. Read only returned files and relevant ranges with the native read tool. Do not dump full files until targeted reads are
   insufficient to understand control flow.

Constrain every search by repository/path, small result count, and compact output. If grepai is stale,
unavailable, or low quality, record the fallback and use `rg`, `gopls`, and targeted reads rather
than retrying repeatedly.

## Verified grepai commands

The workspace uses a local Ollama `nomic-embed-text` embedder and a local GOB index. No paid or remote
embedding service is required.

```bash
grepai status --no-ui
grepai search "<behavior>" --toon --compact --limit 5 --path <relevant-path>
grepai trace callers "<symbol>" --toon
grepai trace callees "<symbol>" --toon
grepai trace graph "<symbol>" --depth 2 --toon
```

In grepai 0.35.0, `--compact` requires `--json` or `--toon`; do not copy examples that omit the output
format. Use `--mode precise` for trace only when the fast extractor is ambiguous. Prefer the CLI for
normal search, trace, status, and watcher work because it avoids invoking an MCP schema. Existing
Claude/Codex MCP configuration is outside tracked project files and also exposes dedicated RPG
operations without a direct CLI equivalent, so it remains unchanged; use it only for that demonstrated
need.

Do not change the embedding model without documenting and performing a reindex. If the local model is
unavailable, report optional setup steps rather than installing a large model silently.

The workspace index configuration is `.grepai/config.yaml`. Its root is the outer workspace, so it
covers `acis_golang/` and `aCis_gameserver/` plus small root documentation; it excludes the datapack,
agent caches/configuration, worktrees, generated output, and logs. Use the repository wrappers from
the workspace root:

```bash
scripts/agent-tools/grepai-status.sh
scripts/agent-tools/grepai-watch.sh start
scripts/agent-tools/grepai-watch.sh status
scripts/agent-tools/grepai-watch.sh stop
```

The watcher is opt-in and starts in grepai's supported background mode. Its state and log location
come from `grepai watch --status --no-ui`; do not infer state from lock files. Starting it requires the
configured local Ollama service and embedding model. No workflow requires it to remain running.

## gopls

Prefer language-server operations over reading several Go files:

```bash
gopls definition <file.go:line:column>
gopls references <file.go:line:column>
gopls implementation <file.go:line:column>
gopls call_hierarchy <file.go:line:column>
gopls check <file.go>
```

The installed version also supports `gopls mcp`, but the CLI is sufficient for bounded lookups and
does not require another always-loaded MCP schema.

Claude enables the verified official `gopls-lsp` 1.0.0 plugin at project scope. Its marketplace
manifest maps `.go` files to the installed `gopls` command. This provides standard definitions,
references, implementations, call/type hierarchy, and diagnostics without repository-specific LSP
configuration.

## Java language server

JDT LS is not currently installed and the local `java` launcher has no runtime. Do not make JDT LS a
required step or add unverified Claude plugin settings. When a user approves installation, install a
compatible Java runtime and JDT LS, verify project import and definition/reference/implementation,
type-hierarchy, call-hierarchy, and diagnostic operations, then enable a locally supported Claude
plugin. Until then, use grepai and `rg`; Codex should not issue raw LSP protocol calls.

## Repository maps

Aider is not installed, so no repository maps or generator dependency are configured. The optional
format and refresh policy live in `.agent-cache/maps/README.md`. Use grepai and LSP navigation
instead; revisit maps only if a local generator can run without a model/API dependency and measured
orientation work justifies it.

## rtk, jq, and ast-grep

Use `rtk go -C acis_golang test`, `rtk go -C acis_golang build`, and `rtk go -C acis_golang vet` for
noisy Go commands, and `rtk err <command>` for other verbose checks. Use `jq` to select only the JSON
fields needed for a decision. Use
`ast-grep run` or `ast-grep scan` only when AST structure makes text search or repetitive edits
unsafe; no repository ast-grep configuration is required for ordinary work.
