package analysis

import (
	"csdemoreview/worker/internal/maps"
	"csdemoreview/worker/internal/model"
	"math"
	"testing"
)

func TestAngleWrap(t *testing.T) {
	for _, c := range []struct{ a, b, w float64 }{{1, 359, 2}, {359, 1, -2}, {-179, 179, 2}, {720, 0, 0}, {-1081, 0, -1}, {180, 0, -180}} {
		if got := AngleDelta(c.a, c.b); math.Abs(got-c.w) > 1e-8 {
			t.Fatalf("%v: %v", c, got)
		}
	}
}
func snapInput(episodes int) Input {
	input := Input{Demo: model.Demo{ID: "demo", TickRate: 64, Engine: "cs2", Map: "test", MapVersion: "test", Status: "ready"}, Player: model.Player{ID: "a"}}
	for i := 0; i < episodes; i++ {
		base := i*640 + 1
		time := float64(base) / 64
		for j := 0; j < 3; j++ {
			yaw := 30.0
			if j == 2 {
				yaw = 0
			}
			tick := base + j
			s := model.Sample{Tick: tick, Time: time + float64(j)/64, PlayerID: "a", Yaw: yaw, Alive: true, Team: "T", Weapon: "AK-47", Eye: &model.Vec3{Z: 64}}
			input.Samples = append(input.Samples, s)
			target := model.Sample{Tick: tick, Time: s.Time, PlayerID: "b", Alive: true, Team: "CT", Eye: &model.Vec3{X: 500, Z: 64}}
			input.ContextSamples = append(input.ContextSamples, target)
		}
		input.Shots = append(input.Shots, model.Shot{ID: "shot", Tick: base + 2, Time: time + 2.0/64, Round: i + 1, PlayerID: "a", Weapon: "AK-47", TimingPrecision: 1.0 / 64, Provenance: "test-fixture"})
	}
	return input
}
func TestIsolatedSkilledFlickNotFlagged(t *testing.T) {
	got := Analyze(snapInput(1))
	if len(got.Findings) != 0 || got.Verdict != "Insufficient data" {
		t.Fatalf("single flick: %+v", got)
	}
}
func TestRepeatedEpisodesProduceReviewEvidence(t *testing.T) {
	got := Analyze(snapInput(4))
	if len(got.Findings) < 4 || got.Verdict != "Suspicious" {
		t.Fatalf("repeated signal: %+v", got)
	}
	for _, f := range got.Findings {
		if f.Severity != "review" || len(f.Limitations) < 2 || len(f.Alternatives) == 0 {
			t.Fatalf("unexplained evidence: %+v", f)
		}
	}
}
func TestMissingAndLowPrecisionDataNotClean(t *testing.T) {
	input := snapInput(12)
	input.ContextSamples = nil
	got := Analyze(input)
	if got.Verdict != "Insufficient data" || len(got.Findings) != 0 {
		t.Fatalf("missing context: %+v", got)
	}
	input = snapInput(12)
	for i := range input.Shots {
		input.Shots[i].TimingPrecision = .1
	}
	got = Analyze(input)
	if len(got.Findings) != 0 {
		t.Fatal("low precision produced findings")
	}
}
func TestSamplingGapSuppressesSnap(t *testing.T) {
	input := snapInput(8)
	for i := range input.Samples {
		if i%3 == 1 {
			input.Samples[i].Time -= .1
		}
	}
	got := Analyze(input)
	if len(got.Findings) != 0 {
		t.Fatalf("gap generated %d findings", len(got.Findings))
	}
}
func TestVisualEventsDoNotDiscardAimMeasurements(t *testing.T) {
	for _, kind := range []string{"smoke-start", "flash", "door-move"} {
		input := snapInput(5)
		for _, s := range input.Shots {
			input.Events = append(input.Events, model.GameEvent{Kind: kind, Time: s.Time - .2, PlayerID: "a"})
		}
		if got := Analyze(input); got.Review.EligibleAimShots != 5 || len(got.Findings) == 0 {
			t.Fatalf("%s discarded recorded aim measurements", kind)
		}
	}
	input := snapInput(5)
	for i := range input.Samples {
		input.Samples[i].Flashed = true
	}
	if got := Analyze(input); len(got.Findings) > 0 {
		t.Fatal("flashed player generated evidence")
	}
}
func TestContinuousCS2PunchNotAccepted(t *testing.T) {
	input := snapInput(10)
	for i := range input.Samples {
		input.Samples[i].AimPunch = &model.Vec3{X: 2}
	}
	got := Analyze(input)
	for _, c := range got.Capabilities {
		if c.Signal == "recoil" && c.Status != "unsupported" {
			t.Fatal("continuous punch field should not enable recoil")
		}
	}
}
func TestImpactsAreNotBulletDirections(t *testing.T) {
	input := snapInput(5)
	for i := range input.Shots {
		input.Shots[i].Impacts = []model.Vec3{{X: 1}, {Y: 2}, {Z: 3}}
	}
	for _, f := range Analyze(input).Findings {
		if f.Signal == "shot-direction" {
			t.Fatal("penetration impacts interpreted as direction")
		}
	}
}
func TestCorrelatedSignalsDoNotProduceStrongVerdict(t *testing.T) {
	fs := []model.Finding{}
	for i := 0; i < 10; i++ {
		for _, signal := range []string{"acquisition", "reaction", "aim-snap"} {
			fs = append(fs, model.Finding{Signal: signal, EpisodeID: string(rune('a' + i)), Round: i + 1})
		}
	}
	if got := Aggregate(fs, nil, 10, false); got != "Suspicious" {
		t.Fatalf("got %s", got)
	}
	for i := 0; i < 8; i++ {
		fs = append(fs, model.Finding{Signal: "recoil", EpisodeID: string(rune('p' + i)), Round: i + 1})
	}
	if got := Aggregate(fs, nil, 10, false); got != "Highly suspicious" {
		t.Fatalf("got %s", got)
	}
}
func TestInsufficientCoverageNeverLowConcern(t *testing.T) {
	caps := []model.Capability{}
	for _, s := range []string{"acquisition", "aim-snap", "reaction", "recoil", "shot-direction"} {
		caps = append(caps, model.Capability{Signal: s, Status: "available", Samples: 40})
	}
	if Aggregate(nil, caps, 10, false) != "Low concern" {
		t.Fatal("full tested coverage expected low concern")
	}
	caps[0].Status = "unsupported"
	if Aggregate(nil, caps, 10, false) != "Reviewed with limits" {
		t.Fatal("missing family hid completed supported review")
	}
	caps[0].Status = "available"
	if Aggregate(nil, caps, 10, true) != "Reviewed with limits" {
		t.Fatal("partial recording should retain a limited review")
	}
}
func TestMapVersionMismatchDisablesReaction(t *testing.T) {
	input := snapInput(4)
	scene, err := maps.NewScene(model.MapAsset{Engine: "cs2", Map: "test", Version: "other"}, []maps.Triangle{{{X: -100, Y: -100, Z: 0}, {X: 100, Y: -100, Z: 0}, {X: 0, Y: 100, Z: 0}}}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	input.Visibility = scene
	for _, c := range Analyze(input).Capabilities {
		if c.Signal == "reaction" && c.Status != "unsupported" {
			t.Fatal("mismatched geometry enabled reaction")
		}
	}
}

func TestTeleportAndSpectatorTransitions(t *testing.T) {
	for _, mode := range []string{"teleport", "spectator", "respawn"} {
		input := snapInput(5)
		for i := range input.Samples {
			switch mode {
			case "teleport":
				if i%3 == 2 {
					input.Samples[i].X = 1000
				}
			case "spectator":
				input.Samples[i].Team = "Spectators"
			case "respawn":
				if i%3 == 1 {
					input.Samples[i].Alive = false
				}
			}
		}
		if got := Analyze(input); len(got.Findings) > 0 {
			t.Fatalf("%s generated findings", mode)
		}
	}
}
func TestLowCadenceHasCoverageAndBoundedTiming(t *testing.T) {
	input := snapInput(25)
	input.Demo.TickRate = 128
	for i := range input.Samples {
		base := int(math.Floor(input.Samples[i].Time/10)) * 1280
		local := i % 3
		input.Samples[i].Tick = base + local*5
		input.Samples[i].Time = float64(input.Samples[i].Tick) / 128
		input.ContextSamples[i].Tick = input.Samples[i].Tick
		input.ContextSamples[i].Time = input.Samples[i].Time
	}
	for i := range input.Shots {
		s := input.Samples[i*3+2]
		input.Shots[i].Tick = s.Tick
		input.Shots[i].Time = s.Time
		input.Shots[i].TimingPrecision = 1.0 / 128
	}
	got := Analyze(input)
	found := false
	for _, c := range got.Capabilities {
		if c.Signal == "aim-snap" && c.Status == "available" && c.Samples == 25 {
			found = true
		}
	}
	if !found {
		t.Fatalf("25.6fps samples lost coverage: %+v", got.Capabilities)
	}
	for _, f := range got.Findings {
		for _, m := range f.Measurements {
			if m.Label == "Timing uncertainty" && m.Value < 78 {
				t.Fatal("low cadence falsely precise", m)
			}
		}
	}
}
func TestHeldAnglesAreNotReactionEvidence(t *testing.T) {
	input := snapInput(5)
	for i := range input.Samples {
		input.Samples[i].Yaw = 0
	}
	got := Analyze(input)
	if len(got.Findings) > 0 {
		t.Fatal("held angle generated aim evidence")
	}
}

func recoilInput(residual float64) Input {
	input := Input{Demo: model.Demo{ID: "recoil", Engine: "cs2", TickRate: 64}, Player: model.Player{ID: "a"}}
	for round := 1; round <= 4; round++ {
		base := round * 640
		for tick := base - 1; tick <= base+36; tick++ {
			input.Samples = append(input.Samples, model.Sample{Tick: tick, Time: float64(tick) / 64, PlayerID: "a", Alive: true, Team: "T", Weapon: "AK-47", Eye: &model.Vec3{Z: 64}})
		}
		for j := 0; j < 7; j++ {
			tick := base + j*6
			index := float64(j)
			punch := &model.Vec3{X: float64(j) * .3}
			scale := 2.0
			input.Shots = append(input.Shots, model.Shot{Tick: tick, Time: float64(tick) / 64, Round: round, PlayerID: "a", Weapon: "AK-47", TimingPrecision: 1.0 / 64, ViewAngles: &model.Vec3{X: -2*punch.X + float64(j)*residual}, AimPunch: punch, AimPunchScale: &scale, RecoilIndex: &index, Provenance: "shot-native synthetic fixture"})
		}
	}
	return input
}
func TestNativeRecoilAndLegitimateControl(t *testing.T) {
	got := Analyze(recoilInput(0))
	if got.Verdict != "Suspicious" {
		t.Fatalf("native recoil fixture: %+v", got)
	}
	for _, f := range got.Findings {
		if f.Signal != "recoil" {
			t.Fatal("unexpected family")
		}
	}
	if got := Analyze(recoilInput(.2)); len(got.Findings) != 0 {
		t.Fatal("ordinary imperfect compensation flagged")
	}
	input := recoilInput(0)
	for i := range input.Shots {
		input.Shots[i].AimPunch = nil
	}
	if got := Analyze(input); len(got.Findings) != 0 {
		t.Fatal("missing punch inferred")
	}
}

func TestVisibilityTransitionAndDynamicExclusion(t *testing.T) {
	input := snapInput(4)
	for i := range input.Samples {
		y := -20.0
		if i%3 == 2 {
			y = 20
			input.Samples[i].Yaw = math.Atan2(-20, 500) * 180 / math.Pi
		}
		input.Samples[i].Eye = &model.Vec3{Y: y, Z: 64}
	}
	scene, e := maps.NewScene(model.MapAsset{Engine: "cs2", Map: "test", Version: "test"}, []maps.Triangle{{{X: 100, Y: -100, Z: 0}, {X: 100, Y: 0, Z: 0}, {X: 100, Y: 0, Z: 128}}, {{X: 100, Y: -100, Z: 0}, {X: 100, Y: 0, Z: 128}, {X: 100, Y: -100, Z: 128}}}, true, false)
	if e != nil {
		t.Fatal(e)
	}
	input.Visibility = scene
	count := 0
	for _, f := range Analyze(input).Findings {
		if f.Signal == "reaction" {
			count++
		}
	}
	if count != 4 {
		t.Fatalf("expected known visibility transitions, got %d", count)
	}
	scene.DynamicGeometry = true
	for _, f := range Analyze(input).Findings {
		if f.Signal == "reaction" {
			t.Fatal("unresolved dynamics generated reaction finding")
		}
	}
}
