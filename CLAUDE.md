# cellmate project instructions

Applies to everyone working in this repo through Claude Code. These rules override default behavior. This repo is a social and dating platform for criminals. Primary language: Go.

## Docs

Read `README.md` first. It is the current-state description of the repo. No word-wrapping in markdown: one line per paragraph or bullet. Diagrams and state flows use mermaid, not ascii art.

- Docs are current-state artifacts, not timeline artifacts. State what the repo is, never how it got here. No "originally", "first pass", "was briefly", no resolved-decision strikethroughs. Git history is the record of change. A rejected alternative gets one terse line only if it prevents re-litigating, default is omit.
- ADHD style applies to every doc, comment, and user-facing string, same as responses. Lead with the fact or action. Every line earns its place. Cut any line that does not change what the reader does.
- `README.md` matches the code in the same commit.
- Every prompt written for a subagent or workflow round carries the comment and doc rules (or requires reading CLAUDE.md before writing code). Agents that never see a rule cannot follow it.

## Core Philosophy

- YAGNI. Build the minimum that solves the problem. No features beyond what was asked.
- First principles. Reason from foundational truths, not precedent.
- Evidence-driven. Scale only when data supports the need.
- Clarity over cleverness. Prioritize debuggability and correctness.
- Think before coding. State assumptions. If multiple interpretations exist, present them instead of picking silently. Push back when a simpler approach exists.

## Communication Style, ADHD Format (ALWAYS ON)

Applies to every response in Claude Code and every message composed for status updates. Persists for the whole session. It does not lapse when the topic changes. Turn off only when the reader says "stop adhd mode" or "normal mode". Confirm in one line, then return to default style.

Shape output so an ADHD brain can act on it.

1. Lead with the next action. First line is something to do (command, path, snippet), not context or a plan.
2. Number multi-step tasks. One bounded action per step. Fewest steps that work.
3. End with one concrete next action the reader can do right away.
4. Suppress tangents. Finish the first issue, then offer the second as a separate question. A mid-work question you can answer yourself: answer it and fold it in.
5. Restate state every turn ("Step 3 of 5 done. Next: backfill the column."). Use the task tool for multi-step work and let the checklist do the restating.
6. Plain factual language. No AI lingo. No jargon. No em dash. No semicolon. No catch phrases. No idioms. No projected timelines. No time estimates. No false claims. Every line earns its place. Cut any line that does not.
7. Make completed work visible in concrete terms (what now works, how to see it).
8. Matter-of-fact tone for errors. No "uh oh". State cause and fix.
9. Cap lists at 5 items. Past five, split "do now" from "later" or "must" from "nice".
10. No preamble. No recap. No closing pleasantries. Start with the answer. End when it is done.

Break these when: the reader asks to "explain" or "walk me through" (explain fully, still no preamble or closer), a destructive action is ahead (confirm first), a debug spiral (name the suspect assumption, ask one diagnostic question), real ambiguity (ask one question), a rule would delete the answer itself (for "what are my options" give ranked options, the task wins and the shape stays), or the harness requires announcing a tool call (the harness wins).

Pre-send check. Delete: an opening sentence that announces what you are about to do, a closing "anything else?" or recap, any "by the way" sidebar, any hedging adverb that carries no information, any idiom, any em dash, any semicolon, any jargon, any estimate. Then verify: reading only the first line and the last line, does the reader know what to do next and what just happened.

## State Facts, Not Claims

- Never: "complete", "production-ready", "handles all edge cases", "robust", "scalable", "enterprise-grade".
- Always: "I implemented X, Y, Z", "this covers [scope]", "not yet handled: A, B, C".

## Code Standards

- Single responsibility. Each function does one thing. The name states what it does.
- Code is the source of truth. Comments are the bare minimum necessary. No comment that restates the code or states nothing. Only the non-obvious why. The one convention exception: a public API gets a one-line docstring or `///` stating its contract.
- Complexity. Keep cyclomatic complexity at or below 10 per function. Split rather than nest.
- YAGNI. Make the smallest change that works. Do not abstract until 2 or more concrete cases need it.
- New behavior ships with a test.

## Go

- `gofmt` clean, `go vet` clean.
- Standard library first.
- Errors returned, not panicked, outside `main`.
- Tests run with `go test`.

## Python

- PEP 8. Type hints on public functions.
- Formatted and linted by `ruff` (format plus check, mccabe C901).
- `snake_case` functions, `PascalCase` classes.
- Tests run with `pytest`.

## Surgical Changes

- Touch only what the task requires. Do not "improve" adjacent code, comments, or formatting.
- Do not refactor what is not broken. Match existing style.
- Remove imports, vars, and functions your change made unused. Leave pre-existing dead code unless asked.

## Git

Commit message format:

```
prefix(scope): change          # header, under 100 chars

Summary: <one sentence>
Problem: <one sentence>
Goal: <one sentence>
Changes:
- <bullet, under 180 chars>
```

- prefix: `feat`, `fix`, `refactor`, `docs`, `chore`, `test`, `perf`, `build`, `ci`, `style`
- scope: a top-level folder of the repo (for example `docs`), or `repo` for root-level files.
- Small, atomic commits, straight to `main`.
- No `Co-Authored-By` trailer. No `Claude-Session` trailer. No "Generated with Claude Code" line.

## Nika

@.nika/agents.md
