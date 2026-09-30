package analysis

import (
	"csdemoreview/worker/internal/model"
	"fmt"
	"math"
	"sort"
	"strings"
)

func finiteVector(v *model.Vec3) bool {
	return v != nil && finite(v.X) && finite(v.Y) && finite(v.Z)
}
func validAngles(v *model.Vec3) bool {
	return finiteVector(v) && math.Abs(v.X) <= 90.1 && math.Abs(v.Y) <= 360
}
func punchScale(s model.Shot) (float64, bool) {
	if s.AimPunchScale == nil || !finite(*s.AimPunchScale) || *s.AimPunchScale <= 0 || *s.AimPunchScale > 4 {
		return 0, false
	}
	return *s.AimPunchScale, true
}
func nativeRecoil(s model.Shot) bool {
	_, hasScale := punchScale(s)
	return hasScale && !s.Ambiguous && s.Round > 0 && finiteVector(s.AimPunch) && validAngles(s.ViewAngles) && s.RecoilIndex != nil && finite(*s.RecoilIndex) && *s.RecoilIndex >= 0 && finite(s.Time) && finite(s.TimingPrecision) && s.TimingPrecision > 0 && s.TimingPrecision <= 0.065 && strings.Contains(s.Provenance, "shot-native") && weaponSupported(s.Weapon)
}

// Every interval of a burst must be observed, not just its shot endpoints.
// Ordinary motion remains measurable, but a missing frame, death, flash,
// team/weapon change or camera discontinuity creates a new run.
func recoilSampleInterval(samples []model.Sample, from, to int, stationary bool) bool {
	if from < 0 || to <= from || to >= len(samples) {
		return false
	}
	for i := from + 1; i <= to; i++ {
		a, b := samples[i-1], samples[i]
		if !safeSample(a, b) || (stationary && (a.Velocity > 20 || b.Velocity > 20)) {
			return false
		}
		// Speeds may themselves be derived from position deltas, so they cannot
		// enlarge the discontinuity limit and thereby excuse a teleport.
		limit := math.Max(80, (b.Time-a.Time)*1200)
		positionDistance := math.Sqrt(math.Pow(b.X-a.X, 2) + math.Pow(b.Y-a.Y, 2) + math.Pow(b.Z-a.Z, 2))
		eyeDistance := math.Sqrt(math.Pow(b.Eye.X-a.Eye.X, 2) + math.Pow(b.Eye.Y-a.Eye.Y, 2) + math.Pow(b.Eye.Z-a.Eye.Z, 2))
		if positionDistance > limit || eyeDistance > limit {
			return false
		}
	}
	return true
}

