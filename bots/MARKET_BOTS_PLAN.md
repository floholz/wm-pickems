# Plan: prediction-market bots (Kalshi + Polymarket)

Two new bot kinds — `BOT_KIND=kalshi` and `BOT_KIND=polymarket` — that derive their
predictions from the betting/prediction markets each site publishes for WC2026,
instead of from an LLM (`Brain`) or a rating model (`AlgoBrain`).

This document is the implementation plan only. No code is written yet.

---

## 1. Guiding principle: parity

Both bots must consume **the same kind of data** so they are comparable. I checked
both live APIs (June 2026). The constraint that fixes the design:

> **A Kalshi game exposes exactly three markets — home / away / tie. Pure 1/X/2,
> no exact-score market.** (`KXWCGAME-26JUN27JORARG-{ARG,JOR,TIE}`.)

Polymarket *also* has exact-score for some games, but Kalshi never does. So the
**common denominator both sites always publish is the 1/X/2 moneyline**, and that
is what both bots use. Polymarket's exact-score markets are deliberately **not**
used in the default path (they would break parity). They can become an opt-in
enrichment later (`POLYMARKET_USE_EXACT_SCORE=1`), but that is out of scope here.

Everything else (groups, bracket) maps to per-team futures both sites publish.
**No Monte-Carlo simulation** — every prediction is a direct read of published
probabilities, with one deterministic math step (1/X/2 → scoreline) for tips.

---

## 2. Architecture — mirror the `Brain` + `completer` split

The codebase already has the exact seam: `Brain` (brain.go) owns provider-agnostic
logic and consumes any `completer` transport (Anthropic vs OpenRouter). We add the
same shape for markets:

```
Brain        → completer    → { anthropicCompleter, openrouterCompleter }   (existing)
MarketBrain  → marketSource → { kalshiSource,        polymarketSource     }   (new)
```

- **`MarketBrain`** implements `Predictor` (predictor.go). It holds the
  site-agnostic logic: turn 1/X/2 → scoreline distribution, rank groups by
  group-winner odds, pick KO advancers by title-odds ratio. Identical for both
  sites.
- **`marketSource`** is the transport: one per site, differing only in HTTP
  endpoints, JSON shape, price units, and team-name/fixture mapping.
- **Fallback:** `MarketBrain` wraps a fallback `Predictor` (an `AlgoBrain`) for any
  match/group the market doesn't price yet (very common — markets are listed early
  but priced only near kickoff; e.g. a June-27 game reads `yes_bid: null` today).

Downstream is untouched: `MarketBrain` emits `TipOutcome` distributions that flow
through the existing `selectTip` / `candidateGrid` EV machinery (ev.go).

---

## 3. New files

| File | Contents |
|---|---|
| `marketbrain.go` | `MarketBrain` (implements `Predictor`); wraps a `marketSource` + fallback `Predictor`. |
| `market.go` | `marketSource` interface + shared types (`gameOdds`, `teamOdds`) + vig normalization + the Poisson `oneXTwoToScores`. |
| `source_kalshi.go` | `kalshiSource` — Kalshi trade-api client + ticker parsing + code map. |
| `source_polymarket.go` | `polymarketSource` — Gamma API client + slug map + name normalization. |
| `marketbrain_test.go` | Poisson round-trip, group ranking, KO ratio, fallback-on-missing. |
| `source_*_test.go` | JSON-fixture parsing + price/vig extraction per site (golden files, no network). |

`main.go` and `.env.example` get edits (sections 9–10).

---

## 4. The `marketSource` interface

