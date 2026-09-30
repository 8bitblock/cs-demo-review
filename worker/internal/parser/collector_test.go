package parser

import (
	"csdemoreview/worker/internal/model"
	"csdemoreview/worker/internal/store"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepeatedTickKeepsSpeedAndDropsInvalidOptionalTelemetry(t *testing.T) {
	s, e := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	c := NewCollector(model.Demo{ID: "demo"}, s, func(float64, string) {})
	c.Sample(model.Sample{PlayerID: "one", Tick: 64, Time: 1})
	c.Sample(model.Sample{PlayerID: "one", Tick: 65, Time: 1 + 1.0/64, X: 2})
	c.Sample(model.Sample{PlayerID: "one", Tick: 65, Time: 1 + 1.0/64, X: 2, Eye: &model.Vec3{X: math.NaN()}, AimPunch: &model.Vec3{Y: math.Inf(1)}, RecoilIndex: Ptr(math.NaN())})
	got := c.last["one"]
	if got.Velocity != 128 {
		t.Fatalf("duplicate game tick lost sampled movement: %v", got.Velocity)
	}
	if got.Eye != nil || got.AimPunch != nil || got.RecoilIndex != nil {
		t.Fatal("invalid optional telemetry must be omitted without losing player snapshot")
	}
	if e := c.Batch.Flush(); e != nil {
		t.Fatal(e)
	}
}

func TestMatchEligibilityPersistsWithoutRemovingReplayShots(t *testing.T) {
	s, e := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	c := NewCollector(model.Demo{ID: "demo", TickRate: 64}, s, func(float64, string) {})
	for i, tc := range []struct{ warmup, started, ambiguous bool }{{true, false, true}, {true, true, true}, {false, false, true}, {false, true, false}} {
		shot := model.Shot{PlayerID: "one", Weapon: "AK-47"}
		MatchEligibility(&shot, tc.warmup, tc.started)
		c.Tick = 100 + i*10
		c.Shot(shot)
		got := c.Pending[len(c.Pending)-1]
		if got.Ambiguous != tc.ambiguous {
			t.Fatalf("warmup=%v started=%v: %+v", tc.warmup, tc.started, got)
		}
		if tc.ambiguous && !strings.Contains(got.Provenance, "excluded from assessments") {
			t.Fatal("eligibility provenance was lost")
		}
	}
	c.FlushShots(true)
	if e = c.Batch.Flush(); e != nil {
		t.Fatal(e)
	}
	shots, e := s.PlayerShots("demo", "one")
	if e != nil || len(shots) != 4 {
		t.Fatal("noncompetitive shots must remain in replay")
	}
}
func TestMapCRCVersionRetainsUnknownAndLeadingZeros(t *testing.T) {
	if MapCRCVersion(0) != "" {
		t.Fatal("zero CRC cannot verify a map")
	}
	if MapCRCVersion(0x123) != "crc32:00000123" {
		t.Fatal("CRC formatting lost leading zeros")
	}
	if MapCRCVersion(0xFEDCBA98) != "crc32:fedcba98" {
		t.Fatal("CRC changed value")
	}
}

func TestDetectSignatures(t *testing.T) {
	for _, tc := range []struct {
		data, want string
		bad        bool
	}{{"PBDEMS2\x00payload", "cs2", false}, {"HL2DEMO\x00payload", "csgo", false}, {"PBDE", "", true}, {"something", "", true}} {
		path := filepath.Join(t.TempDir(), "not-trusted.dem")
		if e := os.WriteFile(path, []byte(tc.data), 0600); e != nil {
			t.Fatal(e)
		}
		got, e := Detect(path)
		if (e != nil) != tc.bad || got != tc.want {
			t.Fatalf("%q: %s, %v", tc.data, got, e)
		}
	}
}
func TestImpactAssociationDoesNotInventTrajectory(t *testing.T) {
	s, e := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	c := NewCollector(model.Demo{ID: "demo", TickRate: 64}, s, func(float64, string) {})
	c.Tick = 100
	c.Shot(model.Shot{PlayerID: "one", Weapon: "AK-47"})
	if c.Pending[0].TimingPrecision != 1.0/64 {
		t.Fatal("shot timing precision must use protocol seconds")
	}
	c.Impact("one", model.Vec3{X: 1})
	c.Impact("one", model.Vec3{X: 2})
	if len(c.Pending[0].Impacts) != 2 || c.Pending[0].Ambiguous {
		t.Fatal("penetration endpoints should remain one observed shot, not multiple direction findings")
	}
	c.Shot(model.Shot{PlayerID: "one", Weapon: "AK-47"})
	c.Impact("one", model.Vec3{X: 3})
	if !c.Pending[0].Ambiguous || !c.Pending[1].Ambiguous || len(c.Pending[1].Impacts) != 0 {
		t.Fatal("same-tick ambiguous association must be flagged and endpoint unassigned")
	}
	c.Tick = 110
	c.Impact("one", model.Vec3{X: 4})
	if len(c.Pending[0].Impacts) != 2 {
		t.Fatal("stale impact was associated")
	}
}
func TestMetadataRejectsDifferentGames(t *testing.T) {
	for _, dir := range []string{"csgo", "C:\\Steam\\game\\csgo", "/home/steam/csgo"} {
		if e := ValidateGame(dir); e != nil {
			t.Fatal(e)
		}
	}
	for _, dir := range []string{"", "dota", "csgo-malicious"} {
		if ValidateGame(dir) == nil {
			t.Fatalf("accepted %q", dir)
		}
	}
}
