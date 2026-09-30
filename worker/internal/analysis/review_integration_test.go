package analysis

import (
	"csdemoreview/worker/internal/model"
	"csdemoreview/worker/internal/store"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Opt-in integration verifies new review summaries against an existing library
// through a strictly read-only SQLite connection. Original demos and library
// records are never modified by this check.
func TestRealLibraryReview(t *testing.T) {
	path := os.Getenv("CS_DEMO_REVIEW_LIBRARY")
	if path == "" {
		t.Skip("Set CS_DEMO_REVIEW_LIBRARY for read-only real recording checks")
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	s := &store.Store{DB: db}
	demos, err := s.Demos()
	if err != nil {
		t.Fatal(err)
	}
	if len(demos) == 0 {
		t.Fatal("empty validation library")
	}
	results := []map[string]any{}
	for _, demo := range demos {
		players, err := s.Players(demo.ID)
		if err != nil {
			t.Fatal(err)
		}
		rounds, err := s.Rounds(demo.ID)
		if err != nil {
			t.Fatal(err)
		}
		events, err := s.Events(demo.ID)
		if err != nil {
			t.Fatal(err)
		}
		clips, metrics, shots, findings := 0, 0, 0, 0
		for _, player := range players {
			samples, err := s.PlayerSamples(demo.ID, player.ID)
			if err != nil {
				t.Fatal(err)
			}
			playerShots, err := s.PlayerShots(demo.ID, player.ID)
			if err != nil {
				t.Fatal(err)
			}
			result := Analyze(Input{Demo: demo, Player: player, Samples: samples, Shots: playerShots, Rounds: rounds, Events: events, Context: func(from, to int) []model.Sample {
				v, e := s.Samples(demo.ID, from, to)
				if e != nil {
					t.Fatal(e)
				}
				return v
			}})
			if result.Review == nil || result.Review.TotalShots != len(playerShots) {
				t.Fatalf("%s %s missing review", demo.Name, player.Name)
			}
			if len(playerShots) > 20 && (len(result.Review.Metrics) == 0 || len(result.Review.Clips) == 0) {
				t.Fatalf("%s %s has no useful recorded measurements: %+v", demo.Name, player.Name, result.Review)
			}
			accounted := result.Review.EligibleAimShots
			for _, x := range result.Review.Exclusions {
				accounted += x.Count
			}
			if accounted != len(playerShots) {
				t.Fatalf("shots not accounted for: %d/%d", accounted, len(playerShots))
			}
			clips += len(result.Review.Clips)
			metrics += len(result.Review.Metrics)
			shots += result.Review.TotalShots
			findings += len(result.Findings)
			if len(results) == 0 && player.ID == players[0].ID {
				t.Logf("Example review: %+v", result.Review)
			}
		}
		results = append(results, map[string]any{"demo": demo.Name, "map": demo.Map, "players": len(players), "shots": shots, "clips": clips, "metrics": metrics, "findings": findings, "analysisVersion": Version})
		t.Logf("%s: %d players, %d shots, %d neutral clips, %d measurements, %d findings", demo.Map, len(players), shots, clips, metrics, findings)
	}
	if output := os.Getenv("CS_DEMO_REVIEW_RESULTS"); output != "" {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
