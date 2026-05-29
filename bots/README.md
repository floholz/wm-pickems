# wm-pickems bots

A standalone side project that plays [wm-pickems](../) as a bot. v1 ships **Claude** — it logs in as a bot user and submits a tournament **Forecast** and per-match **Tips** through the public REST API, playing by the exact same server-side locks as a human:

- can't tip after kickoff,
- can't tip a knockout match before both teams are resolved,
- can't submit a Forecast after the tournament starts,
- can't see anyone else's tips before kickoff.

No bypass anywhere — the bot competes on equal footing. It's a separate Go module with no dependency on the main app: just an HTTP client + the Anthropic SDK.

## How it works

1. **Auth** — logs in via `users/auth-with-password`.
2. **Forecast** (once, before the tournament locks): asks Claude to rank every group 1–4 and pick the 8 best thirds, then walks the knockout rounds R32→FINAL, resolving each match's two concrete teams and asking Claude who advances. The bracket-resolution logic mirrors the server's scoring engine (`bracket.go`) so the Forecast scores correctly. Uses FIFA's official Annex C third-place table served by `/api/forecast/structure`.
3. **Tips** (every run, idempotent): finds every match that's still open (kickoff in the future, matchup resolved) and not already tipped, then asks Claude for a scoreline — a decisive 90' result for knockouts so the server derives the advancer. Skips matches it has already tipped.

It reads the server clock from `/api/now`, so it also works against the `WMP_DEV=1` simulator — you can advance the virtual clock and watch Claude play a whole tournament before June 2026.

The large, unchanging tournament reference (teams, groups, knockout skeleton) is sent as a **cached system prompt**, so every prediction call after the first reuses it as a prompt-cache prefix.

## Setup

1. In the PocketBase admin, create the bot's user account, set `role=bot` and `botKind=claude`, and add it to your league(s) — or set `BOT_LEAGUE_CODE`.
2. Copy `.env.example` and fill in `BOT_EMAIL`, `BOT_PASSWORD`, `ANTHROPIC_API_KEY`, and `WMP_BASE_URL`.

## Run

```sh
go run .            # one pass: ensure the Forecast exists, tip all open matches
go run . --loop --interval 1h   # keep running on a schedule
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
docker build -t wm-pickems-bot:latest .

# Run one bot, continuously
docker run -d --name wmp_bot_claude --restart unless-stopped \
  --env-file claude.env wm-pickems-bot:latest
```

`claude.env` holds the same variables as `.env.example`. For a second bot later, run another container with its own env file off the same image.

A Compose starting point is in `docker-compose.example.yml` (copy to `docker-compose.yml`, add a per-bot env file, `docker compose up -d`). If the app runs in its own Compose project, put the bot(s) on a shared Docker network and point `WMP_BASE_URL` at the app's container name.

## Configuration

See `.env.example`. Defaults: `WMP_BASE_URL=http://127.0.0.1:8090`, `CLAUDE_MODEL=claude-opus-4-8`. The container additionally defaults its command to `--loop --interval=1h`.

## Tests

```sh
go test ./...
```

`bracket_test.go` checks the resolver port (bracket winners are always valid participants; W/L feeder labels resolve; the Annex C third-place lookup works).

## Future

ChatGPT and an algorithmic bot drop in as alternative "brains" (`brain.go`); the client and bracket logic are shared. Showing each bot's reasoning in the app UI is a planned follow-up (would add an optional `rationale` field on tips).
