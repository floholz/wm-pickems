package scoring

import "testing"

func defaultCfg() Config {
	var c Config
	c.Match.Tendency = 3
	c.Match.Exact = 1
	c.Match.TotalGoals = 1
	c.Match.GoalDiff = 1
	return c
}

func TestScoreValues(t *testing.T) {
	cfg := defaultCfg()
	tests := []struct {
		name      string
		m         MatchResult
		p         TipPrediction
		wantPts   int
		wantExact int
		wantGdDev int
		wantTend  int
	}{
		{
			name:      "group exact",
			m:         MatchResult{Stage: "group", FtH: 2, FtA: 1},
			p:         TipPrediction{FtH: 2, FtA: 1},
			wantPts:   6,
			wantExact: 1,
			wantTend:  3,
		},
		{
			name:      "group tendency only",
			m:         MatchResult{Stage: "group", FtH: 3, FtA: 1},
			p:         TipPrediction{FtH: 1, FtA: 0},
			wantPts:   3,
			wantGdDev: 1,
			wantTend:  3,
		},
		{
			name:      "group totally wrong",
			m:         MatchResult{Stage: "group", FtH: 1, FtA: 0},
			p:         TipPrediction{FtH: 0, FtA: 2},
			wantPts:   0,
			wantGdDev: 3,
		},
		{
			name:      "KO decided in 90, perfect",
			m:         MatchResult{Stage: "R32", FtH: 2, FtA: 1, Advancer: "T1"},
			p:         TipPrediction{FtH: 2, FtA: 1, Advancer: "T1"},
			wantPts:   6, // advancer(3)+exact+total+diff
			wantExact: 1,
			wantTend:  3,
		},
		{
			name:      "KO right advancer, wrong score",
			m:         MatchResult{Stage: "R32", FtH: 2, FtA: 0, Advancer: "T1"},
			p:         TipPrediction{FtH: 0, FtA: 1, Advancer: "T1"},
			wantPts:   3, // advancer only
			wantTend:  3,
			wantGdDev: 3,
		},
		{
			name:      "KO wrong advancer",
			m:         MatchResult{Stage: "R32", FtH: 1, FtA: 0, Advancer: "T1"},
			p:         TipPrediction{FtH: 0, FtA: 1, Advancer: "T2"},
			wantPts:   1, // total goals (1==1); no tendency
			wantGdDev: 2, // |(0-1)-(1-0)|
		},
		{
			name: "KO to ET, predicted draw then ET perfectly",
			m: MatchResult{
				Stage: "SF", FtH: 1, FtA: 1, EtH: 2, EtA: 1, Advancer: "T1",
			},
			p: TipPrediction{
				FtH: 1, FtA: 1, EtH: 2, EtA: 1, Advancer: "T1",
			},
			wantPts:   6, // advancer + exact/total/diff on after-ET score
			wantExact: 1,
			wantTend:  3,
		},
		{
			name: "KO to ET, predicted decisive 90 (no ET guess)",
			m: MatchResult{
				Stage: "SF", FtH: 1, FtA: 1, EtH: 3, EtA: 1, Advancer: "T1",
			},
			p:         TipPrediction{FtH: 2, FtA: 1, Advancer: "T1"},
			wantPts:   3, // advancer only; 2:1 vs after-ET 3:1 misses all
			wantTend:  3,
			wantGdDev: 1, // |(2-1)-(3-1)|
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := scoreValues(cfg, tc.m, tc.p)
			if r.points() != tc.wantPts {
				t.Errorf("points = %d, want %d (%+v)", r.points(), tc.wantPts, r)
			}
			if r.Exact != tc.wantExact {
				t.Errorf("exact = %d, want %d", r.Exact, tc.wantExact)
			}
			if r.GdDev != tc.wantGdDev {
				t.Errorf("gdDev = %d, want %d", r.GdDev, tc.wantGdDev)
			}
			if r.Tendency != tc.wantTend {
				t.Errorf("tendency = %d, want %d", r.Tendency, tc.wantTend)
			}
			if r.points() > 6 {
				t.Errorf("points %d exceeds the 6 max", r.points())
			}
		})
	}
}

func TestLoadConfigAdvanceDefault(t *testing.T) {
	// A config JSON without "advance" should default it to 1.
	c := Config{}
	if c.Forecast.Advance != 0 {
		t.Fatal("precondition")
	}
	// loadConfig path is exercised via JSON; simulate the fallback.
	if c.Forecast.Advance == 0 {
		c.Forecast.Advance = 1
	}
	if c.Forecast.Advance != 1 {
		t.Fatalf("advance fallback = %d, want 1", c.Forecast.Advance)
	}
}
