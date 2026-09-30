package analysis

import (
	"csdemoreview/worker/internal/maps"
	"csdemoreview/worker/internal/model"
	"math"
	"strings"
	"testing"
)

func TestIsolatedObservationRemainsReviewableWithoutFinding(t *testing.T) {
	input := snapInput(1)
	got := Analyze(input)
	if got.Review == nil || len(got.Review.Clips) == 0 || len(got.Review.Metrics) == 0 {
		t.Fatalf("an isolated fast flick was silently discarded: %+v", got.Review)
	}
	if len(got.Findings) != 0 || got.Verdict != "Insufficient data" {
		t.Fatalf("neutral observation became a cheating finding: %+v", got)
	}
	if got.Review.TotalShots != 1 || got.Review.SampledShots != 1 || got.Review.EligibleAimShots != 1 || got.Review.CoveredRounds != 1 {
		t.Fatalf("bad coverage: %+v", got.Review)
	}
	if !strings.Contains(got.Review.Summary, "Review complete") || !strings.Contains(got.Review.Summary, "No repeated rule triggers") {
		t.Fatal(got.Review.Summary)
	}
	if got.Review.MedianSampleMS == nil || *got.Review.MedianSampleMS != 15.63 {
		t.Fatal("sample cadence is not measured", got.Review.MedianSampleMS)
	}
}