// Split at unusable telemetry rather than discarding an otherwise observable
// whole spray. Invalid shots remain barriers: the runs never bridge over one.
func recoilRuns(input Input, shots []model.Shot, strict bool) [][]model.Shot {
	runs := [][]model.Shot{}
	start := -1
	previousIndex := -1
	var previousSample model.Sample
	flush := func(end int) {
		if start >= 0 {
			runs = append(runs, shots[start:end])
			start = -1
		}
	}
	for i, s := range shots {
		p, index, ok := playerAt(input, s)
		valid := reviewShot(s, input.Player.ID) && recoilSource(s) != "" && ok && !p.Flashed && (p.Team == "T" || p.Team == "CT") && strings.EqualFold(p.Weapon, s.Weapon)
		if s.RecoilIndex != nil {
			valid = valid && finite(*s.RecoilIndex) && *s.RecoilIndex >= 0
		}
		if strict {
			valid = valid && nativeRecoil(s) && p.Velocity <= 20
		}
		if !valid {
			flush(i)
			previousIndex = -1
			continue
		}
		connected := start >= 0
		if connected {
			previous := shots[i-1]
			scale, _ := punchScale(s)
			previousScale, _ := punchScale(previous)
			connected = s.PlayerID == previous.PlayerID && s.Round == previous.Round && s.Weapon == previous.Weapon && s.Time > previous.Time && s.Time-previous.Time <= .2 && recoilSource(s) == recoilSource(previous) && scale == previousScale && recoilSampleInterval(input.Samples, previousIndex, index, strict)
			if strict {
				connected = connected && cohort(previousSample, previous) == cohort(p, s)
			}
			if s.RecoilIndex != nil && previous.RecoilIndex != nil {
				connected = connected && finite(*s.RecoilIndex) && finite(*previous.RecoilIndex) && *s.RecoilIndex >= *previous.RecoilIndex
				if strict {
					connected = connected && *s.RecoilIndex > *previous.RecoilIndex
				}
			}
		}
		if !connected {
			flush(i)
			start = i
		}
		previousIndex, previousSample = index, p
	}
	flush(len(shots))
	return runs
}
func analyseRecoil(input Input) ([]candidate, int) {
	result := []candidate{}
	eligible := 0
	// Shots are sorted by Analyze; each run contains only supported intervals.
	for _, group := range recoilRuns(input, input.Shots, true) {
		if len(group) < 7 {
			continue
		}
		residuals := []float64{}
		valid := true
		co := ""
		lastIndex := -1.0
		nativeMotion := 0.0
		for i, s := range group {
			if !nativeRecoil(s) || s.PlayerID != input.Player.ID {
				valid = false
				break
			}
			p, _, ok := playerAt(input, s)
			if !ok || p.Velocity > 20 || p.Flashed {
				valid = false
				break
			}
			current := cohort(p, s)
			if co == "" {
				co = current
			} else if co != current {
				valid = false
				break
			}
			if lastIndex >= 0 && *s.RecoilIndex <= lastIndex {
				valid = false
				break
			}
			lastIndex = *s.RecoilIndex
			if i > 0 {
				previous := group[i-1]
				scale, _ := punchScale(s)
				previousScale, _ := punchScale(previous)
				if scale != previousScale {
					valid = false
					break
				}
				punchPitch := scale * AngleDelta(s.AimPunch.X, previous.AimPunch.X)
				punchYaw := scale * AngleDelta(s.AimPunch.Y, previous.AimPunch.Y)
				nativeMotion += math.Hypot(punchPitch, punchYaw)
				viewPitch := AngleDelta(s.ViewAngles.X, previous.ViewAngles.X)
				viewYaw := AngleDelta(s.ViewAngles.Y, previous.ViewAngles.Y)
				residuals = append(residuals, math.Hypot(viewPitch+punchPitch, viewYaw+punchYaw))
			}
		}
		if !valid || len(residuals) < 6 || nativeMotion < 1 {
			continue
		}
		eligible++
		sum := 0.0
		max := 0.0
		for _, v := range residuals {
			sum += v
			max = math.Max(max, v)
		}
		mean := sum / float64(len(residuals))
		// This strict near-quantisation signature is deliberately not a detector of
		// ordinary skilled spray control. Without native evidence it stays disabled.
		if mean < 0.025 && max < 0.06 {
			f := finding(input, group[0], "recoil", "Repeated near-identical recoil cancellation", "Several comparable stationary sprays showed almost exact cancellation of validated shot-native aim-punch changes.", []model.Measurement{measure("Mean cancellation residual", mean, "°"), measure("Maximum residual", max, "°"), measure("Spray length", float64(len(group)), "shots"), measure("Native recoil movement", nativeMotion, "°")})
			f.EndTick = group[len(group)-1].Tick
			f.Limitations = append(f.Limitations, fmt.Sprintf("Uses the recorded source-specific aim-punch scale %.2g; packet/view timing and build semantics remain limitations.", *group[0].AimPunchScale), "Quantisation, spectator angle processing and practiced spray control remain alternative explanations.")
			result = append(result, candidate{finding: f, cohort: strings.ReplaceAll(co, "first-shot", "spray")})
		}
	}
	return result, eligible
}
func analyseDirection(input Input) ([]candidate, int) {
	result := []candidate{}
	eligible := 0
	for _, s := range input.Shots {
		scale, hasScale := punchScale(s)
		if !hasScale || s.PlayerID != input.Player.ID || s.Ambiguous || s.Round <= 0 || !validAngles(s.NativeAngles) || !validAngles(s.ViewAngles) || !finiteVector(s.AimPunch) || s.Spread == nil || s.Inaccuracy == nil || !strings.Contains(s.Provenance, "validated-shot-direction") || !weaponSupported(s.Weapon) || !finite(s.Time) || !finite(s.TimingPrecision) || s.TimingPrecision <= 0 || s.TimingPrecision > 0.065 {
			continue
		}
		p, _, ok := playerAt(input, s)
		if !ok || p.Flashed || p.Velocity > 20 {
			continue
		}
		if *s.Spread < 0 || *s.Inaccuracy < 0 || !finite(*s.Spread) || !finite(*s.Inaccuracy) {
			continue
		}
		eligible++
		expected := model.Vec3{X: s.ViewAngles.X + scale*s.AimPunch.X, Y: s.ViewAngles.Y + scale*s.AimPunch.Y}
		difference := angular(*s.NativeAngles, expected)
		spreadBound := math.Atan(*s.Spread+*s.Inaccuracy) * 180 / math.Pi
		excess := difference - spreadBound
		if excess > 5 {
			f := finding(input, s, "shot-direction", "Repeated native shot-direction discrepancy", "Validated native bullet directions repeatedly differed from sampled view plus recoil by more than the conservative spread envelope.", []model.Measurement{measure("Direction discrepancy", difference, "°"), measure("Spread envelope", spreadBound, "°"), measure("Excess difference", excess, "°")})
			f.Limitations = append(f.Limitations, "Impact endpoints, penetration segments and pellets are deliberately excluded from this test.", "Parser field semantics and recording timing should be independently checked before interpreting a discrepancy.")
			result = append(result, candidate{finding: f, cohort: cohort(p, s)})
		}
	}
	return result, eligible
}

