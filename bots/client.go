package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a thin wrapper over the wm-pickems public REST API. A bot is just a
// normal authenticated user, so it goes through the same endpoints — and the
// same server-side locks — as a human player. No bypass anywhere.
type Client struct {
	baseURL string
	http    *http.Client
	token   string
	UserID  string
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		// PocketBase reads the auth token straight from the Authorization header.
		req.Header.Set("Authorization", c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// ---- auth ----

func (c *Client) Login(ctx context.Context, identity, password string) error {
	var out struct {
		Token  string `json:"token"`
		Record struct {
			ID string `json:"id"`
		} `json:"record"`
	}
	err := c.do(ctx, http.MethodPost, "/api/collections/users/auth-with-password",
		map[string]string{"identity": identity, "password": password}, &out)
	if err != nil {
		return err
	}
	c.token = out.Token
	c.UserID = out.Record.ID
	return nil
}

// JoinLeague joins a league by invite code (idempotent server-side). Optional —
// the bot can also be added to leagues directly in the PocketBase admin.
func (c *Client) JoinLeague(ctx context.Context, code string) error {
	return c.do(ctx, http.MethodPost, "/api/leagues/join",
		map[string]string{"code": code}, nil)
}

// ---- clock ----

// Now returns the server's notion of "now" (honours the WMP_DEV virtual clock),
// so the bot decides what is tippable using the same clock the locks use.
func (c *Client) Now(ctx context.Context) (time.Time, error) {
	var out struct {
		Now int64 `json:"now"` // unix millis
	}
	if err := c.do(ctx, http.MethodGet, "/api/now", nil, &out); err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(out.Now).UTC(), nil
}

// ---- reference data ----

type Team struct {
	ID       string `json:"id"`
	FifaCode string `json:"fifaCode"`
	Name     string `json:"name"`
}

type Match struct {
	ID          string `json:"id"`
	ExtID       string `json:"extId"`
	Stage       string `json:"stage"`
	Num         int    `json:"num"`
	GroupLetter string `json:"groupLetter"`
	HomeLabel   string `json:"homeLabel"`
	AwayLabel   string `json:"awayLabel"`
	HomeTeam    string `json:"homeTeam"`
	AwayTeam    string `json:"awayTeam"`
	Kickoff     string `json:"kickoff"`
	Status      string `json:"status"`
	// Result fields — populated once a match is finished (the feedback signal).
	FtHome      int    `json:"ftHome"`
	FtAway      int    `json:"ftAway"`
	EtHome      int    `json:"etHome"`
	EtAway      int    `json:"etAway"`
	FinalizedAt string `json:"finalizedAt"`
}

// Finished reports whether the match has a usable result.
func (m Match) Finished() bool {
	return m.Status == "finished" && m.HomeTeam != "" && m.AwayTeam != ""
}

func (c *Client) Teams(ctx context.Context) ([]Team, error) {
	var out struct {
		Items []Team `json:"items"`
	}
	err := c.do(ctx, http.MethodGet, "/api/collections/teams/records?perPage=500&skipTotal=1", nil, &out)
	return out.Items, err
}

func (c *Client) Matches(ctx context.Context) ([]Match, error) {
	var out struct {
		Items []Match `json:"items"`
	}
	err := c.do(ctx, http.MethodGet,
		"/api/collections/matches/records?perPage=500&skipTotal=1&sort=kickoff", nil, &out)
	return out.Items, err
}

// Structure is the Forecast builder payload: group memberships, the knockout
// skeleton with placeholder labels, the best-third slots, and FIFA's official
// Annex C third-place allocation table — everything needed to assemble a
// self-consistent bracket that the scoring engine will resolve identically.
type Structure struct {
	Groups []struct {
		Letter string   `json:"letter"`
		Teams  []string `json:"teams"`
	} `json:"groups"`
	Knockout []struct {
		Num       int    `json:"num"`
		Stage     string `json:"stage"`
		HomeLabel string `json:"homeLabel"`
		AwayLabel string `json:"awayLabel"`
	} `json:"knockout"`
	ThirdSlots []struct {
		MatchNum int      `json:"matchNum"`
		Winner   string   `json:"winner"`  // group-winner letter this slot pairs with
		Allowed  []string `json:"allowed"` // eligible group letters (fallback only)
	} `json:"thirdSlots"`
	ThirdTable map[string]map[string]string `json:"thirdTable"` // sortedKey -> {winnerLetter: thirdGroupLetter}
	Locked     bool                         `json:"locked"`
}

func (c *Client) Structure(ctx context.Context) (*Structure, error) {
	var s Structure
	err := c.do(ctx, http.MethodGet, "/api/forecast/structure", nil, &s)
	return &s, err
}

// ---- the bot's own predictions ----

type Tip struct {
	ID      string `json:"id"`
	Match   string `json:"match"`
	FtHome  int    `json:"ftHome"`
	FtAway  int    `json:"ftAway"`
	Updated string `json:"updated"`
}

func (c *Client) MyTips(ctx context.Context) ([]Tip, error) {
	var out struct {
		Items []Tip `json:"items"`
	}
	filter := url.QueryEscape(fmt.Sprintf("user='%s'", c.UserID))
	err := c.do(ctx, http.MethodGet,
		"/api/collections/tips/records?perPage=500&skipTotal=1&filter=("+filter+")", nil, &out)
	return out.Items, err
}

// CreateTip submits a per-match prediction. For group games only ftHome/ftAway
// are needed; for knockouts the bot predicts a decisive 90' score so the server
// derives the advancer (no extra-time handling in v1).
func (c *Client) CreateTip(ctx context.Context, matchID string, ftHome, ftAway int) error {
	return c.do(ctx, http.MethodPost, "/api/collections/tips/records", map[string]any{
		"user":   c.UserID,
		"match":  matchID,
		"ftHome": ftHome,
		"ftAway": ftAway,
	}, nil)
}

// UpdateTip revises an existing tip's scoreline (allowed while the match is
// still open — same lock the server enforces for humans).
func (c *Client) UpdateTip(ctx context.Context, tipID string, ftHome, ftAway int) error {
	return c.do(ctx, http.MethodPatch, "/api/collections/tips/records/"+tipID, map[string]any{
		"ftHome": ftHome,
		"ftAway": ftAway,
	}, nil)
}

// MyForecast returns the bot's existing Forecast id, or "" if none.
func (c *Client) MyForecast(ctx context.Context) (string, error) {
	var out struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	filter := url.QueryEscape(fmt.Sprintf("user='%s'", c.UserID))
	err := c.do(ctx, http.MethodGet,
		"/api/collections/forecasts/records?perPage=1&skipTotal=1&filter=("+filter+")", nil, &out)
	if err != nil || len(out.Items) == 0 {
		return "", err
	}
	return out.Items[0].ID, nil
}

// SaveForecast creates the bot's one-time pre-tournament Forecast. groupOrder is
// {letter: [teamId x4]}, thirdQualifiers is {letter: teamId} for the 8 chosen
// groups, bracket is {matchNum: winnerTeamId} — the exact shapes the scoring
// engine consumes.
func (c *Client) SaveForecast(ctx context.Context, order map[string][]string, thirds map[string]string, bracket map[string]string) error {
	return c.do(ctx, http.MethodPost, "/api/collections/forecasts/records", map[string]any{
		"user":            c.UserID,
		"groupOrder":      order,
		"thirdQualifiers": thirds,
		"bracket":         bracket,
	}, nil)
}

// UpdateForecast overwrites an existing forecast record (used to regenerate after
// a brain change). The server still rejects edits once the forecast has locked.
func (c *Client) UpdateForecast(ctx context.Context, forecastID string, order map[string][]string, thirds map[string]string, bracket map[string]string) error {
	return c.do(ctx, http.MethodPatch, "/api/collections/forecasts/records/"+forecastID, map[string]any{
		"groupOrder":      order,
		"thirdQualifiers": thirds,
		"bracket":         bracket,
	}, nil)
}
