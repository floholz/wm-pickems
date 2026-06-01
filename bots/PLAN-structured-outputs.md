# Bot upgrade plan — structured outputs, EV selection, observability, richer prompts

> **Status: PLAN ONLY — review before any code is written.**
> Scope: the `bots/` side project (`github.com/floholz/wm-pickems/bots`). No changes to the
> main app, the PocketBase schema, or the server-side scorer. The `Predictor` interface
> keeps the two brains (`claude`/`brain.go`, `algo`/`algo.go`) interchangeable; `bracket.go`
> stays a faithful port of the server resolver.

This bundles four related threads agreed in discussion:

1. **Structured Outputs** — replace manual JSON extraction with schema-guaranteed output.
2. **Probabilities → EV selection** — elicit outcome distributions, pick the points-maximizing tip in shared code.
3. **Observability** — `slog` JSON-to-stdout telemetry (token/cache, tip diffs, optional rationale).
4. **Prompt content** — keep strength + form, guard against fabrication, add matchday + group-standings context.

---

## Guiding principles (carried from discussion)

- **Data-minimal / "just ask the AI what it'd pick."** Only feed context an informed human would already have in front of them (schedule position, current table). Exclude anything needing active research (injuries, lineups, news). No web/external lookup in v1.
- **Bots are fully self-contained and unaware of each other.** Shared code is fine; **shared data is not**. Each bot is its own process/container with its own env file. A brain's internal reasoning (Claude's elicited probabilities, the algo's ratings) stays private to that brain — the only thing it exposes is the distribution *it chose to return* through the `Predictor` interface. The leaderboard is the only comparison surface.
- **Caching discipline.** The large tournament reference is the cached system-prompt prefix and must stay byte-identical across calls. All dynamic content (results/form, standings, matchday) goes in the **user turn**, never the system prompt.
- **Shape vs semantics.** Structured outputs guarantees JSON *shape*, not *correctness*. Existing semantic validators (`repairOrder`, `chooseThirds`, clamps, draw-coercion) stay as the safety net.

---

## SDK facts (verified against `anthropic-sdk-go v1.46.0`)

- `MessageNewParams.OutputConfig` exists → `anthropic.OutputConfigParam{Format: anthropic.JSONOutputFormatParam{Schema: map[string]any{...}}}` (`message.go`). `Type` defaults to `json_schema`.
- Strict tool-use is also available (`ToolParam.Strict`), but **structured outputs is the recommended path** here — no `tool_use`/`tool_result` plumbing.
- Structured outputs is **compatible with streaming + adaptive thinking + prompt caching** (incompatible only with citations and assistant prefills, neither of which we use).
- Schema constraints to design around: every object needs `additionalProperties: false`; **no dynamic-keyed maps**; `enum`/nested arrays OK; **no** `minItems`/`maxItems`/`minLength`/numeric bounds. → drives the array-of-record redesign below.

---

## Thread 1 — Structured Outputs (✅ IMPLEMENTED)

> **Done:** `complete()` now sends `OutputConfig.Format` with a per-call JSON schema; `completeJSON`+`extractJSON` replaced by `completeStructured` (direct unmarshal of the schema-guaranteed reply). Schemas built via `obj`/`arr`/`str`/`int` helpers (`groupsSchema`/`winnersSchema`/`tipsSchema`), all array-of-records with `additionalProperties:false`. Response structs + fold-back keep the `Predictor` return types unchanged (tips still `map[string]Scoreline` — distribution comes in Thread 2). Semantic validators (`repairOrder`/`chooseThirds`/clamps/draw-coercion) retained. `brain_test.go` now asserts schema validity recursively. Build/vet/tests green.


### 1.1 Schema redesign (maps → arrays of records)

Current responses are dynamic-keyed maps (`{"groups":{"A":[...]}}`, `{"winners":{"73":"home"}}`,
`{"tips":{"<id>":[2,1]}}`) — none expressible under structured outputs. Switch to arrays of records.

**Groups** (`PredictGroups`):
```jsonc
{ "type":"object", "additionalProperties":false,
  "required":["groups","bestThirds"],
  "properties":{
    "groups":{"type":"array","items":{
      "type":"object","additionalProperties":false,
      "required":["letter","teamIds"],            // +"rationale" when enabled
      "properties":{
        "letter":{"type":"string"},
        "teamIds":{"type":"array","items":{"type":"string"}},
        "rationale":{"type":"string"}}}},
    "bestThirds":{"type":"array","items":{"type":"string"}}}}
```

**Winners** (`PredictWinners`): array of `{matchNum:int, side:"home"|"away"(enum)}` (+`rationale`).

