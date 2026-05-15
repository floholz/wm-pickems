package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/wm-pickems/internal/seed"
)

// openfootball is the free live-results source: the same project we seed
// from publishes scores into 2026/worldcup.json during the tournament.
// Matches map 1:1 to our rows by the shared deterministic ExtID (no team
// name aliasing), and its `score.et` is already the cumulative after-120
// score — exactly our model.
const ofLiveURL = "https://raw.githubusercontent.com/openfootball/worldcup.json/master/2026/worldcup.json"

type ofScore struct {
	FT []int `json:"ft"`
	ET []int `json:"et"`
	P  []int `json:"p"`
}
type ofLiveMatch struct {
	Round string   `json:"round"`
	Num   int      `json:"num"`
	Team1 string   `json:"team1"`
	Team2 string   `json:"team2"`
	Group string   `json:"group"`
	Score *ofScore `json:"score"`
}

func pi(v int) *int { return &v }

// openfootballSync pulls openfootball's live JSON and applies any results.
// Idempotent: a record is only saved when something actually changed.
func openfootballSync(ctx context.Context, app core.App) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ofLiveURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "wm-tips/1.0")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("openfootball fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("openfootball: status %d", resp.StatusCode)
	}
	var doc struct {
		Matches []ofLiveMatch `json:"matches"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return err
	}

	byExt := map[string]*core.Record{}
	recs, err := app.FindRecordsByFilter("matches", "id != ''", "", 0, 0)
	if err != nil {
		return err
	}
	for _, r := range recs {
		byExt[r.GetString("extId")] = r
	}

	updated := 0
	for _, m := range doc.Matches {
		if m.Score == nil || len(m.Score.FT) != 2 {
			continue // not played yet
		}
		rec := byExt[seed.ExtID(m.Round, m.Num, m.Group, m.Team1, m.Team2)]
		if rec == nil {
			continue
		}
		ftH, ftA := m.Score.FT[0], m.Score.FT[1]
		var etH, etA, penH, penA *int
		if len(m.Score.ET) == 2 { // cumulative after-120
			etH, etA = pi(m.Score.ET[0]), pi(m.Score.ET[1])
		}
		if len(m.Score.P) == 2 {
			penH, penA = pi(m.Score.P[0]), pi(m.Score.P[1])
		}
		// Skip if nothing changed (avoids needless recompute storms).
		if rec.GetString("status") == "finished" &&
			rec.GetInt("ftHome") == ftH && rec.GetInt("ftAway") == ftA &&
			rec.GetInt("penHome") == ip(penH) && rec.GetInt("penAway") == ip(penA) &&
			rec.GetInt("etHome") == ip(etH) && rec.GetInt("etAway") == ip(etA) {
			continue
		}
		applyResult(rec, "finished", pi(ftH), pi(ftA), etH, etA, penH, penA)
		if app.Save(rec) == nil {
			updated++
		}
	}
	if err := ResolveBracket(app); err != nil {
		return err
	}
	return nil
}