// These observations expose usable demo telemetry without promoting sampled
// recoil, fire-packet orientation or impact rays into native anomaly findings.
type ballisticReviewResult struct {
	metrics                           []model.ReviewMetric
	clips                             []model.ReviewClip
	recoilObserved, directionObserved int
}

func recoilSource(s model.Shot) string {
	scale, hasScale := punchScale(s)
	if !hasScale || !finiteVector(s.AimPunch) || math.Abs(s.AimPunch.X) > 90 || math.Abs(s.AimPunch.Y) > 90 {
		return ""
	}
	if strings.Contains(s.Provenance, "shot-native") {
		return fmt.Sprintf("Shot-native aim-punch and recorded view angles; source-specific recoil scale %.2g.", scale)
	}
	if strings.Contains(s.Provenance, "sampled-recoil") {
		return fmt.Sprintf("Network-sampled aim-punch and view angles at weapon fire; assumed entity recoil scale %.2g.", scale)
	}
	return ""
}

func reviewShot(s model.Shot, playerID string) bool {
	return s.PlayerID == playerID && s.Round > 0 && !s.Ambiguous && weaponSupported(s.Weapon) && validAngles(s.ViewAngles) && finite(s.Time) && finite(s.TimingPrecision) && s.TimingPrecision > 0 && s.TimingPrecision <= .065
}

func directionSeparation(a, b model.Vec3) float64 {
	pa, pb, dy := a.X*math.Pi/180, b.X*math.Pi/180, AngleDelta(a.Y, b.Y)*math.Pi/180
	dot := math.Sin(pa)*math.Sin(pb) + math.Cos(pa)*math.Cos(pb)*math.Cos(dy)
	return math.Acos(math.Max(-1, math.Min(1, dot))) * 180 / math.Pi
}

