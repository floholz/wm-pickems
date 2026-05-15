// Package seed populates teams, tournament groups and the 104-match fixture
// list from the embedded openfootball WC2026 dataset. It runs once on first
// boot (idempotent: guarded by an app_meta flag and skipped if teams exist).
package seed

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

//go:embed data/worldcup2026.json data/teams_meta2026.json
var dataFS embed.FS

type ofMatch struct {
	Round  string `json:"round"`
	Num    int    `json:"num"`
	Date   string `json:"date"`
	Time   string `json:"time"`
	Team1  string `json:"team1"`
	Team2  string `json:"team2"`
	Group  string `json:"group"`
	Ground string `json:"ground"`
}

type ofTeam struct {
	Name        string `json:"name"`
	FifaCode    string `json:"fifa_code"`
	FlagUnicode string `json:"flag_unicode"`
	Group       string `json:"group"`
	Confed      string `json:"confed"`
}

var (
	flagCPRe   = regexp.MustCompile(`1F1[0-9A-Fa-f]{2}`)
	roundStage = map[string]string{
		"Round of 32":           "R32",
		"Round of 16":           "R16",
		"Quarter-final":         "QF",
		"Semi-final":            "SF",
		"Match for third place": "3RD",
		"Final":                 "FINAL",
	}
)

// HomeNationISO maps FIFA codes that have no ISO-3166 country (UK home
// nations use emoji tag-sequences, not regional indicators) to the
// flag-icons file name.
var HomeNationISO = map[string]string{
	"ENG": "gb-eng",
	"SCO": "gb-sct",
	"WAL": "gb-wls",
	"NIR": "gb-nir",
}

// iso2FromFlag turns openfootball's "\u{1F1F2}\u{1F1FD}" regional-indicator
// escape into the ISO-3166 alpha-2 code ("mx") used for the bundled flag SVGs.
func iso2FromFlag(flagUnicode string) string {
	cps := flagCPRe.FindAllString(flagUnicode, 2)
	if len(cps) != 2 {
		return ""
	}
	var sb strings.Builder
	for _, c := range cps {
		v, err := strconv.ParseInt(c, 16, 32)
		if err != nil {
			return ""
		}
		sb.WriteRune(rune('a' + (v - 0x1F1E6)))
	}
	return sb.String()
}

// parseKickoff combines "2026-06-11" + "13:00 UTC-6" into a UTC time.
func parseKickoff(date, tm string) (time.Time, error) {
	parts := strings.Fields(tm) // ["13:00", "UTC-6"]
	clock := "00:00"
	offset := 0
	if len(parts) >= 1 {
		clock = parts[0]
	}
	if len(parts) >= 2 {
		off := strings.TrimPrefix(parts[1], "UTC")
		if n, err := strconv.Atoi(off); err == nil {
			offset = n
		}
	}
	loc := time.FixedZone("seed", offset*3600)
	t, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, loc)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func extID(m ofMatch, stage string) string {
	if stage == "group" {
		return fmt.Sprintf("WC2026-G-%s-%s-%s",
			strings.ReplaceAll(m.Group, " ", ""), slug(m.Team1), slug(m.Team2))
	}
	if m.Num > 0 {
		return fmt.Sprintf("WC2026-K-%d", m.Num)
	}
	return "WC2026-K-" + stage
}

func slug(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, s)
}

// DefaultScoringConfig — the agreed rules; tunable without code changes
// (per-League overrides reference a different scoring_configs record).
// Max 6 per game (group 1/X/2, KO = who advances; no separate advancer / ET
// bonus). Forecast: exact group position (+ perfect bonus), +advance per
// correctly-predicted advancer, escalating KO rounds.
const DefaultScoringConfig = `{
  "match": {
    "tendency": 3,
    "exact": 1,
    "totalGoals": 1,
    "goalDiff": 1
  },
  "forecast": {
    "groupPosition": 1,
    "perfectGroupBonus": 2,
    "advance": 1,
    "round": { "R32": 1, "R16": 2, "QF": 3, "SF": 5, "FINAL": 8, "CHAMPION": 13 }
  },
  "tiebreakers": ["points", "exactScores", "correctWinners", "goalDiffDeviation", "earliestEdit"]
}`

