# WM Tips

A World Cup 2026 prediction game for you and your friends. Predict the whole
tournament up front, tip every individual match, and compete on private
leaderboards. Ships as a **single Docker image** (one Go binary serving the
API and the embedded SvelteKit app).

> Naming note: the repo/module is `wm-pickems`; the app is branded **WM Tips**.

## Features

- **Tips** — predict the score of every one of the 104 matches. Editable
  until kickoff; knockout entry is progressive (90′ → extra time → penalty
  winner). After kickoff your tip locks and you can see friends' picks.
- **Forecast** — one pre-tournament call: full group standings (1st–4th),
  the 8 best-third qualifiers, and the whole knockout bracket. Locks at the
  first kickoff; correctness is shown per stage as results come in
  (exact / advanced-but-wrong-slot / missed).
- **Leagues** — private competitions you join via invite code or a
  shareable `/join/<code>` link (with public preview + auth resume).
  Combined leaderboard plus separate **Overall / Tips / Forecast** views,
  with the tiebreaker stats exposed and a built-in scoring legend. Your
  own row is highlighted. Every user is auto-joined to a shared **Global**
  league.
- **Live tournament view** — group tables and a knockout bracket that fill
  in from real results.
- **Bot opponents** — `role=bot` accounts that play your leagues through the
  public API under the exact same locks as humans. Two brains: a deterministic
  API-free **rating model** (`algo`) or **Claude** (`claude`). They run as a
  standalone side project ([`bots/`](bots/README.md)) and revise their
  predictions from results as the tournament unfolds.
- **Accounts** — email/password with **password reset** (forgot-password +
  in-app confirm route), **Google sign-in** (OAuth2, configured from env),
  and a **settings page** to edit display name and avatar.
- **PWA** — installable (topbar button + first-visit banner on mobile),
  offline app shell, maskable icons + screenshots.
- **Results** — auto-synced from API-Football (free tier) when a key is set;
  always overridable by an admin; fully playable on the seeded fixtures
  without any key.

## Scoring (config-driven, max 6 points per match)

| Per match | Pts |
|---|---|
| Correct result — group `1/X/2`; knockout: the team that advances | 3 |
| Exact score | +1 |
| Correct total goals | +1 |
| Correct goal difference | +1 |

Knockout games have no draw; the score points use the after-extra-time score
when a tie goes to extra time.

**Forecast:** each team in its correct final group position `1` (whole group
perfect `+2`); `+1` per predicted advancer (group top-2 or a best-third pick)
that actually advances; escalating knockout reach per predicted team —
R32 `1` / R16 `2` / QF `3` / SF `5` / Final `8` / Champion `13`.

**Tiebreakers:** points → most exact scores → most correct winners → smaller
goal-difference error → fewer tips submitted → earliest submission. Users
who never submitted a tip are sorted to the bottom regardless.

Every weight lives in the `scoring_configs` "Default" record (per-League
overrides supported) and can be changed without a redeploy — the in-app
legend always reflects the live config.

## Stack

- **Backend** — Go with [PocketBase](https://pocketbase.io) as a framework
  (auth, SQLite, REST, cron, hooks).
- **Frontend** — SvelteKit SPA (`adapter-static`, Svelte 5), bundled into the
  Go binary via `go:embed`.
- **Ship** — one multistage Docker image; SQLite data on a `pb_data` volume.

## Project layout

```
main.go                 wiring: migrations, seed, route registration, SPA serve
migrations/             Go-code PocketBase schema + data migrations
internal/
  seed/                 embedded openfootball WC2026 data + first-boot seed
  football/             API-Football client
  sync/                 result sync, manual override, bracket resolver
  tips/                 per-match tip rules (lock, KO, friends endpoint)
  forecast/             forecast validation + structure endpoint
  scoring/              pure scoring core, recompute, leaderboard (+ tests)
  leagues/              create / join / leaderboard endpoints
  bracket/              FIFA Annex C best-third → R32 allocation table
  oauth/                Google sign-in wiring from env (idempotent)
  clock/                overridable "now" (dev virtual clock)
  dev/                  WMP_DEV-only simulator + bot generator
  web/                  go:embed of the built SPA
frontend/               SvelteKit app
bots/                   standalone bot player (algo / Claude) — own Go module
```

## Develop

```sh
make install        # frontend deps
make dev-backend    # PocketBase on http://127.0.0.1:8090 (admin UI at /_/)
make dev-frontend   # SvelteKit dev server (proxies /api to the backend)
```

## Build & run as a single binary

```sh
make run            # builds the SPA, embeds it, runs the binary
```

App + API are served from one origin on `:8090`.

## Test

```sh
make test           # Go unit tests (scoring engine)
```

The frontend type-checks with `cd frontend && npm run check`.

## Docker / deploy

```sh
cp .env.example .env       # API_FOOTBALL_KEY, admin creds, port
docker compose up --build -d
```

Full operations guide (superuser, results override, recompute, backup, TLS):
see [DEPLOY.md](DEPLOY.md).

## Dev / test harness

Run with `WMP_DEV=1` to unlock the **/dev** page (also linked in the user
menu):

- **Advance** to any timestamp — matches before it are simulated finished
  (mid-match → live, not scored), later ones reset; the virtual clock drives
  every lock/visibility rule so you can test the whole lifecycle.
- **Generate bot players** — full random Forecast + a tip on every match,
  joined to your leagues, for instant leaderboard testing.
- **Reset** — clear all results and the clock.

The dev endpoints are **not registered** unless `WMP_DEV=1`.

## Data

- Fixtures/teams/groups seeded from
  [openfootball/worldcup.json](https://github.com/openfootball/worldcup.json)
  (embedded, public domain).
- Live results from [API-Football](https://www.api-football.com) free tier
  (one request per sync, well under the daily limit).

## Roadmap / known follow-ups

See [PLAN.md](PLAN.md) for the original design. No major open items at the
moment — Google OAuth, the official FIFA Annex C best-third → R32 table,
and viewing friends' Forecast detail after lock have all shipped.

## License

See [LICENSE](LICENSE).
