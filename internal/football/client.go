// Package football is a thin client for the API-Football (api-sports.io) free
// tier. One /fixtures call returns all 104 WC2026 matches, so a periodic sync
// costs a single request and stays well within the 100/day free limit.
package football

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	baseURL  = "https://v3.football.api-sports.io"
	leagueID = 1    // FIFA World Cup
	season   = 2026 // WC 2026
)

type Client struct {
	key  string
	http *http.Client
}

func New(key string) *Client {
	return &Client{key: key, http: &http.Client{Timeout: 20 * time.Second}}
}

// Fixture is the subset of the API-Football fixture payload we use.
type Fixture struct {
	ID        int       // provider fixture id
	Date      time.Time // kickoff (UTC)
	Round     string    // e.g. "Group A - 1", "Round of 32"
	Status    string    // NS, 1H, HT, 2H, ET, BT, P, FT, AET, PEN, PST, CANC, ...
	HomeName  string
	AwayName  string
	HomeGoals *int // full 90' (nil if not played)
	AwayGoals *int
	FTHome    *int // regulation
	FTAway    *int
	ETHome    *int // after extra time (cumulative)
	ETAway    *int
	PenHome   *int // shootout
	PenAway   *int
}

// Finished reports whether the provider considers the match complete.
func (f Fixture) Finished() bool {
	switch f.Status {
	case "FT", "AET", "PEN", "WO":
		return true
	}
	return false
}

// Live reports whether the match is currently in progress.
func (f Fixture) Live() bool {
	switch f.Status {
	case "1H", "2H", "HT", "ET", "BT", "P", "LIVE", "INT":
		return true
	}
	return false
}

type apiResponse struct {
	Errors   json.RawMessage `json:"errors"`
	Results  int             `json:"results"`
	Response []struct {
		Fixture struct {
			ID     int       `json:"id"`
			Date   time.Time `json:"date"`
			Status struct {
				Short string `json:"short"`
			} `json:"status"`
		} `json:"fixture"`
		League struct {
			Round string `json:"round"`
		} `json:"league"`
		Teams struct {
			Home struct {
				Name string `json:"name"`
			} `json:"home"`
			Away struct {
				Name string `json:"name"`
			} `json:"away"`
		} `json:"teams"`
		Goals struct {
			Home *int `json:"home"`
			Away *int `json:"away"`
		} `json:"goals"`
		Score struct {
			Fulltime  scorePair `json:"fulltime"`
			Extratime scorePair `json:"extratime"`
			Penalty   scorePair `json:"penalty"`
		} `json:"score"`
	} `json:"response"`
}

type scorePair struct {
	Home *int `json:"home"`
	Away *int `json:"away"`
}

// Fixtures returns every WC2026 fixture in a single request.
func (c *Client) Fixtures(ctx context.Context) ([]Fixture, error) {
	return c.FixturesForSeason(ctx, season)
}

// FixturesForSeason fetches the World Cup fixtures for any season (used by the
// dev API diagnostic to replay a finished tournament, e.g. 2022).
func (c *Client) FixturesForSeason(ctx context.Context, yr int) ([]Fixture, error) {
	url := fmt.Sprintf("%s/fixtures?league=%d&season=%d", baseURL, leagueID, yr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-apisports-key", c.key)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api-football: status %d", resp.StatusCode)
	}

	var ar apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, err
	}
	if s := strings.TrimSpace(string(ar.Errors)); s != "" && s != "[]" && s != "{}" {
		return nil, fmt.Errorf("api-football errors: %s", s)
	}

	out := make([]Fixture, 0, len(ar.Response))
	for _, r := range ar.Response {
		out = append(out, Fixture{
			ID:        r.Fixture.ID,
			Date:      r.Fixture.Date.UTC(),
			Round:     r.League.Round,
			Status:    r.Fixture.Status.Short,
			HomeName:  r.Teams.Home.Name,
			AwayName:  r.Teams.Away.Name,
			HomeGoals: r.Goals.Home,
			AwayGoals: r.Goals.Away,
			FTHome:    r.Score.Fulltime.Home,
			FTAway:    r.Score.Fulltime.Away,
			ETHome:    r.Score.Extratime.Home,
			ETAway:    r.Score.Extratime.Away,
			PenHome:   r.Score.Penalty.Home,
			PenAway:   r.Score.Penalty.Away,
		})
	}
	return out, nil
}

// Status reports the account/plan and request quota (api-football /status).
func (c *Client) Status(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/status", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-apisports-key", c.key)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api-football: status %d", resp.StatusCode)
	}
	var out struct {
		Response map[string]any `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Response, nil
}

// NormalizeName lowercases and strips non-alphanumerics so provider team names
// can be matched against the openfootball-seeded names despite spelling
// differences ("Korea Republic" vs "South Korea" still need an alias map).
func NormalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