**Tips** (`PredictTips`): array of records, scoreline as two ints (not a `[h,a]` tuple). With the
EV work (Thread 2) each record carries a small **candidate-score distribution** rather than a single
scoreline — see §2.2.

### 1.2 `brain.go` changes
- `complete()` gains `OutputConfig.Format` with the per-call schema. System block + `CacheControl` + adaptive thinking + streaming unchanged.
- New `completeStructured(ctx, task, schema, out)` → stream, accumulate, `json.Unmarshal` directly (output is guaranteed schema-valid).
- **Delete `extractJSON` and its test** (`brain_test.go`'s extract case).
- Each predict method builds its schema, unmarshals the record array, then folds back into the
  existing return types so `main.go` / `Predictor` are minimally affected (see §2.3 for the tips
  signature change).

### 1.3 What stays (semantic safety net)
`repairOrder`, `chooseThirds`, default-to-home on missing winner, clamp-negative scores,
coerce-draw-to-decisive on knockouts. Structured outputs does **not** guarantee 4 valid members
per group, exactly 8 thirds, or a decisive KO score.

### 1.4 Out of scope for this thread
`predictor.go` interface (except the tips return type, §2.3), `algo.go`, `bracket.go`, the run flow.

---

## Thread 2 — Probabilities → EV selection (✅ IMPLEMENTED)

> **Done:** new `ev.go` — `ScoringWeights` (hardcoded WC2026 3/1/1/1 default with a sync note), `points()`, pure `selectTip()` (model distribution → EV-maximizing scoreline against a model-candidates ∪ common-grid set; KO draws excluded), and the `TipOutcome`/`ScoreProb` types. `Predictor.PredictTips` now returns `map[string]TipOutcome`; Claude emits a candidate-score distribution (schema gains `scores:[{home,away,p}]`), algo returns a degenerate `p=1` distribution. `submitTips` calls `selectTip` to get the concrete scoreline. Semantic cleanup (clamps/KO-draw) moved into `selectTip`/`normalizeDist`. `ev_test.go` covers points, degenerate, dominant-tendency, and KO-decisive cases.
>
> **Also landed here (deferred from Thread 3): rationale.** `BOT_RATIONALE` flag → `Brain.rationale`; when on, all three schemas carry a `rationale` field. Tips rationale rides the `tip` event via `TipOutcome.Rationale`; groups/winners log `group_pick`/`winner_pick` events. Claude-only. Docs updated.


### 2.1 Why (tied to the live scoring rules)

Tips score (default config, max 6/match) is **separable**: correct result (1/X/2, or advancer) = **3**;
then exact score +1, correct total goals +1, correct goal difference +1. Because a dominant outcome
term sits beside three partial-credit shape terms, the **expected-points-maximizing scoreline can
differ from the single most-likely scoreline** (e.g. submit a home-win 2-1 to bank the 3-pt tendency
even if 1-1 is the modal exact score). EV selection captures that; "ask for one scoreline" doesn't.

### 2.2 Distribution shape (model output)

Each tip record returns a compact candidate-score distribution + rationale:
```jsonc
{ "key":"<matchId>",
  "scores":[ {"home":2,"away":1,"p":0.28}, {"home":1,"away":0,"p":0.18}, ... ],  // 3–5 candidates
  "rationale":"..." }                                                            // when enabled
```
Tendency probabilities (1/X/2, or advance for KO) are **derived in code** by summing `p` over the
relevant scorelines — no separate field, no inconsistency. (For KO the reference score may be ET;
v1 keeps the model on decisive 90' candidates and coerces draws out, consistent with today.)

### 2.3 The EV selector (shared, pure, stateless)

A pure function in shared bot code — the only "shared" piece; it holds no state and never sees more
than one bot's data:
```go
// selectTip picks the submission scoreline that maximizes expected points
// under cfg, given the model's candidate-score distribution.
func selectTip(o TipOutcome, stage string, cfg ScoringWeights) Scoreline
```
- **Candidate submissions:** the model's listed scores ∪ a small fixed common grid (0-0,1-0,1-1,2-0,2-1,2-2,…) so a "safe" pick not in the model's list is reachable.
- **EV of a candidate** = Σ over the distribution's true scores `p(true) · points(candidate, true, cfg)`, where `points` applies the 3/1/1/1 rule (tendency/advance + exact + total-goals + goal-diff). Pick argmax; tie-break toward the higher-probability tendency, then the more common scoreline.
- **`ScoringWeights`:** **DECIDED — hardcode** the documented default config (result 3, exact 1, total 1, diff 1) as constants, with a code note that these mirror the app's default `scoring_configs` for WC2026 and must be revisited if that default changes. No fetch from the app. Rationale: a user's tip is global (one per match, shared across leagues), so per-league configs can't be honored anyway — optimize for the global default.

### 2.4 Interface change
`Predictor.PredictTips` returns `map[string]TipOutcome` (was `map[string]Scoreline`). `submitTips`
in `main.go` calls `selectTip(...)` to get the concrete `Scoreline` before create/update.

- **Claude brain:** returns the rich distribution from §2.2.
- **Algo brain:** **DECIDED — stays simple** for now: returns a **degenerate distribution** (its single chosen scoreline at `p=1`); `selectTip` degenerates gracefully (EV = that score). Spreading the Elo win-prob into a real distribution is deferred to a dedicated **Elo-bot pass** (the algo brain is expected to get its own rework later — out of scope here).

### 2.5 Forecast (groups/winners) — scoping
- `PredictGroups`/`PredictWinners` may **elicit advance probabilities** for rationale/telemetry, but the bracket continues to use the existing greedy "pick the favorite per node" resolver (`bracket.go`), which approximates argmax-advancement.
- **DECIDED — greedy bracket stays**, in v1 and beyond. Champion/round probability-weighting (weighting the 13-pt champion pick by overall title probability rather than per-match favoritism) is **not planned** — deliberately keeping the forecast simple. Advance probabilities, if elicited at all, are for rationale/telemetry only and do not drive the resolver.

---

## Thread 3 — Observability (✅ IMPLEMENTED)

Decision: **`log/slog` JSON to stdout, with a text toggle. Loki/Grafana wiring deferred.**

> **Done:** `slog` swap in `main.go`+`brain.go`, `LOG_FORMAT` text/json handler, per-run `run_id` + process-constant `bot_kind`, `ai_call` events (token/cache/duration) in `complete()`, `tip` created/revised diff events in `submitTips`. Build/vet/tests green. Docs updated (`.env.example`, README).
> **Deferred to Thread 1/2:** the `rationale` field + `BOT_RATIONALE` flag — there's no model-produced rationale to log until the structured-output schemas carry it. Wire the flag and the field together with that work.

### 3.1 Logger setup
- Replace the bot's `log.Printf` with `log/slog` (stdlib, no new deps).
- Init once in `main.go`: `LOG_FORMAT=text` (default, human-readable for local dev) → `slog.NewTextHandler`; `LOG_FORMAT=json` (set in containers) → `slog.NewJSONHandler`; both → `os.Stdout`. Set as default logger.
- Base attributes via `logger.With(...)`: `bot_kind`, and a `run_id` stamped per `runOnce` (from start time) so a run's events correlate.
- Levels: `INFO` normal, `WARN` recoverable (e.g. a single `UpdateTip` failure currently swallowed), `ERROR` failed run.
- **stdout only** — no in-bot files, no PocketBase writes.

### 3.2 Event types

**`ai_call`** — emitted inside `complete()` (takes a task label):
```
level=INFO msg=ai_call task=tips model=claude-opus-4-8 in=312 out=1840 cache_read=8421 cache_create=0 dur_ms=4210
```
from accumulated `msg.Usage` (`InputTokens`/`OutputTokens`/`CacheReadInputTokens`/`CacheCreationInputTokens`) + `time.Since(start)`. `cache_read=0` across a run = the cached reference silently broke. Algo path emits none (no API calls).

**`tip`** — in `submitTips`, replacing the bare `created++/updated++`:
```
level=INFO msg=tip action=revised match=<id> home_team=Brazil away_team=Serbia old=2-1 new=1-1 trigger=new_result
level=INFO msg=tip action=created match=<id> new=2-0 trigger=first_tip
```
`action` ∈ created|revised, `trigger` ∈ first_tip|new_result. All from data `submitTips` already holds.

**rationale** — gated behind `BOT_RATIONALE` (default off). When on, the model's one-line reason rides as a `rationale` field on `tip` (and the forecast group/winner events). Claude-only; algo absent/empty.

### 3.3 Deferred / out
- No Loki/Alloy/Grafana wiring yet. **Upgrade path (code-free):** point a collector (Grafana Alloy/Promtail) at the JSON stdout → Loki → Grafana; later derive metrics (cache-hit ratio, token spend, tip churn) from log fields, or add a Prometheus pushgateway for alerting. Logs→Loki is the natural primary path for a 1h-loop job (not Prometheus scraping).
- Docs: add `LOG_FORMAT` + `BOT_RATIONALE` to `.env.example`, `claude.env`, README.

---

## Thread 4 — Prompt content

### 4.1 Signal buckets (what the bot can actually use)

| Signal | Source | Treatment |
|---|---|---|
| Historical team strength / FIFA pedigree | Claude training knowledge | Encourage (static, cached) |
| In-tournament form / previous results | We feed it (`buildResultsText`) | Encourage (dynamic, user turn) |
| Injuries / suspensions / lineups | **Not available, unknowable** | **Guard against fabrication** |
| Home/away | No venue in data | Clarify: nominal slots; only host-nation advantage is real |
| **Matchday + group standings** | **Derived from existing data** | **Add (group stage only, user turn)** |

### 4.2 Static guidance → cached `buildReference` system prompt
Augment the existing persona with:
- Weigh historical strength / squad pedigree / host advantage + the in-tournament form provided below; output the requested distribution.
- **Injury guard:** *"You have no live squad, lineup, injury, or suspension data. Do not invent or assume specific player availability. Base predictions on team strength, squad pedigree, and the in-tournament results provided."*
- **Home/away clarification:** *"Home/away positions are nominal fixture slots, not venues — do not infer a home advantage from them. The only genuine host advantage belongs to the host nations (USA, Canada, Mexico)."*

Opus 4.8 is literal/conservative and follows "don't fabricate" well, so the guard holds.

### 4.3 Dynamic context → user turn (group stage only)
- **Matchday tag (1/2/3):** derived by sorting each group's matches by `Kickoff` and chunking into pairs (MD3 games kick off simultaneously, so kickoff-ordering is robust). Each group tip target annotated, e.g. `[group MD3] Brazil vs Serbia`. KO targets keep their `Stage` (R32/R16/QF/…).
- **Group standings snapshot:** lightweight table per group with results — points, goal difference, goals for — computed by accumulating over finished group matches (data the feedback loop already pulls). **No FIFA tiebreaker resolution** (the model needs the picture, e.g. "Brazil 6pts +4 locked top; Serbia 0pts eliminated", not an exact rank). Empty before MD1 → no early overhead.
- **Form block (`buildResultsText`):** enrich cheaply with stage + recency ordering (newest last) so a KO upset is weighted differently from a dead-rubber group result. Stays in the user turn.

Why group-stage only: KO has no standings, and its "round" is already the stage. Matchday + standings are exactly the human reasoning for rotation / dead rubbers / must-win math (a side fixed 1st may rest stars in MD3).

---

## Cross-cutting

### Files likely touched
- `brain.go` — schemas, `completeStructured`, `OutputConfig`, `complete()` returns/logs usage, rationale, prompt builders, distribution output. Delete `extractJSON`.
- `predictor.go` — `PredictTips` return type → `map[string]TipOutcome`.
- `main.go` — `slog` init (`LOG_FORMAT`), `run_id`, `BOT_RATIONALE` wiring, `buildReference` augmentation, enriched `buildResultsText`, **new** standings + matchday derivation, `submitTips` calls `selectTip` + emits `tip`/`ai_call` events.
- **new** `ev.go` — `ScoringWeights`, `points(...)`, `selectTip(...)`, `TipOutcome`/`ScoreProb` (pure, shared, unit-tested).
- `algo.go` — `PredictTips` returns degenerate `TipOutcome`.
- `brain_test.go` — drop extract test, add unmarshal-per-schema tests.
- **new** `ev_test.go` — EV selection (tendency dominance, safe-scoreline preference, degenerate distribution).
- Docs: `.env.example`, `claude.env`, `README.md` (`LOG_FORMAT`, `BOT_RATIONALE`).

### Testing
- `ev_test.go`: confirm EV picks the dominant tendency over a higher-prob exact draw; degenerate (p=1) distribution returns that score; common-grid safe pick wins when it should.
- Schema unmarshal tests per response type.
- Keep `bracket_test.go` / `algo_test.go` green (no resolver/algo logic change beyond the tips return type).

### Suggested build order
1. **Thread 3 (observability)** — `slog` swap + the three events. Smallest, immediately useful, independent.
2. **Thread 1 (structured outputs)** — schema redesign + drop `extractJSON`.
3. **Thread 2 (EV)** — `ev.go` + `TipOutcome` + `selectTip`, distributions from both brains.
4. **Thread 4 (prompt content)** — guards + matchday/standings, folded into the now-structured prompts.

### Resolved decisions
1. **Forecast (§2.5):** greedy bracket stays — champion/title-probability weighting not planned. Forecast is kept deliberately simple.
2. **Scoring weights (§2.3):** hardcode the WC2026 default (3/1/1/1) with a code note to revisit if the app default changes. No app fetch.
3. **Algo distribution (§2.4):** degenerate `p=1` for now; richer Elo distribution deferred to a future dedicated Elo-bot rework (out of scope here).
