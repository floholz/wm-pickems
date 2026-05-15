# WM-Pickems — Implementation Plan

A WC 2026 prediction app for you + friends. Predict every match, predict the whole
tournament up front, compare against friends on a leaderboard.

## Glossary / naming

- **Forecast** — the one-time, pre-tournament prediction: full group standings (1–4 in
  each of the 12 groups), manual pick of the 8 best third-placed qualifiers and their
  R32 slots, and the full knockout bracket. No scores. Locks at the tournament's first
  kickoff.
- **Tip** — a per-match score prediction. Editable until that match kicks off.
- **League** — a private competition group friends join via invite code. (Distinct from
  the tournament's groups A–L.)

## Tech stack

- **Backend:** Go with PocketBase used as a framework (auth + SQLite + REST + hooks +
  scheduler). SQLite data on a mounted `pb_data` volume.
- **Frontend:** SvelteKit + `adapter-static` (SPA, `fallback: index.html`), mobile-first
  and responsive, talking to PocketBase via the JS SDK on the same origin.
- **Packaging:** one multistage Docker image — Node builds the SvelteKit bundle → Go
  embeds it via `embed.FS` → minimal final image serving API + app from one binary,
  with SPA fallback routing.
- **Data:** seed teams/groups/fixtures from `openfootball/worldcup.json` (no key);
  live results from API-Football free tier (100 req/day, `league=1&season=2026`) via a
  scheduled, cached, ramped poller; manual admin override always available.

## Data model (PocketBase collections)

- `users` (PB auth) — name, avatar. Email/password + optional Google OAuth (config).
- `teams` — code, name, flag, confederation.
- `tournament_groups` — letter A–L, team list.
- `matches` — extId, stage (`group|R32|R16|QF|SF|3RD|FINAL`), group letter, kickoff (UTC),
  status, FT score, ET score, penalty winner, resolved advancer, bracket slot, and
  placeholder labels for unresolved KO slots ("Winner Grp A", "3rd A/D/E/F").
- `leagues` — name, invite code, owner, scoring-config ref.
- `league_members` — league, user, role, joinedAt.
- `tips` — user, match, ftHome/ftAway, etHome/etAway, penWinner, derived advancer,
  updatedAt. One per user/match. Server enforces edit only while `now < kickoff`.
- `forecasts` — user, group orderings, third-place qualifiers + R32 slots, bracket
  picks. One per user. Server enforces lock at tournament start.
- `scoring_configs` — JSON weights; global default + optional per-league override.
- `match_scores` / `forecast_scores` — per-user computed point components, recomputed
  on result finalize and on later corrections.

## Scoring (config-driven; defaults)

- **Tip, group match** vs actual 90′: tendency 1/X/2 = 3; exact +1; correct total
  goals +1; correct goal diff +1 (max 6).
- **Tip, KO match** (phased): predict FT; if FT a draw predict ET (cumulative after
  120); if ET a draw pick penalty winner. Score = the 4 rules on FT vs actual 90′;
  **ET bonus** (default on, switchable off to reproduce your legacy system) = exact /
  total-goals / diff on ET vs actual-after-120 when the game truly went to ET and you
  predicted an FT draw; **+2** for the correct advancer.
- **Forecast** (awarded progressively as rounds resolve): correct group position = 1
  each (+2 perfect-group bonus); correct best-third = 2 each; KO progression escalates
  per surviving predicted team: R32 1 / R16 2 / QF 3 / SF 5 / Final 8 / Champion 13.
- **Tiebreakers:** points → #exact scores → #correct winners → smaller aggregate
  goal-diff deviation → earliest last edit.

## Visibility & deadline rules

- A user's Tip becomes visible to other league members **only after that match kicks
  off** (enforced server-side, not just hidden in UI).
- KO Tips open as soon as both teams of a matchup are known, until that kickoff.
- Forecast visible to league members only after it locks.
- Leaderboard: **Overall (combined)** plus separate **Tips** and **Forecast** tabs.

## Screens (SvelteKit, mobile-first)

Auth · Dashboard (next tips due, your ranks, deadlines) · Matches + Tip entry
(progressive KO UI) · Group tables + best-third tracker · Bracket tree · Forecast
builder (drag group order, third-place slotting, bracket picker) · Leagues
(create/join, members, leaderboards, others' tips post-kickoff) · Profile · Admin
(PB admin + small manual result/sync panel).

## Build phases (agent-time)

| # | Phase | Est. |
|---|-------|------|
| 0 | Scaffold: repo, Go+PocketBase, SvelteKit+static embed+SPA serving, Dockerfile, compose, config | ~0.5d |
| 1 | Data model, migrations, openfootball seed, API-Football client + scheduled sync + manual override | ~0.5d |
| 2 | Auth + Leagues (create/join/invite, membership, leaderboard scaffold) | ~0.5d |
| 3 | Tips: CRUD with kickoff lock, progressive KO UI, KO availability gating, others-tips visibility gate | ~1d |
| 4 | Forecast: group ordering + third-place + bracket builder, lock at start, advancer derivation | ~1d |
| 5 | Scoring engine: config-driven match + progressive forecast scoring, recompute on finalize/correction, leaderboards + tiebreakers | ~1d |
| 6 | UI polish: group tables, bracket tree, dashboards, responsive pass, locked/empty states | ~1d |
| 7 | Hardening: server-side access rules, edge cases (postponed/abandoned, reseeding), scoring tests, final image, deploy doc | ~0.5d |

**Total ≈ 6–7 agent-days.**

## Risks / notes

- API-Football 100 req/day → mitigated by openfootball seed + caching + ramped polling
  + manual override.
- WC2026's best-third → R32 mapping follows FIFA's official combination table (depends
  on which group letters the 8 thirds come from); implemented as a lookup table and
  reused to resolve Forecast bracket slots.
- The KO/OT-bonus default is a recommendation; flip via `scoring_configs` with no code
  change.
