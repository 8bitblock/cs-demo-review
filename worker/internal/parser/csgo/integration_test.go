package csgo

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

func TestRealFixtureMapIdentityAndEligibility(t *testing.T) {
	path := os.Getenv("CSDEMO_TEST_CSGO")
	if path == "" {
		t.Skip("set CSDEMO_TEST_CSGO to an official or local CS:GO recording")
	}
	s, e := store.Open(filepath.Join(t.TempDir(), "fixture.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	c := base.NewCollector(model.Demo{ID: "fixture", Engine: "csgo", Warnings: []string{}}, s, func(float64, string) {})
	if e = Parse(context.Background(), path, c); e != nil {
		t.Fatal(e)
	}
	if e = c.Finish(nil); e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(c.Demo.MapVersion, "crc32:") {
		t.Fatalf("missing recorded map CRC %+v", c.Demo)
	}
	var live, excluded, punch, spotted int
	for id := range c.Players {
		shots, e := s.PlayerShots("fixture", id)
		if e != nil {
			t.Fatal(e)
		}
		for _, shot := range shots {
			if shot.AimPunch != nil && strings.Contains(shot.Provenance, "sampled-recoil") {
				punch++
				if shot.AimPunchScale == nil || *shot.AimPunchScale != 2 {
					t.Fatal("sampled CS:GO punch must declare the nominal entity recoil scale")
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
	if os.Getenv("CSDEMO_TEST_EXPECT_NATIVE") == "1" && (punch == 0 || spotted == 0) {
		t.Fatalf("fixture telemetry lost: sampled recoil shots=%d spotting samples=%d", punch, spotted)
	}
	t.Logf("map=%s version=%s ticks=%d competitive shots=%d excluded=%d sampled recoil=%d spotting samples=%d", c.Demo.Map, c.Demo.MapVersion, c.Demo.TotalTicks, live, excluded, punch, spotted)
}