```go
// market.go

// gameOdds is one match's vig-normalized 1/X/2 (pHome+pDraw+pAway == 1).
type gameOdds struct{ PHome, PDraw, PAway float64 }

// marketSource is the per-site transport. All probabilities are returned already
// vig-normalized. ok=false means "no usable price" → caller falls back to algo.
type marketSource interface {
    // GameOdds returns the 1/X/2 for a fixture, matched by FIFA codes + kickoff date.
    GameOdds(ctx context.Context, homeCode, awayCode, kickoffISO string) (o gameOdds, ok bool, err error)
    // GroupWinnerOdds returns P(win group) per teamID for one group letter.
    GroupWinnerOdds(ctx context.Context, group string) (map[string]float64, error)
    // AdvanceOdds returns P(reach knockouts) per teamID. Optional — returns
    // (nil,false,nil) on a site that doesn't publish it (Kalshi may not).
    AdvanceOdds(ctx context.Context) (probs map[string]float64, ok bool, err error)
    // TitleOdds returns P(win tournament) per teamID.
    TitleOdds(ctx context.Context) (map[string]float64, error)
}
```

The source maps the site's team identifiers to our `teamID`. `MarketBrain` is
constructed with the resolved `[]Team` so each source can build a
`code/name → teamID` lookup once.

Shared helpers in `market.go`:

- `normalizeVig(yesHome, yesDraw, yesAway float64) gameOdds` — divide each by the
  sum so they total 1 (both sites bake in a margin; Polymarket group/winner
  events also carry an `"Other"` bucket that is excluded first).
- `oneXTwoToScores(o gameOdds) []ScoreProb` — the Poisson inversion (section 6).

---

## 5. Per-site source details

### 5a. `polymarketSource` (Gamma API — public, no auth)

Base: `https://gamma-api.polymarket.com`. Prices arrive as `outcomePrices:
["yes","no"]` strings already in 0–1; take the `"yes"` value.

| Need | Endpoint | Parse |
|---|---|---|
| Title | `/events?slug=world-cup-winner` | `markets[]`: `groupItemTitle` = team name, `outcomePrices[0]` = P(win). neg-risk: drop `"Other"`, renormalize. |
| Group winner | `/events?slug=world-cup-group-{a..l}-winner` | same shape, per group. |
| To-advance | `/events?slug=world-cup-team-to-advance-to-knockout-stages` | 48 per-team binaries; `outcomePrices[0]` = P(advance). |
| Game 1/X/2 | game event slug (not posted yet for soccer; appears near kickoff) | 3-way market: home / draw / away `outcomePrices`. |

- Team id: full names → `teamID` via a normalization table
  (`"South Korea"→KOR`, `"Czechia"→CZE`, `"Bosnia and Herzegovina"→BIH`, …),
  reconciled against `Team.Name`/`Team.FifaCode`.
- **Game discovery is the one unknown** — soccer per-game markets aren't posted
  this far out (only cricket games showed up in search). When they post, find the
  slug via `/public-search?q=...` or the sports tag and match on team names + date.
  Until then `GameOdds` returns `ok=false` and tips fall back to algo. Finalize the
  game-slug pattern against a live fixture before shipping the tips path.

### 5b. `kalshiSource` (trade-api — public for reads, no auth)

Base: `https://api.elections.kalshi.com/trade-api/v2` (serves all categories incl.
sports). Prices are **cents 0–100** → divide by 100. Per market take
`(yes_bid+yes_ask)/2` when both present, else `last_price`, else treat as no price
(`ok=false`).

| Need | Endpoint | Parse |
|---|---|---|
| Title | `/markets?event_ticker=KXMENWORLDCUP-26` | per-team markets; ticker suffix = code. |
| Group winner | `/events?series_ticker=KXWCGROUPWINNER` → `/markets?event_ticker=…` | confirm per-group event tickers. |
| Game 1/X/2 | `/events?series_ticker=KXWCGAME` lists `KXWCGAME-{YY}{MMM}{DD}{HOME}{AWAY}`; then `/markets?event_ticker=…` | exactly 3 markets, suffix `-HOME / -AWAY / -TIE`. |
| To-advance | (likely absent) | `AdvanceOdds → (nil,false,nil)`. |

- Team id: ticker codes are near-FIFA (`ARG`,`COL`,`POR`,`COD`=Congo DR,
  `DZA`=Algeria→our `ALG`,`UZB`,…). Small alias map `kalshiCode → teamID`.