func visibilityReviewInput(t *testing.T, episodes int, held bool) Input {
	t.Helper()
	input := snapInput(episodes)
	for i := range input.Samples {
		y := -20.0
		if i%3 == 2 {
			y = 20
		}
		input.Samples[i].Eye = &model.Vec3{Y: y, Z: 64}
		if held || i%3 == 2 {
			input.Samples[i].Yaw = math.Atan2(-y, 500) * 180 / math.Pi
		}
	}
	scene, err := maps.NewScene(model.MapAsset{Engine: "cs2", Map: "test", Version: "test"}, []maps.Triangle{
		{{X: 100, Y: -100, Z: 0}, {X: 100, Y: 0, Z: 0}, {X: 100, Y: 0, Z: 128}},
		{{X: 100, Y: -100, Z: 0}, {X: 100, Y: 0, Z: 128}, {X: 100, Y: -100, Z: 128}},
	}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	input.Visibility = scene
	return input
}

func capabilityBySignal(t *testing.T, result Result, signal string) model.Capability {
	t.Helper()
	for _, c := range result.Capabilities {
		if c.Signal == signal {
			return c
		}
	}
	t.Fatal("missing capability", signal)
	return model.Capability{}
}

func TestHeldAngleVisibilityHasMeasurementsWithoutReactionFindings(t *testing.T) {
	got := Analyze(visibilityReviewInput(t, 4, true))
	reaction := capabilityBySignal(t, got, "reaction")
	if reaction.Samples != 4 || reaction.MeasuredSamples != 4 || reaction.Status != "limited" || len(got.Findings) != 0 {
		t.Fatalf("held visibility result: %+v, findings %v", reaction, got.Findings)
	}
	found := false
	for _, metric := range got.Review.Metrics {
		if metric.Label == "Median estimated visibility-to-shot delay" && metric.Samples == 4 {
			found = true
		}
	}
	if !found {
		t.Fatal("visibility observations disappeared without acquisition", got.Review.Metrics)
	}
}

func TestVisibilitySmokeExclusionIsSpatialAndExpires(t *testing.T) {
	for _, scenario := range []string{"unknown", "near", "far", "expired"} {
		t.Run(scenario, func(t *testing.T) {
			input := visibilityReviewInput(t, 1, true)
			x, y, z := 250.0, 0.0, 0.0
			if scenario == "far" {
				y = 2000
			}
			start := model.GameEvent{Kind: "smoke-start", Time: input.Samples[0].Time - .01, X: &x, Y: &y, Z: &z}
			if scenario == "unknown" {
				start.X = nil
			}
			input.Events = append(input.Events, start)
			if scenario == "expired" {
				end := start
				end.Kind = "smoke-end"
				end.Time = input.Samples[0].Time - .005
				input.Events = append(input.Events, end)
			}
			got := Analyze(input)
			reaction := capabilityBySignal(t, got, "reaction")
			expected := 0
			if scenario == "far" || scenario == "expired" {
				expected = 1
			}
			if reaction.Samples != expected || got.Review.EligibleAimShots != 1 {
				t.Fatalf("visibility=%+v aim=%d", reaction, got.Review.EligibleAimShots)
			}
		})
	}
}

func TestSpottedTransitionsAreObserverSpecificMeasuredProxies(t *testing.T) {
	for _, scenario := range []string{"valid", "unknown", "other-observer", "persistent", "round-boundary", "dead-target", "gap"} {
		t.Run(scenario, func(t *testing.T) {
			input := snapInput(1)
			for i := range input.Samples {
				input.Samples[i].Yaw = 0
			}
			for i := range input.ContextSamples {
				input.ContextSamples[i].SpottedKnown = true
				if i%3 == 2 || scenario == "persistent" {
					input.ContextSamples[i].SpottedBy = []string{"a"}
				}
			}
			switch scenario {
			case "unknown":
				input.ContextSamples[1].SpottedKnown = false
			case "other-observer":
				input.ContextSamples[2].SpottedBy = []string{"teammate"}
			case "round-boundary":
				input.Rounds = []model.Round{{Number: 1, StartTick: 3}}
			case "dead-target":
				input.ContextSamples[1].Alive = false
			case "gap":
				input.ContextSamples[1].Time -= .1
			}
			got := Analyze(input)
			reaction := capabilityBySignal(t, got, "reaction")
			if reaction.Samples != 0 || len(got.Findings) != 0 || got.Verdict != "Insufficient data" {
				t.Fatal("proxy became a detector", got)
			}
			if scenario == "valid" {
				if reaction.Status != "measured" || reaction.MeasuredSamples != 1 || len(got.Review.Clips) == 0 {
					t.Fatal("proxy lost", got.Review, reaction)
				}
			} else if reaction.MeasuredSamples != 0 {
				t.Fatal("invalid proxy measured", reaction)
			}
		})
	}
}

func TestGeometryAndProxyCountUniqueObservedShots(t *testing.T) {
	input := visibilityReviewInput(t, 1, true)
	for i := range input.ContextSamples {
		input.ContextSamples[i].SpottedKnown = true
		if i == 2 {
			input.ContextSamples[i].SpottedBy = []string{"a"}
		}
	}
	c := capabilityBySignal(t, Analyze(input), "reaction")
	if c.Samples != 1 || c.MeasuredSamples != 1 {
		t.Fatal("same shot counted twice", c)
	}
}

func TestUsefulReviewIsNotBlockedByMissingIndependentFamilies(t *testing.T) {
	input := snapInput(25)
	for i := range input.Samples {
		input.Samples[i].Yaw = 0
		input.Samples[i].Scoped = true
	}
	got := Analyze(input)
	if got.Verdict != "Reviewed with limits" || len(got.Findings) != 0 || got.Review.EligibleAimShots != 25 {
		t.Fatal("supported clean review hidden", got)
	}
	if Aggregate(nil, []model.Capability{{Signal: "reaction", Status: "measured", Samples: 100, MeasuredSamples: 100}}, 10, false) != "Insufficient data" {
		t.Fatal("proxy qualified aggregate")
	}
}

func TestMixedPlayerInputDoesNotSuppressFirstShotsOrMutateSlices(t *testing.T) {
	input := snapInput(4)
	otherShot := input.Shots[0]
	otherShot.PlayerID = "other"
	otherShot.Time -= .01
	input.Shots = append(input.Shots, otherShot)
	otherSample := input.Samples[0]
	otherSample.PlayerID = "other"
	input.Samples = append(input.Samples, otherSample)
	got := Analyze(input)
	if got.Review.TotalShots != 4 || got.Review.EligibleAimShots != 4 || got.Verdict != "Suspicious" {
		t.Fatal("another player changed analysis", got)
	}
	if input.Shots[len(input.Shots)-1].PlayerID != "other" || input.Samples[len(input.Samples)-1].PlayerID != "other" {
		t.Fatal("input slices were mutated")
	}
}

func TestSingleSampleStillCountsAsRecordedShotCoverage(t *testing.T) {
	input := snapInput(1)
	input.Samples = input.Samples[2:]
	got := Analyze(input)
	if got.Review.SampledShots != 1 || got.Review.EligibleAimShots != 0 {
		t.Fatal(got.Review)
	}
}

func TestRoundBoundaryNeverCreatesSnapOrTimingEvidence(t *testing.T) {
	input := snapInput(4)
	for _, shot := range input.Shots {
		input.Rounds = append(input.Rounds, model.Round{Number: shot.Round, StartTick: shot.Tick})
	}
	got := Analyze(input)
	if len(got.Findings) != 0 || got.Review.EligibleAimShots != 0 {
		t.Fatal("round transition became evidence", got)
	}
}

func TestExcludedObservationsExplainWhyAndNeverBecomeFindings(t *testing.T) {
	input := snapInput(5)
	for _, s := range input.Shots {
		input.Events = append(input.Events, model.GameEvent{Kind: "spawn", PlayerID: "a", Time: s.Time - .2})
	}
	got := Analyze(input)
	if got.Review.EligibleAimShots != 0 || len(got.Findings) != 0 || len(got.Review.Clips) == 0 {
		t.Fatalf("spawn context result: %+v", got)
	}
	if len(got.Review.Exclusions) != 1 || got.Review.Exclusions[0].Count != 5 || !strings.Contains(got.Review.Exclusions[0].Reason, "spawn") {
		t.Fatal(got.Review.Exclusions)
	}
	for _, clip := range got.Review.Clips {
		if !strings.Contains(strings.Join(clip.Limitations, " "), "Excluded from aim/timing assessment") {
			t.Fatal("neutral clip hid its exclusion", clip)
		}
	}
}

func TestReviewNoSamplesNeverInventsMetrics(t *testing.T) {
	input := snapInput(1)
	input.Samples = nil
	got := Analyze(input)
	if got.Review == nil || got.Review.MedianSampleMS != nil || len(got.Review.Metrics) != 0 || len(got.Review.Clips) != 0 {
		t.Fatal(got.Review)
	}
	if len(got.Review.Exclusions) != 1 || got.Review.Exclusions[0].Count != 1 {
		t.Fatal("missing samples not explained", got.Review)
	}
}

func TestKillBookmarksDoNotAffectAssessment(t *testing.T) {
	input := Input{Demo: model.Demo{ID: "x"}, Player: model.Player{ID: "a"}, Events: []model.GameEvent{{ID: "kill", Kind: "kill", PlayerID: "a", Round: 1, Tick: 100, Weapon: "AWP", Headshot: true}}}
	got := Analyze(input)
	if len(got.Review.Clips) != 1 || len(got.Findings) != 0 || got.Verdict != "Insufficient data" {
		t.Fatal(got)
	}
}

func TestReviewCountsPartitionAllShotsAndBoundsClipCount(t *testing.T) {
	input := snapInput(20)
	input.Shots[0].Weapon = "Nova"
	input.Shots[1].TimingPrecision = .2
	input.Shots[2].Ambiguous = true
	input.Shots[3].Round = 0
	got := Analyze(input)
	total := got.Review.EligibleAimShots
	for _, exclusion := range got.Review.Exclusions {
		total += exclusion.Count
	}
	if total != len(input.Shots) || len(got.Review.Clips) > 16 {
		t.Fatalf("coverage accounting or bounded clip count failed: %+v", got.Review)
	}
	for i, clip := range got.Review.Clips {
		if i > 0 && clip.Tick < got.Review.Clips[i-1].Tick {
			t.Fatal("clips not ordered")
		}
	}
}
