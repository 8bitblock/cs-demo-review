package analysis

import (
	"csdemoreview/worker/internal/model"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestBallisticReviewMeasuresImperfectSpraysWithoutFindings(t *testing.T) {
	input := recoilInput(.2)
	got := ballisticReview(input)
	if got.recoilObserved != 4 || len(got.metrics) != 2 || len(got.clips) != 3 {
		t.Fatalf("missing measured sprays: %+v", got)
	}
	if math.Abs(got.metrics[0].Value-66.67) > .01 || math.Abs(got.metrics[1].Value-.2) > .001 {
		t.Fatalf("unexpected compensation/residual: %+v", got.metrics)
	}
	if findings, _ := analyseRecoil(input); len(findings) != 0 {
		t.Fatal("ordinary measured control became a finding")
	}
	for i := range input.Shots {
		input.Shots[i].Provenance = "weapon_fire; sampled-recoil CS:GO entity"
	}
	got = ballisticReview(input)
	if got.recoilObserved != 4 || !strings.Contains(got.clips[0].Provenance, "Network-sampled") {
		t.Fatal("sampled recoil lost its provenance", got)
	}
	if findings, count := analyseRecoil(input); len(findings) != 0 || count != 0 {
		t.Fatal("sampled recoil promoted to native anomaly test")
	}
}

func TestBallisticReviewDoesNotInferMissingRecoil(t *testing.T) {
	input := recoilInput(0)
	for i := range input.Shots {
		input.Shots[i].AimPunch = nil
	}
	for i := range input.Samples {
		input.Samples[i].AimPunch = &model.Vec3{X: 2}
	}
	if got := ballisticReview(input); got.recoilObserved != 0 {
		t.Fatal("unproven continuous punch consumed")
	}
	input = recoilInput(0)
	for i := range input.Shots {
		input.Shots[i].AimPunch.X = math.NaN()
	}
	if got := ballisticReview(input); got.recoilObserved != 0 {
		t.Fatal("non-finite recoil measured")
	}
	if _, count := analyseRecoil(input); count != 0 {
		t.Fatal("non-finite recoil counted as eligible")
	}
}

func TestNativePacketScaleIsNotDoubled(t *testing.T) {
	input := recoilInput(.2)
	for i := range input.Shots {
		s := &input.Shots[i]
		scale := 1.0
		s.AimPunchScale = &scale
		s.AimPunch.X *= 2 // CS2 Extra already contains the effective angle offset.
	}
	got := ballisticReview(input)
	if got.recoilObserved != 4 || math.Abs(got.metrics[0].Value-66.67) > .01 || math.Abs(got.metrics[1].Value-.2) > .001 {
		t.Fatalf("native offset doubled: %+v", got)
	}
	input = recoilInput(0)
	for i := range input.Shots {
		input.Shots[i].AimPunchScale = nil
	}
	if got := ballisticReview(input); got.recoilObserved != 0 {
		t.Fatal("unknown scale silently assumed")
	}
	if _, count := analyseRecoil(input); count != 0 {
		t.Fatal("unknown scale counted for strict recoil")
	}
}

func TestDirectionReviewSeparatesNativeAndImpactSources(t *testing.T) {
	input := snapInput(4)
	for i := range input.Shots {
		input.Shots[i].Origin = &model.Vec3{Z: 64}
		input.Shots[i].ViewAngles = &model.Vec3{}
		input.Shots[i].Impacts = []model.Vec3{{X: 100, Y: 100, Z: 64}}
	}
	input.Shots[0].NativeAngles = &model.Vec3{Y: 15}
	input.Shots[0].Provenance = "shot-native CS2 fire packet"
	input.Shots[2].Impacts = append(input.Shots[2].Impacts, model.Vec3{X: 200, Y: 200, Z: 64})
	input.Shots[3].Ambiguous = true
	got := ballisticReview(input)
	if got.directionObserved != 2 || len(got.metrics) != 2 {
		t.Fatalf("direction coverage %+v", got)
	}
	if got.metrics[0].Value != 15 || got.metrics[1].Value != 45 {
		t.Fatalf("wrong measured offsets %+v", got.metrics)
	}
	if findings, count := analyseDirection(input); len(findings) != 0 || count != 0 {
		t.Fatal("reference angles or impact rays became native bullet evidence")
	}
}

func TestDirectionReviewRejectsInvalidAndNearbyEndpoints(t *testing.T) {
	input := snapInput(3)
	for i := range input.Shots {
		input.Shots[i].Origin = &model.Vec3{Z: 64}
		input.Shots[i].ViewAngles = &model.Vec3{}
		input.Shots[i].Impacts = []model.Vec3{{X: 1, Z: 64}}
	}
	input.Shots[1].Impacts[0].X = math.NaN()
	input.Shots[2].NativeAngles = &model.Vec3{X: math.Inf(1)}
	input.Shots[2].Provenance = "shot-native"
	if got := ballisticReview(input); got.directionObserved != 0 {
		t.Fatal("invalid geometry measured", got)
	}
	if got := directionSeparation(model.Vec3{Y: 179}, model.Vec3{Y: -179}); math.Abs(got-2) > 1e-6 {
		t.Fatal("direction wrap", got)
	}
}

func ballisticBurstInput(count int) Input {
	input := Input{Demo: model.Demo{ID: "burst", TickRate: 64, Engine: "cs2"}, Player: model.Player{ID: "a"}}
	for tick := 99; tick <= 100+(count-1)*6; tick++ {
		input.Samples = append(input.Samples, model.Sample{PlayerID: "a", Tick: tick, Time: float64(tick) / 64, Team: "T", Alive: true, Weapon: "AK-47", Eye: &model.Vec3{Z: 64}})
	}
	for i := 0; i < count; i++ {
		index, scale := float64(i), 2.0
		tick := 100 + i*6
		input.Shots = append(input.Shots, model.Shot{PlayerID: "a", Round: 1, Tick: tick, Time: float64(tick) / 64, Weapon: "AK-47", TimingPrecision: 1.0 / 64, ViewAngles: &model.Vec3{X: -float64(i) * .4}, AimPunch: &model.Vec3{X: float64(i) * .3}, AimPunchScale: &scale, RecoilIndex: &index, Provenance: "shot-native fixture"})
	}
	return input
}

func TestRecoilRunsRecoverValidShotsAfterMissingNativeFields(t *testing.T) {
	for _, scenario := range []string{"first", "middle", "ambiguous-middle"} {
		t.Run(scenario, func(t *testing.T) {
			count, barrier, expected := 8, 0, 1
			if scenario != "first" {
				count, barrier, expected = 15, 7, 2
			}
			input := ballisticBurstInput(count)
			if scenario == "ambiguous-middle" {
				input.Shots[barrier].Ambiguous = true
			} else {
				input.Shots[barrier].AimPunch = nil
			}
			got := ballisticReview(input)
			_, strictCount := analyseRecoil(input)
			if got.recoilObserved != expected || strictCount != expected {
				t.Fatalf("valid runs lost: measured=%d strict=%d", got.recoilObserved, strictCount)
			}
			for _, clip := range got.clips {
				if clip.Tick <= input.Shots[barrier].Tick && clip.EndTick >= input.Shots[barrier].Tick {
					t.Fatal("run bridged missing or ambiguous shot", clip)
				}
				for _, m := range clip.Measurements {
					if m.Label == "Burst length" && m.Value != 7 {
						t.Fatal("valid run did not retain seven shots", m)
					}
				}
			}
		})
	}
}

func TestRecoilRunsKeepThreeAndSevenShotMinimums(t *testing.T) {
	for _, count := range []int{2, 3, 6, 7} {
		input := ballisticBurstInput(count)
		measured := ballisticReview(input).recoilObserved
		_, strict := analyseRecoil(input)
		wantMeasured, wantStrict := 0, 0
		if count >= 3 {
			wantMeasured = 1
		}
		if count >= 7 {
			wantStrict = 1
		}
		if measured != wantMeasured || strict != wantStrict {
			t.Fatalf("%d shots: measured=%d strict=%d", count, measured, strict)
		}
	}
}

func TestRecoilRunsNeverBridgeUnobservedOrDiscontinuousIntervals(t *testing.T) {
	for _, scenario := range []string{"death", "flash", "spectator", "team-change", "weapon-change", "vertical-teleport", "eye-teleport", "sampling-gap", "round", "recoil-reset"} {
		t.Run(scenario, func(t *testing.T) {
			input := ballisticBurstInput(14)
			for i := range input.Samples {
				s := &input.Samples[i]
				if s.Tick != 139 {
					continue
				}
				switch scenario {
				case "death":
					s.Alive = false
				case "flash":
					s.Flashed = true
				case "spectator":
					s.Team = "Spectator"
				case "team-change":
					s.Team = "CT"
				case "weapon-change":
					s.Weapon = "Knife"
				case "vertical-teleport":
					s.Z = 500
					s.Eye = &model.Vec3{Z: 564}
					s.Velocity = 32000
				case "eye-teleport":
					s.Eye = &model.Vec3{Z: 564}
					s.Velocity = 32000
				}
			}
			if scenario == "sampling-gap" {
				kept := []model.Sample{}
				for _, s := range input.Samples {
					if s.Tick <= 136 || s.Tick >= 142 {
						kept = append(kept, s)
					}
				}
				input.Samples = kept
			}
			if scenario == "round" {
				for i := 7; i < len(input.Shots); i++ {
					input.Shots[i].Round = 2
				}
			}
			if scenario == "recoil-reset" {
				for i := 7; i < len(input.Shots); i++ {
					index := float64(i - 7)
					input.Shots[i].RecoilIndex = &index
				}
			}
			got := ballisticReview(input)
			_, strict := analyseRecoil(input)
			if got.recoilObserved != 2 || strict != 2 {
				t.Fatalf("barrier ignored or good runs lost: measured=%d strict=%d", got.recoilObserved, strict)
			}
			for _, clip := range got.clips {
				if clip.Tick < 139 && clip.EndTick > 139 {
					t.Fatal("clip crossed interval barrier", clip)
				}
			}
		})
	}
}

func TestRecoilRunsAllowNeutralMovementWithoutCallingItStationary(t *testing.T) {
	input := ballisticBurstInput(7)
	for i := range input.Samples {
		input.Samples[i].X = float64(i) * 4
		input.Samples[i].Eye = &model.Vec3{X: float64(i) * 4, Z: 64}
		input.Samples[i].Velocity = 256
	}
	if got := ballisticReview(input); got.recoilObserved != 1 {
		t.Fatal("ordinary motion discarded neutral telemetry", got)
	}
	if _, count := analyseRecoil(input); count != 0 {
		t.Fatal("moving spray passed stationary detector")
	}
}

func TestAngularMeasurementsKeepSmallRecoilResidualPrecision(t *testing.T) {
	if got := measure("Residual", .0146, "°"); got.Value != .015 {
		t.Fatal("angular residual over-rounded", got)
	}
	if got := measure("Delay", .0146, "ms"); got.Value != .01 {
		t.Fatal("non-angular precision changed", got)
	}
	input := recoilInput(.0046)
	result := ballisticReview(input)
	if len(result.metrics) < 2 || result.metrics[1].Value != .005 {
		t.Fatal("metric angular residual over-rounded", result.metrics)
	}
	encoded, err := json.Marshal(result.metrics[1])
	if err != nil || !strings.Contains(string(encoded), `"value":0.005`) {
		t.Fatalf("serialized residual lost precision: %s %v", encoded, err)
	}
}