- Fixture match: parse date + the two codes out of the event ticker
  (`KXWCGAME-26JUN27JORARG` → 2026-06-27, JOR vs ARG) and match to our `Match` by
  codes + kickoff day.

---

## 6. The math: `oneXTwoToScores` (1/X/2 → scoreline distribution)

Turns a vig-free 1/X/2 into a full scoreline distribution by **inverting an
independent-Poisson goal model** — the rigorous form of "which scorelines produce
these odds." Deterministic, no deps, no simulation.

1. Model goals as `home ~ Poisson(λH)`, `away ~ Poisson(λA)`. The 1/X/2 are pure
   functions of `(λH, λA)`; three outcomes summing to 1 = 2 d.o.f. = the two λs.
2. **Fit** `(λH, λA)` so the model reproduces the target `(PHome, PDraw)`
   (`PAway` follows). Coarse grid `λ ∈ [0.1, 3.5]` step `0.05`, minimize squared
   error vs `(PHome, PDraw)`; then one local refine pass at step `0.01`. ~5k cheap
   evaluations — trivial.
   - Model probs: `pHome = Σ_{h>a} pois(h;λH)·pois(a;λA)`, `pDraw = Σ_{h=a} …`,
     summed over `h,a ∈ 0..MAXG` (MAXG=8).
3. **Emit** the distribution: `ScoreProb{h, a, pois(h;λH)·pois(a;λA)}` for
   `h,a ∈ 0..6`. Return it raw — `selectTip`/`normalizeDist` clamp, drop KO draws,
   renormalize, and pick the EV-max scoreline.

Round-trip test: feed a known `(λH,λA)`, derive 1/X/2, invert, assert recovered
λs ≈ originals and that EV-max scoreline is sensible.

---

## 7. `MarketBrain` — implementing `Predictor`

```go
type MarketBrain struct {
    src      marketSource
    fallback Predictor   // AlgoBrain, for anything the market doesn't price
    teams    []Team      // for code/name → id and group membership
}
func NewMarketBrain(src marketSource, fallback Predictor, teams []Team) *MarketBrain
```

**`PredictGroups`** — rank, no simulation:
- `GroupWinnerOdds(letter)` per group. 1st place = max P(win group).
- Places 2nd–4th ordered by `AdvanceOdds` if available, else by P(win group).
- Best-thirds: rank all 12 third-place teams by advance odds (or win-group odds
  fallback), take the top 8 group letters.
- Any group the source can't price → defer that group to `fallback.PredictGroups`.

**`PredictWinners`** (KO bracket forecast) — direct title-odds ratio:
- `TitleOdds()` once. For tie A vs B:
  `P(A advances) = t[A] / (t[A] + t[B])` → higher advances.
- Missing either team's odds → `fallback.PredictWinners` for that match.

**`PredictTips`** (per-match scorelines):
- For each target, resolve home/away FIFA codes + kickoff date →
  `GameOdds(...)`. If `ok`: `oneXTwoToScores` → `TipOutcome`. If not:
  `fallback.PredictTips` for that match.
- KO draws handled downstream by `selectTip` (decisive scores only).
- No rationale (deterministic) — like `AlgoBrain`, return empty rationale maps.

All network reads are cached per run (one `GameOdds` map fetch, one each for
title/group/advance) so a single pass makes a handful of calls, not one per match.

---

## 8. Fallback semantics

`MarketBrain` always holds an `AlgoBrain` built from the same `teams` + finished
matches. Fallback fires per-item (per match / per group), never whole-run, so a
bot that prices 60% of fixtures still uses the market for those 60% and algo for
the rest. Log at debug which source served each item, with a per-run summary count
(`market_served`, `fallback_served`) so coverage is observable.

---

## 9. `main.go` wiring

- **config** (main.go:42-50): no new struct fields strictly needed; reuse none of
  the LLM fields. Add the two kinds to validation.
- **loadConfig** (main.go:64-84): add
  ```go
  case "kalshi", "polymarket":
      // no API key required — reads are public
  ```
  and update the `default` error string to list them.
