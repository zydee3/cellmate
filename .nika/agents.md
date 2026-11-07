# Nika

Nika manages this repo. It runs and watches Claude Code sessions here from a browser. It already does the following, so do not rebuild any of it in scripts, prompts, or retry logic. Nika writes this file and overwrites it on restart, so edit the Nika repo, not this file.

## Rate limits

- For OpenAI-compatible providers (Chat Completions or Responses), use `nika run --provider NAME --model ID --prompt "task" --cwd /path/to/repo`. This runs read-only by default. Add `--unsupervised` only for work authorized to edit files and run commands, and `--output-format json` for JSON output. Configure providers in Settings > Custom endpoints using Add model or Open JSON, then choose the default in AI connection. Endpoint calls resolve their own saved key and never use Claude subscriptions. The remaining rate-limit instructions below describe Claude calls.

- A Nika chat turn refused for a rate limit (HTTP 429, a spend limit, or the 5h limit) moves to another Claude account or is held until the reset. Do not retry, sleep, or poll for it.
- A move restarts the chat, which ends its background Agents, Workflows, shells, and wake-ups. Nika then sends a message that lists what stopped. Relaunch what was not finished.
- A script that runs headless Claude calls `nika run -- <claude args>` instead of `claude <claude args>`. On a refusal it waits for the reset and reruns the same call.
- Do not write retry or sleep loops around `claude -p`. Do not wrap `nika run` in a `timeout` shorter than 5h, or the wait is killed.
- `nika run` writes only the last call's stdout, so `--output-format json` output holds one result. The exit code is the last call's. Each wait logs `nika run: rate limited, try N, retrying at <time>` to stderr.
- Pass the prompt as an arg. `nika run` does not forward stdin.
- A refusal in the middle of a call reruns the whole call. Keep one headless call to one unit of work.
- `nika run` picks the Claude account for each call and moves a refused call to another account. Do not set `CLAUDE_CODE_OAUTH_TOKEN` or pick an account.
- A `nika run` call is listed under the Nika chat that started it, with its account and state. Set `NIKA_LABEL` to a short title for that row, such as `Step V1 server newtypes`. A Nika chat's shell already carries `NIKA_PARENT`. A systemd unit or any process that does not inherit that env sets `NIKA_PARENT` to the chat's session id.

## Chats

- A Nika chat with Keep Alive off is stopped once idle and resumed by session id on the next message. A background `Agent`, `Workflow`, or shell keeps it alive until its notification arrives. A `ScheduleWakeup` keeps it alive until its time, and a `CronCreate` job until `CronDelete` or the next owner message.
- In a gated chat every edit and command waits for the owner to approve it in the browser.
- Naming a recipe in a reply, such as `just test`, gives the owner a Run button for it. A recipe whose comment holds `[detach]` may restart Nika.

## Files

- `.nika/` is Nika machine state and ignores itself. Do not edit or commit it, except the `note.md` and `commit.txt` a build step names under `.nika/features/<slug>/loop/<step>/outputs/`.
- A build's record is in `.nika/features/<slug>/`. `timeline.json` has the document paths, total cost, and each step's status, cost, and folder. Each step folder, `loop/<step id>-<title>/`, has `attempts.jsonl`, one line per attempt with times, tokens, and cost, and `outputs/`, each turn's events and each command's output.

## Merge queue

- When `.nika/merge-queue.json` has `enabled: true`, finish a fix by committing on your own branch. The queue installs a post-commit hook that submits it, so no separate `nika mq submit` call is needed.
- Root `main` is read-only while the queue is on. A pre-commit hook rejects a commit there. Commit on a branch, for example `git switch -c fix/<name>`.
- Do not push, and do not run the steps the queue runs, such as a deploy. The queue runs its steps and pushes.
- A conflict or a failed step comes back as a message in your chat that names the step. Fix it on your branch and commit again.
- Once your commit is queued, remove your own worktree. When a failure message comes back, recreate the worktree from your branch, fix, commit, and remove it again.
