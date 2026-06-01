# wm-pickems bots

A standalone side project that plays [wm-pickems](../) as a bot. It logs in as a bot user and submits a tournament **Forecast** and per-match **Tips** through the public REST API, playing by the exact same server-side locks as a human:

- can't tip after kickoff,
- can't tip a knockout match before both teams are resolved,
- can't submit a Forecast after the tournament starts,
- can't see anyone else's tips before kickoff.

No bypass anywhere — the bot competes on equal footing. It's a separate Go module with no dependency on the main app: just an HTTP client + the Anthropic SDK.

## Strategies (`BOT_KIND`)

The prediction "brain" is selected by `BOT_KIND`; everything else (auth, the bracket resolver, the submit flow) is shared:

- **`algo`** (default) — a deterministic, API-free **rating model**. Each team gets a strength rating from a small embedded table (`algo.go`, keyed by FIFA code, neutral default for unknowns). Group order = by rating; best-8 thirds = the highest-rated third-placed teams; the bracket = higher rating advances (ties → home); scorelines = expected goals from the rating gap (`round(1.25 + gap/160)`, uncapped so a genuine mismatch can read 4–5+; group games may draw, knockouts coerced decisive). It also **learns**: results Elo-adjust the ratings (see _Feedback loop_). No API key required. Tweak the ratings table to change its starting opinion.
- **`claude`** — asks Claude (Anthropic API) for predictions. Needs `ANTHROPIC_API_KEY`. See _How it works_ below.

## How it works

1. **Auth** — logs in via `users/auth-with-password`.
2. **Forecast** (once, before the tournament locks): asks Claude to rank every group 1–4 and pick the 8 best thirds, then walks the knockout rounds R32→FINAL, resolving each match's two concrete teams and asking Claude who advances. The bracket-resolution logic mirrors the server's scoring engine (`bracket.go`) so the Forecast scores correctly. Uses FIFA's official Annex C third-place table served by `/api/forecast/structure`.
3. **Tips** (every run, idempotent): finds every match that's still open (kickoff in the future, matchup resolved) and not already tipped, then asks Claude for a scoreline — a decisive 90' result for knockouts so the server derives the advancer. Skips matches it has already tipped.

It reads the server clock from `/api/now`, so it also works against the `WMP_DEV=1` simulator — you can advance the virtual clock and watch Claude play a whole tournament before June 2026.

## Feedback loop

Every run the bot pulls in finished results and revises, like a human would:

- **algo** Elo-adjusts its ratings from every result (goal-difference-weighted), so an over-performing team climbs and its upcoming tips shift accordingly; a flop drops.
- **claude** gets the results so far as prompt context and re-reasons.

It then reconciles **every still-open match**: creating missing tips and **updating** ones whose prediction has changed (editing a tip before kickoff is allowed — same as a human). To avoid churn and needless API calls, already-tipped matches are only re-evaluated when a new result has come in since the bot last tipped. The **Forecast is one-shot** and never revised (it locks at the first kickoff, before any results exist).

The large, unchanging tournament reference (teams, groups, knockout skeleton) is sent as a **cached system prompt**, so every prediction call after the first reuses it as a prompt-cache prefix.

`CLAUDE_MODEL` accepts any chat model (default `claude-opus-4-8`). Opus 4.6+/Sonnet 4.6 run with adaptive thinking; `claude-haiku-4-5` has no adaptive thinking, so it runs with thinking omitted — handy as a cheaper/faster model for dev.

## Setup

1. In the PocketBase admin, create the bot's user account, set `role=bot` and `botKind` (`claude` or `algo`), and add it to your league(s) — or set `BOT_LEAGUE_CODE`.
2. Copy `.env.example` and fill in `BOT_EMAIL`, `BOT_PASSWORD`, `WMP_BASE_URL`, and `BOT_KIND`. For `claude` also set `ANTHROPIC_API_KEY`; `algo` needs no key.

## Run

```sh
go run .            # one pass: ensure the Forecast exists, tip all open matches
go run . --loop --interval 1h   # keep running on a schedule
go run . --once     # single pass, even if --loop is set (overrides the container default)
```

### Triggering a run manually

A no-flag invocation is already a single run, so the simplest one-off is just `go run .` (or `./wm-pickems-bot`).