- **brain selection** (main.go:246-273): add a branch before the LLM `else`:
  ```go
  } else if cfg.kind == "kalshi" || cfg.kind == "polymarket" {
      algo := NewAlgoBrain(teams, finished)        // shared fallback
      var src marketSource
      if cfg.kind == "kalshi" {
          src = newKalshiSource(teams, log)
      } else {
          src = newPolymarketSource(teams, log)
      }
      predictor = NewMarketBrain(src, algo, teams)
      log.Info("strategy selected", "strategy", cfg.kind, "fallback", "algo")
  } else {
      // ... existing LLM path ...
  }
  ```

The app side already supports arbitrary `botKind` markers (per
`project-bot-accounts`); register two bot users with `botKind=kalshi` /
`botKind=polymarket`.

---

## 10. Config / env

No secrets needed (reads are public). New optional knobs:

```
BOT_KIND=kalshi | polymarket          # selects the source
KALSHI_API_BASE   (default https://api.elections.kalshi.com/trade-api/v2)
POLYMARKET_API_BASE (default https://gamma-api.polymarket.com)
MARKET_MAX_GOALS  (default 8)         # Poisson fit/emit bound
```

Document both kinds in `main.go`'s header comment (main.go:14-17) and add a
`.kalshi.env` / `.polymarket.env` like the existing per-provider example envs.

---

## 11. Testing

- **Unit, no network:**
  - `oneXTwoToScores` round-trip (section 6).
  - Vig normalization (sum→1; `"Other"` excluded).
  - Group ranking + best-thirds from a fixture odds map.
  - KO title-ratio advancer choice.
  - Fallback fires when `GameOdds`/odds missing.
- **Source parsing:** golden JSON fixtures captured from the live APIs (already
  have real responses from the probing) → assert code/name mapping, cents-vs-0–1,
  bid/ask-mid vs last_price, null-price → `ok=false`.
- **Manual smoke:** run `BOT_KIND=polymarket` against the local server once
  forecast markets are reachable (they are live now); confirm forecast submits and
  tips fall back to algo until soccer game markets post.

Follows existing `*_test.go` style (algo_test, ev_test, completer_test). CI builds
images on tags only (no test/PR gate, per `feedback-ci-policy`).

---

## 12. Open questions / risks (resolve before/while building)

1. **Soccer per-game markets aren't posted yet** on either site this far out. The
   forecast path (title/group/advance) is fully live and buildable now; the tips
   path needs one live fixture to finalize the Kalshi ticker parse and the
   Polymarket game-slug discovery. Until then tips fall back to algo — acceptable.
2. **Kalshi group-winner event tickers** under `KXWCGROUPWINNER` need confirming
   (one event vs one-per-group).
3. **Liquidity / null prices** near listing time → wide or absent odds; the
   `ok=false` + fallback path is the mitigation, but early group games may be algo
   for a while.
4. **Team mapping tables** (Kalshi codes, Polymarket names) are hand-maintained
   glue — the main correctness surface. Keep them beside each source with a test
   that every one of our 48 `Team`s resolves.
5. **The two bots will be similar but not identical** (different liquidity, vig,
   crypto-vs-USD audiences) — intended; they're two real data points.

---

## 13. Milestones

1. **M1 — scaffolding + math:** `market.go` (interface, vig, Poisson) +
   `marketbrain.go` + `MarketBrain` wired to fallback only (no source). Tests for
   the math. Builds, `kalshi`/`polymarket` selectable, everything falls back to
   algo.
2. **M2 — Polymarket forecast:** `source_polymarket.go` for title + group +
   advance (all live today). Forecast now market-driven; tips still algo.
3. **M3 — Kalshi forecast:** `source_kalshi.go` for title + group winner.
4. **M4 — tips (both):** game 1/X/2 fetch + fixture matching once soccer game
   markets post; finalize slug/ticker parse against a live fixture.
5. **M5 — polish:** coverage logging, `.env` examples, README + main.go header
   docs, register the two bot users.

Each milestone is independently shippable (forecast-only bots are useful before
game markets exist).
