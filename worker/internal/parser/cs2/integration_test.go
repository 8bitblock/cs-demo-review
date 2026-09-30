package cs2

import (
	"context"
	"csdemoreview/worker/internal/model"
	base "csdemoreview/worker/internal/parser"
	"csdemoreview/worker/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealFixtureWarmupEligibility(t *testing.T) {
	path := os.Getenv("CSDEMO_TEST_CS2")
	if path == "" {
		t.Skip("set CSDEMO_TEST_CS2 to a local CS2 recording")
	}
	s, e := store.Open(filepath.Join(t.TempDir(), "fixture.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	c := base.NewCollector(model.Demo{ID: "fixture", Engine: "cs2", Warnings: []string{}}, s, func(float64, string) {})
	if e = Parse(context.Background(), path, c); e != nil {
		t.Fatal(e)
	}
	if e = c.Finish(nil); e != nil {
		t.Fatal(e)
	}
	var live, excluded, native, punch, spotted int
	for id := range c.Players {
		shots, e := s.PlayerShots("fixture", id)
		if e != nil {
			t.Fatal(e)
		}
		for _, shot := range shots {
			if shot.NativeAngles != nil {
				native++
			}
			if shot.AimPunch != nil {
				punch++
				if shot.AimPunchScale == nil || *shot.AimPunchScale != 1 {
					t.Fatal("native CS2 punch must retain its effective shot-angle scale")
				}
			}
			if strings.Contains(shot.Provenance, "excluded from assessments") {
				excluded++
				if !shot.Ambiguous {
					t.Fatal("noncompetitive shot eligible")
				}
			} else {
				live++
			}
		}
		samples, e := s.PlayerSamples("fixture", id)
		if e != nil {
			t.Fatal(e)
		}
		for _, sample := range samples {
			if sample.SpottedKnown {
				spotted++
			}
		}
	}
	if live == 0 {
		t.Fatal("fixture yielded no competitive shots")
	}
	if os.Getenv("CSDEMO_TEST_EXPECT_NATIVE") == "1" && (native == 0 || punch == 0 || spotted == 0) {
		t.Fatalf("fixture native telemetry lost: directions=%d aim-punch=%d spotting samples=%d", native, punch, spotted)
	}
	t.Logf("map=%s version=%s ticks=%d competitive shots=%d excluded=%d native directions=%d aim-punch=%d spotting samples=%d", c.Demo.Map, c.Demo.MapVersion, c.Demo.TotalTicks, live, excluded, native, punch, spotted)
}