For a bot that's **already running in `--loop`** (the container default), send it a signal to act now without waiting for the next tick — it reuses the live process and its env:

| Signal | Effect |
|---|---|
| `SIGHUP`  | Run now (normal): tip new open matches + revise where a result changed since the last tip. |
| `SIGUSR1` | Re-evaluate **all** open tips and override existing picks — use after retuning a brain (e.g. the algo ratings table or the Claude prompt). |
| `SIGUSR2` | Regenerate the **forecast**, overriding the existing one. Pre-lock only — the server rejects forecast edits once the tournament starts. |

```sh
kill -USR1 <pid>                               # re-tip, bare process
docker kill --signal=SIGUSR1 wmp_bot_claude    # re-tip, docker run
docker compose kill -s SIGUSR2 bot-claude      # re-forecast, compose service
```

`SIGUSR1`/`SIGUSR2` only re-submit a pick where the new prediction actually differs from the saved one, so re-running after no change is a cheap no-op (aside from the LLM calls).

A fresh one-off against the deployment without touching the running loop (`--once` overrides the image's `--loop` default):

```sh
docker compose run --rm bot-claude --once
docker run --rm --env-file claude.env wm-pickems/bot:latest --once
```

Or build and run via cron / a systemd timer:

```sh
go build -o wmbot .
./wmbot
```

## Docker

One image, **one container per bot** — the same image runs every bot; each container just gets its own env (different `BOT_EMAIL` / `CLAUDE_MODEL` / …). The container defaults to `--loop` so it runs continuously.

```sh
# Build (context is this bots/ directory — separate module from the app)
docker build -t wm-pickems/bot:latest .

# Run one bot, continuously
docker run -d --name wmp_bot_claude --restart unless-stopped \
  --env-file claude.env wm-pickems/bot:latest
```

`claude.env` holds the same variables as `.env.example`. For a second bot later, run another container with its own env file off the same image.

A Compose starting point is in `docker-compose.example.yml` (copy to `docker-compose.yml`, add a per-bot env file, `docker compose up -d`). If the app runs in its own Compose project, put the bot(s) on a shared Docker network and point `WMP_BASE_URL` at the app's container name.

### Releasing (CI)

The bot has its **own** release tags, independent of the app. Pushing a `bot-v*` tag triggers `.github/workflows/docker-publish-bot.yml`, which builds and pushes to GHCR:

```sh
git tag bot-v0.1.0 && git push origin bot-v0.1.0
# -> ghcr.io/floholz/wm-pickems/bot : 0.1.0, 0, latest
```

The package is private by default; flip visibility in GHCR if you want public pulls.

## Configuration

See `.env.example`. Defaults: `WMP_BASE_URL=http://127.0.0.1:8090`, `BOT_KIND=algo` (no API key needed), `CLAUDE_MODEL=claude-opus-4-8` (only used when `BOT_KIND=claude`). The container additionally defaults its command to `--loop --interval=1h`.

### Logging

Structured logging via `log/slog` to **stdout**. Set `LOG_FORMAT=json` in containers for shipper-friendly structured logs (Grafana Alloy/Promtail → Loki), or leave the default `text` for readable local output. Notable events: `ai_call` (per Anthropic call — model, input/output/cache token counts, duration; `cache_read=0` across a run means the cached prompt prefix isn't hitting), `tip` (created/revised with old→new scoreline and trigger), and the per-run `run_id` that ties a run's lines together. The `algo` strategy emits no `ai_call` events (it makes no API calls).

Set `BOT_RATIONALE=1` (claude only) to have the model attach a one-line reason to each prediction; it's logged on the `tip` events and as `group_pick`/`winner_pick` events. Costs extra output tokens, so it's off by default.

## Tests

```sh
go test ./...
```

`bracket_test.go` checks the resolver port (bracket winners are always valid participants; W/L feeder labels resolve; the Annex C third-place lookup works). `algo_test.go` checks the rating model (group order, winner selection, scorelines).

## Future

ChatGPT drops in as a third `Predictor` (`predictor.go`) alongside `claude` and `algo`; the client and bracket logic are shared. Showing each bot's reasoning in the app UI is a planned follow-up (would add an optional `rationale` field on tips).