func ballisticReview(input Input) ballisticReviewResult {
	out := ballisticReviewResult{}
	shots := append([]model.Shot(nil), input.Shots...)
	sort.SliceStable(shots, func(i, j int) bool { return shots[i].Time < shots[j].Time })
	addMetric := func(signal, label, unit, source string, values []float64) {
		if len(values) > 0 {
			out.metrics = append(out.metrics, model.ReviewMetric{Signal: signal, Label: label, Value: measure("", percentile(values, .5), unit).Value, Unit: unit, Samples: len(values), Provenance: source})
		}
	}
	compensation, residuals := []float64{}, []float64{}
	recoilClips, directionClips := []rankedClip{}, []rankedClip{}
	// Three-shot bursts yield two changes. Stationarity is unnecessary for a
	// neutral measurement; strict near-perfect-cancellation findings stay gated.
	for _, group := range recoilRuns(input, shots, false) {
		if len(group) < 3 {
			continue
		}
		source := recoilSource(group[0])
		if source == "" {
			continue
		}
		punchEnergy, cancelDot, residualSum, punchMotion := 0.0, 0.0, 0.0, 0.0
		valid := true
		for i, s := range group {
			if i == 0 {
				continue
			}
			prev := group[i-1]
			if s.RecoilIndex != nil && prev.RecoilIndex != nil && (!finite(*s.RecoilIndex) || !finite(*prev.RecoilIndex) || *s.RecoilIndex < *prev.RecoilIndex) {
				valid = false
				break
			}
			scale, _ := punchScale(s)
			px, py := scale*AngleDelta(s.AimPunch.X, prev.AimPunch.X), scale*AngleDelta(s.AimPunch.Y, prev.AimPunch.Y)
			vx, vy := AngleDelta(s.ViewAngles.X, prev.ViewAngles.X), AngleDelta(s.ViewAngles.Y, prev.ViewAngles.Y)
			punchEnergy += px*px + py*py
			punchMotion += math.Hypot(px, py)
			cancelDot -= vx*px + vy*py
			residualSum += math.Hypot(vx+px, vy+py)
		}
		if !valid || punchMotion < .2 || punchEnergy < 1e-8 {
			continue
		}
		percent := 100 * cancelDot / punchEnergy
		residual := residualSum / float64(len(group)-1)
		if !finite(percent) || !finite(residual) {
			continue
		}
		out.recoilObserved++
		compensation = append(compensation, percent)
		residuals = append(residuals, residual)
		first := group[0]
		recoilClips = append(recoilClips, rankedClip{rank: punchMotion, clip: model.ReviewClip{ID: fmt.Sprintf("%s:%s:review-recoil:%d", input.Demo.ID, input.Player.ID, first.Tick), Signal: "recoil", Provenance: source, Round: first.Round, Tick: first.Tick, EndTick: group[len(group)-1].Tick, Time: first.Time, Title: "Measured recoil compensation", Description: "Recorded view-angle changes are compared with aim-punch changes through a burst. 100% means the view motion cancels the recoil component on average; negative values follow it and values above 100% overcompensate.", Measurements: []model.Measurement{measure("Compensation along recoil", percent, "%"), measure("Mean cancellation residual", residual, "°"), measure("Recoil movement", punchMotion, "°"), measure("Burst length", float64(len(group)), "shots")}, Limitations: []string{"Neutral telemetry measurement; ordinary spray control and target tracking affect these values.", "Uses the source-specific aim-punch scale shown above; sampled fields do not establish raw mouse input or exact subtick ordering.", "This observation does not contribute to suspicious findings."}}})
	}
	addMetric("recoil", "Median compensation along recoil", "%", "Recorded view and aim-punch changes across bursts using source-specific scales. 100% is cancellation along the recoil component, not accuracy.", compensation)
	addMetric("recoil", "Median recoil cancellation residual", "°", "Mean per-shot residual within each measured burst; sampled and shot-native provenance is retained in review moments.", residuals)

	nativeOffsets, impactOffsets := []float64{}, []float64{}
	for _, s := range shots {
		if !reviewShot(s, input.Player.ID) {
			continue
		}
		p, _, ok := playerAt(input, s)
		if !ok || p.Flashed {
			continue
		}
		var direction model.Vec3
		title, source, label := "", "", ""
		if validAngles(s.NativeAngles) && strings.Contains(s.Provenance, "shot-native") {
			direction = *s.NativeAngles
			title = "Recorded fire-angle direction"
			source = "Native fire-packet orientation compared with weapon-fire view angles; final spread-adjusted bullet direction is not established."
			label = "View-to-fire-angle offset"
		} else if len(s.Impacts) == 1 && finiteVector(s.Origin) && finiteVector(&s.Impacts[0]) {
			v := s.Impacts[0]
			distance := math.Sqrt(math.Pow(v.X-s.Origin.X, 2) + math.Pow(v.Y-s.Origin.Y, 2) + math.Pow(v.Z-s.Origin.Z, 2))
			if !finite(distance) || distance < 32 {
				continue
			}
			direction = targetAngles(*s.Origin, v)
			title = "Recorded shot impact direction"
			source = "Straight ray from recorded shot origin to its single associated impact endpoint."
			label = "View-to-impact ray offset"
		} else {
			continue
		}
		offset := directionSeparation(*s.ViewAngles, direction)
		if !finite(offset) {
			continue
		}
		out.directionObserved++
		if label == "View-to-fire-angle offset" {
			nativeOffsets = append(nativeOffsets, offset)
		} else {
			impactOffsets = append(impactOffsets, offset)
		}
		directionClips = append(directionClips, rankedClip{rank: offset, clip: model.ReviewClip{ID: fmt.Sprintf("%s:%s:review-direction:%d", input.Demo.ID, input.Player.ID, s.Tick), Signal: "shot-direction", Provenance: source, Round: s.Round, Tick: s.Tick, EndTick: s.Tick, Time: s.Time, Title: title, Description: "The shot's recorded direction reference is compared with the player's sampled view. Larger offsets are bookmarked for manual review.", Measurements: []model.Measurement{measure(label, offset, "°"), measure("Direction pitch", direction.X, "°"), measure("Direction yaw", direction.Y, "°")}, Limitations: []string{"Neutral measurement; recoil, spread and event timing can explain differences.", "An impact ray is not the travelled bullet path. Multiple-impact shots are excluded from impact-ray measurements.", "Packet angles are not certified final bullet directions; neither source establishes a curved bullet or silent aim.", "Shot provenance: " + s.Provenance}}})
	}
	addMetric("shot-direction", "Median view-to-fire-angle offset", "°", "Native fire-packet orientation, not validated final bullet direction.", nativeOffsets)
	addMetric("shot-direction", "Median view-to-impact ray offset", "°", "Single associated impact endpoint relative to shot origin; includes recoil, spread and sampling effects.", impactOffsets)
	for _, clips := range [][]rankedClip{recoilClips, directionClips} {
		sort.SliceStable(clips, func(i, j int) bool {
			if clips[i].rank == clips[j].rank {
				return clips[i].clip.Tick < clips[j].clip.Tick
			}
			return clips[i].rank > clips[j].rank
		})
		for i := 0; i < len(clips) && i < 3; i++ {
			out.clips = append(out.clips, clips[i].clip)
		}
	}
	return out
}