// ensureDefaultScoringConfig creates the default scoring config once.
func ensureDefaultScoringConfig(app core.App) error {
	if n, _ := app.CountRecords("scoring_configs"); n > 0 {
		return nil
	}
	col, err := app.FindCollectionByNameOrId("scoring_configs")
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("name", "Default")
	rec.Set("isDefault", true)
	rec.Set("config", DefaultScoringConfig)
	return app.Save(rec)
}

// Run seeds the database if it hasn't been seeded yet.
func Run(app core.App) error {
	if err := ensureDefaultScoringConfig(app); err != nil {
		return err
	}

	teamsCol, err := app.FindCollectionByNameOrId("teams")
	if err != nil {
		return err
	}
	if n, _ := app.CountRecords("teams"); n > 0 {
		return nil // already seeded
	}

	teamsRaw, err := dataFS.ReadFile("data/teams_meta2026.json")
	if err != nil {
		return err
	}
	var ofTeams []ofTeam
	if err := json.Unmarshal(teamsRaw, &ofTeams); err != nil {
		return err
	}

	matchesRaw, err := dataFS.ReadFile("data/worldcup2026.json")
	if err != nil {
		return err
	}
	var wc struct {
		Matches []ofMatch `json:"matches"`
	}
	if err := json.Unmarshal(matchesRaw, &wc); err != nil {
		return err
	}

	return app.RunInTransaction(func(txApp core.App) error {
		// Teams, keyed by openfootball display name for fixture resolution.
		byName := map[string]*core.Record{}
		groupTeams := map[string][]string{}
		for _, t := range ofTeams {
			rec := core.NewRecord(teamsCol)
			iso2 := iso2FromFlag(t.FlagUnicode)
			if h, ok := HomeNationISO[t.FifaCode]; ok {
				iso2 = h
			}
			rec.Set("fifaCode", t.FifaCode)
			rec.Set("name", t.Name)
			rec.Set("iso2", iso2)
			rec.Set("confederation", t.Confed)
			if err := txApp.Save(rec); err != nil {
				return fmt.Errorf("save team %s: %w", t.Name, err)
			}
			byName[t.Name] = rec
			groupTeams[t.Group] = append(groupTeams[t.Group], rec.Id)
		}

		// Tournament groups A..L.
		groupsCol, err := txApp.FindCollectionByNameOrId("tournament_groups")
		if err != nil {
			return err
		}
		for letter, ids := range groupTeams {
			rec := core.NewRecord(groupsCol)
			rec.Set("letter", letter)
			rec.Set("teams", ids)
			if err := txApp.Save(rec); err != nil {
				return fmt.Errorf("save group %s: %w", letter, err)
			}
		}

		// Matches.
		matchesCol, err := txApp.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		for _, m := range wc.Matches {
			stage := "group"
			if s, ok := roundStage[m.Round]; ok {
				stage = s
			}
			kickoff, err := parseKickoff(m.Date, m.Time)
			if err != nil {
				return fmt.Errorf("parse kickoff %q %q: %w", m.Date, m.Time, err)
			}
			rec := core.NewRecord(matchesCol)
			rec.Set("extId", extID(m, stage))
			rec.Set("stage", stage)
			rec.Set("num", m.Num)
			rec.Set("roundLabel", m.Round)
			rec.Set("kickoff", kickoff)
			rec.Set("status", "scheduled")
			if stage == "group" {
				rec.Set("groupLetter", strings.TrimPrefix(m.Group, "Group "))
				if h, ok := byName[m.Team1]; ok {
					rec.Set("homeTeam", h.Id)
				}
				if a, ok := byName[m.Team2]; ok {
					rec.Set("awayTeam", a.Id)
				}
			} else {
				// Knockout: teams unknown until results resolve; keep the
				// openfootball placeholder labels ("1A", "3A/B/C/D/F", "W74").
				rec.Set("homeLabel", m.Team1)
				rec.Set("awayLabel", m.Team2)
			}
			if err := txApp.Save(rec); err != nil {
				return fmt.Errorf("save match %s: %w", rec.GetString("extId"), err)
			}
		}
		return nil
	})
}
