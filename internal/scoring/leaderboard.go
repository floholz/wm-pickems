package scoring

import (
	"encoding/json"
	"sort"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Row is one player's standing in a League.
type Row struct {
	UserID         string `json:"userId"`
	Name           string `json:"name"`
	Total          int    `json:"total"`
	TipsPoints     int    `json:"tipsPoints"`
	ForecastPoints int    `json:"forecastPoints"`
	Predicted      int    `json:"predicted"` // # matches the user has tipped
	// Tiebreakers (also returned for transparency).
	ExactScores    int `json:"exactScores"`
	CorrectWinners int `json:"correctWinners"`
	GdDeviation    int `json:"gdDeviation"`
	// Forecast correct-pick counts (groups/advance/champion + R32..FINAL).
	Forecast map[string]int `json:"forecast"`
	lastEdit string         // earliest-wins; not serialized
}

// Leaderboard builds a League's standings using its scoring config and the
// agreed tiebreakers: points → #exact → #correct winners → smaller aggregate
// goal-difference deviation → fewer tips submitted → earliest last edit.
// Users who never submitted a tip are sorted to the bottom regardless.
func Leaderboard(app core.App, leagueID string) (map[string]any, error) {
	league, err := app.FindRecordById("leagues", leagueID)
	if err != nil {
		return nil, err
	}
	cfgID := league.GetString("scoringConfig")
	if cfgID == "" {
		if def, err := app.FindFirstRecordByFilter("scoring_configs", "isDefault = true"); err == nil {
			cfgID = def.Id
		}
	}

	members, err := app.FindRecordsByFilter("league_members",
		"league = {:l}", "", 0, 0, map[string]any{"l": leagueID})
	if err != nil {
		return nil, err
	}

	rows := make([]Row, 0, len(members))
	for _, m := range members {
		uid := m.GetString("user")
		u, err := app.FindRecordById("users", uid)
		if err != nil {
			continue
		}
		row := Row{UserID: uid, Name: u.GetString("name")}

		ms, _ := app.FindRecordsByFilter("match_scores",
			"user = {:u} && config = {:c}", "", 0, 0,
			map[string]any{"u": uid, "c": cfgID})
		for _, s := range ms {
			row.TipsPoints += s.GetInt("points")
			var comp tipComponents
			_ = json.Unmarshal([]byte(s.GetString("components")), &comp)
			if comp.Exact > 0 {
				row.ExactScores++
			}
			if comp.Tendency > 0 {
				row.CorrectWinners++
			}
			row.GdDeviation += comp.GdDev
		}

		if fs, err := app.FindFirstRecordByFilter("forecast_scores",
			"user = {:u} && config = {:c}",
			map[string]any{"u": uid, "c": cfgID}); err == nil {
			row.ForecastPoints = fs.GetInt("points")
			var bd struct {
				GroupsCorrect   int            `json:"groupsCorrect"`
				AdvanceCorrect  int            `json:"advanceCorrect"`
				RoundCorrect    map[string]int `json:"roundCorrect"`
				ChampionCorrect int            `json:"championCorrect"`
			}
			if json.Unmarshal([]byte(fs.GetString("breakdown")), &bd) == nil {
				f := map[string]int{
					"groups":   bd.GroupsCorrect,
					"advance":  bd.AdvanceCorrect,
					"champion": bd.ChampionCorrect,
				}
				for k, v := range bd.RoundCorrect {
					f[k] = v
				}
				row.Forecast = f
			}
		}

		row.Total = row.TipsPoints + row.ForecastPoints

		if n, err := app.CountRecords("tips", dbx.HashExp{"user": uid}); err == nil {
			row.Predicted = int(n)
		}

		// Earliest last-edit across this user's tips (earlier = better).
		if tps, _ := app.FindRecordsByFilter("tips",
			"user = {:u}", "-updated", 1, 0,
			map[string]any{"u": uid}); len(tps) > 0 {
			row.lastEdit = tps[0].GetString("updated")
		}
		rows = append(rows, row)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		aNone, bNone := a.Predicted == 0, b.Predicted == 0
		if aNone != bNone {
			return !aNone
		}
		if a.Total != b.Total {
			return a.Total > b.Total
		}
		if a.ExactScores != b.ExactScores {
			return a.ExactScores > b.ExactScores
		}
		if a.CorrectWinners != b.CorrectWinners {
			return a.CorrectWinners > b.CorrectWinners
		}
		if a.GdDeviation != b.GdDeviation {
			return a.GdDeviation < b.GdDeviation
		}
		if a.Predicted != b.Predicted {
			return a.Predicted < b.Predicted
		}
		return a.lastEdit < b.lastEdit
	})

	return map[string]any{
		"league": map[string]any{"id": league.Id, "name": league.GetString("name")},
		"rows":   rows,
	}, nil
}
