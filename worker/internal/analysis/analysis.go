// Package analysis implements experimental, explainable review heuristics.
// No result is a probability of cheating or a finding of guilt.
package analysis

import (
	"csdemoreview/worker/internal/maps"
	"csdemoreview/worker/internal/model"
	"fmt"
	"math"
	"sort"
	"strings"
)

const Version = "beta-rules-1.2.0"

type Input struct {
	Demo           model.Demo
	Player         model.Player
	Samples        []model.Sample
	Shots          []model.Shot
	Events         []model.GameEvent
	Rounds         []model.Round
	Visibility     *maps.Scene
	ContextSamples []model.Sample
	Context        func(fromTick, toTick int) []model.Sample
}
type Result struct {
	Findings     []model.Finding
	Capabilities []model.Capability
	Verdict      string
	Coverage     string
	Review       *model.ReviewSummary
}
type candidate struct {
	finding  model.Finding
	cohort   string
	eligible int
}

func AngleDelta(a, b float64) float64 {
	value := math.Mod(a-b, 360)
	if value >= 180 {
		value -= 360
	}
	if value < -180 {
		value += 360
	}
	return value
}
func angular(a, b model.Vec3) float64 { return math.Hypot(AngleDelta(a.X, b.X), AngleDelta(a.Y, b.Y)) }
func targetAngles(a, b model.Vec3) model.Vec3 {
	dx, dy, dz := b.X-a.X, b.Y-a.Y, b.Z-a.Z
	return model.Vec3{X: -math.Atan2(dz, math.Hypot(dx, dy)) * 180 / math.Pi, Y: math.Atan2(dy, dx) * 180 / math.Pi}
}
func finite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }
func validEye(s model.Sample) bool {
	return s.Eye != nil && finite(s.Eye.X) && finite(s.Eye.Y) && finite(s.Eye.Z) && finite(s.Yaw) && finite(s.Pitch) && math.Abs(s.Pitch) <= 90.1
}
func closest(samples []model.Sample, tick int, tolerance int) (model.Sample, bool) {
	i := sort.Search(len(samples), func(i int) bool { return samples[i].Tick >= tick })
	if i == len(samples) {
		i--
	} else if i > 0 && tick-samples[i-1].Tick < samples[i].Tick-tick {
		i--
	}
	if i < 0 || abs(samples[i].Tick-tick) > tolerance {
		return model.Sample{}, false
	}
	return samples[i], true
}
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func playerAt(input Input, shot model.Shot) (model.Sample, int, bool) {
	i := sort.Search(len(input.Samples), func(i int) bool { return input.Samples[i].Tick > shot.Tick }) - 1
	if i < 0 {
		return model.Sample{}, i, false
	}
	s := input.Samples[i]
	rate := input.Demo.TickRate
	if rate <= 0 || !finite(rate) {
		rate = 64
	}
	return s, i, s.Alive && validEye(s) && s.Time <= shot.Time && shot.Time-s.Time <= 0.065 && shot.Tick-s.Tick <= int(math.Ceil(rate*.065))
}
func safeSample(a, b model.Sample) bool {
	dt := b.Time - a.Time
	if !(dt > 0 && dt <= 0.065 && a.PlayerID == b.PlayerID && a.Alive && b.Alive && !a.Flashed && !b.Flashed && validEye(a) && validEye(b) && a.Team == b.Team && a.Weapon == b.Weapon) {
		return false
	}
	if a.Team != "T" && a.Team != "CT" {
		return false
	}
	distance := math.Sqrt(math.Pow(a.X-b.X, 2) + math.Pow(a.Y-b.Y, 2) + math.Pow(a.Z-b.Z, 2))
	eyeDistance := math.Sqrt(math.Pow(a.Eye.X-b.Eye.X, 2) + math.Pow(a.Eye.Y-b.Eye.Y, 2) + math.Pow(a.Eye.Z-b.Eye.Z, 2))
	// A generous discontinuity guard tolerates crouch offsets and legitimate
	// movement but rejects teleports, respawns and spectator camera switches.
	maximum := math.Max(80, dt*math.Max(600, 1.5*math.Max(a.Velocity, b.Velocity)))
	return finite(distance) && finite(eyeDistance) && distance <= maximum && eyeDistance <= maximum
}
func cohort(s model.Sample, shot model.Shot) string {
	movement := "stationary"
	if s.Velocity > 30 {
		movement = "moving"
	}
	return fmt.Sprintf("%s|%s|crouch=%t|scope=%t|first-shot", strings.ToLower(shot.Weapon), movement, s.Crouching, s.Scoped)
}
func weaponSupported(w string) bool {
	w = strings.ToLower(w)
	for _, bad := range []string{"knife", "grenade", "smoke", "flash", "molotov", "incendiary", "decoy", "zeus", "taser", "nova", "xm1014", "mag7", "mag-7", "sawedoff", "sawed-off", "unknown"} {
		if strings.Contains(w, bad) {
			return false
		}
	}
	return w != ""
}
func contextAmbiguous(input Input, shot model.Shot, s model.Sample) bool {
	return contextExclusion(input, shot, s) != ""
}
func contextExclusion(input Input, shot model.Shot, s model.Sample) string {
	if shot.Ambiguous {
		return "Ambiguous shot association or competitive phase"
	}
	if shot.Round <= 0 {
		return "Outside a recorded round"
	}
	if s.Flashed {
		return "Player flashed"
	}
	for _, event := range input.Events {
		kind := strings.ToLower(event.Kind)
		if (strings.Contains(kind, "spawn") || strings.Contains(kind, "connect") || strings.Contains(kind, "team-change")) && event.PlayerID == s.PlayerID && event.Time <= shot.Time && shot.Time-event.Time < 1 {
			return "Recent spawn, connection or team change"
		}
	}
	return ""
}

func eventPosition(e model.GameEvent) (model.Vec3, bool) {
	if e.X == nil || e.Y == nil || e.Z == nil || !finite(*e.X) || !finite(*e.Y) || !finite(*e.Z) {
		return model.Vec3{}, false
	}
	return model.Vec3{X: *e.X, Y: *e.Y, Z: *e.Z}, true
}
func pointSegmentDistance(p, a, b model.Vec3) float64 {
	dx, dy, dz := b.X-a.X, b.Y-a.Y, b.Z-a.Z
	denominator := dx*dx + dy*dy + dz*dz
	fraction := 0.0
	if denominator > 0 {
		fraction = math.Max(0, math.Min(1, ((p.X-a.X)*dx+(p.Y-a.Y)*dy+(p.Z-a.Z)*dz)/denominator))
	}
	return math.Sqrt(math.Pow(p.X-a.X-fraction*dx, 2) + math.Pow(p.Y-a.Y-fraction*dy, 2) + math.Pow(p.Z-a.Z-fraction*dz, 2))
}

// Obscurants affect visibility claims, not the existence of recorded aim angles.
// The spatial envelope is deliberately conservative, not a smoke simulation.
func visibilityContextExclusion(input Input, shot model.Shot, observer, target model.Sample) string {
	if observer.Scoped {
		return "Scoped field of view is not validated"
	}
	for _, event := range input.Events {
		kind := strings.ToLower(event.Kind)
		isSmoke := strings.Contains(kind, "smoke") && !strings.Contains(kind, "end") && !strings.Contains(kind, "expired")
		isDynamic := strings.Contains(kind, "door") || strings.Contains(kind, "breakable")
		if !(isSmoke && event.Time <= shot.Time && shot.Time-event.Time < 22) && !(isDynamic && math.Abs(event.Time-shot.Time) < 1) {
			continue
		}
		position, known := eventPosition(event)
		if !known {
			return "Visibility context has an obscurant or dynamic object without a position"
		}
		if pointSegmentDistance(position, *observer.Eye, *target.Eye) > 256 {
			continue
		}
		if isSmoke {
			expired := false
			for _, end := range input.Events {
				if end.Time < event.Time || end.Time > shot.Time || !(strings.Contains(end.Kind, "smoke") && (strings.Contains(end.Kind, "end") || strings.Contains(end.Kind, "expired"))) {
					continue
				}
				endPosition, ok := eventPosition(end)
				if ok && pointSegmentDistance(endPosition, position, position) <= 1 {
					expired = true
					break
				}
			}
			if expired {
				continue
			}
		}
		return "Smoke or dynamic geometry intersects the visibility exclusion envelope"
	}
	return ""
}
func roundStart(input Input, shot model.Shot) int {
	for _, round := range input.Rounds {
		if round.Number == shot.Round {
			return round.StartTick
		}
	}
	return 0
}
func inView(observer, target model.Sample) bool {
	look := targetAngles(*observer.Eye, *target.Eye)
	return math.Abs(AngleDelta(observer.Yaw, look.Y)) <= 45 && math.Abs(AngleDelta(observer.Pitch, look.X)) <= 35
}
func opponentContinuous(a, b model.Sample) bool {
	// The opponent's flash or weapon switch does not change their visibility.
	a.Flashed, b.Flashed = false, false
	a.Weapon, b.Weapon = "", ""
	return safeSample(a, b)
}
func visibilityTransition(input Input, shot model.Shot, observer, target model.Sample, index int, opponents []model.Sample) (float64, float64, bool) {
	if input.Visibility == nil || !input.Visibility.CanScore(input.Demo) || visibilityContextExclusion(input, shot, observer, target) != "" || !inView(observer, target) || !input.Visibility.Visible(*observer.Eye, *target.Eye) {
		return 0, 0, false
	}
	visibleStart := index
	laterTarget := target
	for j := index - 1; j >= 0 && shot.Time-input.Samples[j].Time < .5 && input.Samples[j].Tick >= roundStart(input, shot); j-- {
		a, b := input.Samples[j], input.Samples[j+1]
		if !safeSample(a, b) {
			break
		}
		p, ok := closest(opponents, a.Tick, 1)
		if !ok || p.Team != target.Team || !opponentContinuous(p, laterTarget) {
			break
		}
		moment := shot
		moment.Time = a.Time
		moment.Tick = a.Tick
		if visibilityContextExclusion(input, moment, a, p) != "" {
			break
		}
		if !inView(a, p) || !input.Visibility.Visible(*a.Eye, *p.Eye) {
			delay := shot.Time - input.Samples[visibleStart].Time
			uncertainty := math.Max(shot.TimingPrecision, observer.Time-input.Samples[index-1].Time) + b.Time - a.Time + math.Abs(p.Time-a.Time) + math.Abs(target.Time-observer.Time)
			return delay, uncertainty, delay >= 0 && finite(delay) && finite(uncertainty)
		}
		visibleStart = j
		laterTarget = p
	}
	return 0, 0, false
}
func spottedBy(sample model.Sample, observerID string) bool {
	for _, id := range sample.SpottedBy {
		if id == observerID {
			return true
		}
	}
	return false
}
func spottedTransition(input Input, shot model.Shot, observer, target model.Sample, index int, opponents []model.Sample) (float64, float64, bool) {
	if !target.SpottedKnown || !spottedBy(target, input.Player.ID) {
		return 0, 0, false
	}
	visibleStart := index
	laterTarget := target
	for j := index - 1; j >= 0 && shot.Time-input.Samples[j].Time < .5 && input.Samples[j].Tick >= roundStart(input, shot); j-- {
		a, b := input.Samples[j], input.Samples[j+1]
		if !safeSample(a, b) {
			break
		}
		p, ok := closest(opponents, a.Tick, 1)
		if !ok || !p.SpottedKnown || !opponentContinuous(p, laterTarget) || p.Team != target.Team {
			break
		}
		if !spottedBy(p, input.Player.ID) {
			delay := shot.Time - input.Samples[visibleStart].Time
			uncertainty := math.Max(shot.TimingPrecision, observer.Time-input.Samples[index-1].Time) + b.Time - a.Time + math.Abs(p.Time-a.Time) + math.Abs(target.Time-observer.Time)
			return delay, uncertainty, delay >= 0 && finite(delay) && finite(uncertainty)
		}
		visibleStart = j
		laterTarget = p
	}
	return 0, 0, false
}
func finding(input Input, shot model.Shot, signal, title, description string, measurements []model.Measurement) model.Finding {
	return model.Finding{ID: fmt.Sprintf("%s:%s:%s:%d", input.Demo.ID, input.Player.ID, signal, shot.Tick), DemoID: input.Demo.ID, PlayerID: input.Player.ID, Round: shot.Round, Tick: shot.Tick, EndTick: shot.Tick, Time: shot.Time, Signal: signal, Severity: "review", Title: title, Description: description, Measurements: measurements, EpisodeID: fmt.Sprintf("%s:%s:%d:%.0f", input.Demo.ID, input.Player.ID, shot.Round, math.Floor(shot.Time/1.5)), Alternatives: []string{"Prediction, practiced movement, sound cues or a skilled flick can produce similar recordings.", "Review the full round and the player's perspective before drawing a conclusion."}, Limitations: []string{"Experimental heuristic; thresholds have not been calibrated against independently labelled cheating recordings.", "Recorded angles and timestamps are sampled server observations, not raw mouse input or the player's display latency.", "Shot provenance: " + shot.Provenance}}
}
func measure(label string, value float64, unit string) model.Measurement {
	precision := 100.0
	if unit == "°" {
		precision = 1000
	}
	return model.Measurement{Label: label, Value: math.Round(value*precision) / precision, Unit: unit}
}
func Analyze(input Input) Result {
	result := Result{Findings: []model.Finding{}, Capabilities: []model.Capability{}, Verdict: "Insufficient data"}
	// Callers can reuse slices; the analyser never mutates their contents.
	samples := make([]model.Sample, 0, len(input.Samples))
	for _, s := range input.Samples {
		if s.PlayerID == input.Player.ID {
			samples = append(samples, s)
		}
	}
	shots := make([]model.Shot, 0, len(input.Shots))
	for _, s := range input.Shots {
		if s.PlayerID == input.Player.ID {
			shots = append(shots, s)
		}
	}
	input.Samples, input.Shots = samples, shots
	sort.Slice(input.Samples, func(i, j int) bool { return input.Samples[i].Tick < input.Samples[j].Tick })
	sort.Slice(input.Shots, func(i, j int) bool { return input.Shots[i].Time < input.Shots[j].Time })
	review := newReview(input)
	rate := input.Demo.TickRate
	if rate <= 0 || !finite(rate) {
		rate = 64
	}
	eligible := map[string]int{}
	seenRounds := map[int]bool{}
	assessedRounds := map[int]bool{}
	candidates := []candidate{}
	contextCount := 0
	sampledShots := 0
	for si, shot := range input.Shots {
		if shot.PlayerID != input.Player.ID {
			continue
		}
		if !weaponSupported(shot.Weapon) {
			review.exclude("Unsupported weapon or multiple-pellet weapon")
			continue
		}
		if !finite(shot.TimingPrecision) || shot.TimingPrecision <= 0 || shot.TimingPrecision > 0.065 {
			review.exclude("Shot timing missing or too coarse")
			continue
		}
		s, index, ok := playerAt(input, shot)
		if !ok {
			review.exclude("Missing, stale or invalid live player sample")
			continue
		}
		sampledShots++
		review.sampled++
		firstShot := si == 0 || shot.Round != input.Shots[si-1].Round || shot.Weapon != input.Shots[si-1].Weapon || shot.Time-input.Shots[si-1].Time >= .4
		review.observeAim(input, shot, s, index, firstShot)
		if reason := contextExclusion(input, shot, s); reason != "" {
			review.exclude(reason)
			continue
		}
		assessedRounds[shot.Round] = true
		if !firstShot {
			review.exclude("Ongoing burst or spray")
			continue
		}
		if index < 1 || !safeSample(input.Samples[index-1], s) {
			review.exclude("Aim sample gap, flash or movement discontinuity")
			continue
		}
		if input.Samples[index-1].Tick < roundStart(input, shot) {
			review.exclude("Aim samples cross a round boundary")
			continue
		}
		context := input.ContextSamples
		if input.Context != nil {
			context = input.Context(shot.Tick-int(rate*0.8)-2, shot.Tick+2)
		}
		byPlayer := map[string][]model.Sample{}
		for _, c := range context {
			if c.PlayerID != input.Player.ID && c.Team != s.Team && (c.Team == "T" || c.Team == "CT") {
				byPlayer[c.PlayerID] = append(byPlayer[c.PlayerID], c)
			}
		}
		for id := range byPlayer {
			sort.Slice(byPlayer[id], func(i, j int) bool { return byPlayer[id][i].Tick < byPlayer[id][j].Tick })
		}
		if len(byPlayer) == 0 {
			review.exclude("Synchronized opponent context unavailable")
			continue
		}
		co := cohort(s, shot)
		var target model.Sample
		targetID := ""
		best := math.Inf(1)
		for id, ss := range byPlayer {
			p, ok := closest(ss, s.Tick, 1)
			if !ok || !p.Alive || !validEye(p) {
				continue
			}
			error := angular(model.Vec3{X: s.Pitch, Y: s.Yaw}, targetAngles(*s.Eye, *p.Eye))
			if error < best || (error == best && (targetID == "" || id < targetID)) {
				best = error
				target = p
				targetID = id
			}
		}
		if targetID == "" {
			review.exclude("Valid live opponent eye position unavailable")
			continue
		}
		contextCount++
		seenRounds[shot.Round] = true
		review.eligible++
		review.targetErrors = append(review.targetErrors, best)
		eligible["aim-snap"]++
		eligible["acquisition"]++
		// Visibility observations are useful even for a held angle or a body shot.
		// They are not conditioned on the much stricter eye-aligned finding rule.
		delay, uncertainty, visibilityOK := visibilityTransition(input, shot, s, target, index, byPlayer[targetID])
		if best <= 5 && visibilityOK {
			eligible["reaction"]++
			review.observeVisibility(input, shot, delay, uncertainty, false)
		}
		proxyObserved := false
		if best <= 5 {
			if proxyDelay, proxyUncertainty, ok := spottedTransition(input, shot, s, target, index, byPlayer[targetID]); ok {
				review.observeVisibility(input, shot, proxyDelay, proxyUncertainty, true)
				proxyObserved = true
			}
		}
		if best <= 5 && (visibilityOK || proxyObserved) {
			eligible["reaction-observed"]++
		}
		// A target must be aligned at shot time. No inference is made from a large
		// flick by itself; the destination, timing and repeated cohort all matter.
		if best > 0.45 {
			continue
		}
		before := input.Samples[index-1]
		dt := s.Time - before.Time
		turn := angular(model.Vec3{X: s.Pitch, Y: s.Yaw}, model.Vec3{X: before.Pitch, Y: before.Yaw})
		speed := turn / dt
		if turn >= 18 && speed >= 1200 && best <= 0.3 && shot.Time-s.Time <= 0.02 {
			f := finding(input, shot, "aim-snap", "Repeated abrupt target alignment", "A sampled view change ended close to an opponent immediately before firing. Repeated comparable episodes are highlighted for review.", []model.Measurement{measure("Turn", turn, "°"), measure("Angular speed", speed, "°/s"), measure("Target error", best, "°"), measure("Sampling interval", dt*1000, "ms")})
			f.Limitations = append(f.Limitations, "Target eye-point alignment is an approximation of crosshair placement, not a hitbox or visibility test.")
			candidates = append(candidates, candidate{finding: f, cohort: co})
		}
		// Walk backwards through contiguous samples to find a genuine crossing,
		// bounded by a known outside sample. Long crosshair holds are not reactions.
		start := index
		crossed := false
		laterTarget := target
		for j := index - 1; j >= 0 && shot.Time-input.Samples[j].Time < 0.25 && input.Samples[j].Tick >= roundStart(input, shot); j-- {
			a, b := input.Samples[j], input.Samples[j+1]
			if !safeSample(a, b) {
				break
			}
			p, ok := closest(byPlayer[targetID], a.Tick, 1)
			if !ok || p.Team != target.Team || !opponentContinuous(p, laterTarget) {
				break
			}
			error := angular(model.Vec3{X: a.Pitch, Y: a.Yaw}, targetAngles(*a.Eye, *p.Eye))
			if error > 0.8 {
				crossed = true
				break
			}
			start = j
			laterTarget = p
		}
		if crossed {
			delay := shot.Time - input.Samples[start].Time
			uncertainty := math.Max(shot.TimingPrecision, dt) + (input.Samples[start].Time - input.Samples[start-1].Time)
			review.observeAcquisition(input, shot, delay, uncertainty)
			if delay >= 0 && delay+uncertainty < 0.085 {
				f := finding(input, shot, "acquisition", "Repeated short alignment-to-shot interval", "Firing followed estimated crosshair acquisition unusually quickly. This is alignment timing, not a measurement of visual reaction time.", []model.Measurement{measure("Observed delay", delay*1000, "ms"), measure("Upper delay bound", (delay+uncertainty)*1000, "ms"), measure("Timing uncertainty", uncertainty*1000, "ms")})
				f.Limitations = append(f.Limitations, "The target may already have been visible, predicted or heard. Acquisition timing cannot establish visual reaction speed.")
				candidates = append(candidates, candidate{finding: f, cohort: co})
			}
		}
		// An already aligned crosshair can be a held angle or prefire. Without
		// an observed acquisition transition, do not call it a visual reaction.
		if !crossed || !visibilityOK {
			continue
		}
		if delay >= 0 && delay+uncertainty < 0.1 {
			f := finding(input, shot, "reaction", "Repeated short estimated visibility-to-shot interval", "Firing followed the first unobstructed sampled eye-point observation in matching map geometry. Context exclusions and timing bounds are applied.", []model.Measurement{measure("Observed delay", delay*1000, "ms"), measure("Upper delay bound", (delay+uncertainty)*1000, "ms"), measure("Timing uncertainty", uncertainty*1000, "ms")})
			f.Limitations = append(f.Limitations, "Eye-point ray casting approximates visibility; other body parts may have been visible earlier.", "Field of view and map completeness are approximations; client rendering and latency are absent.")
			candidates = append(candidates, candidate{finding: f, cohort: co})
		}
	}
	recoilCandidates, recoilCount := analyseRecoil(input)
	candidates = append(candidates, recoilCandidates...)
	eligible["recoil"] = recoilCount
	directionCandidates, directionCount := analyseDirection(input)
	candidates = append(candidates, directionCandidates...)
	eligible["shot-direction"] = directionCount
	ballistics := ballisticReview(input)
	// Require repeated same-cohort observations spread across rounds, keeping the
	// individual episodes reviewable. Correlated acquisition/snap/reaction belong
	// to one family when aggregating assessments.
	grouped := map[string][]candidate{}
	// Cluster actual neighboring shots rather than fixed time buckets: two
	// measurements either side of a bucket boundary are still one encounter.
	episodesByTick := map[int]string{}
	episodeStart := 0
	previousTime := math.Inf(-1)
	previousRound := -1
	for _, shot := range input.Shots {
		if shot.PlayerID != input.Player.ID {
			continue
		}
		if shot.Round != previousRound || shot.Time-previousTime > 1.5 {
			episodeStart = shot.Tick
		}
		episodesByTick[shot.Tick] = fmt.Sprintf("%s:%s:%d:%d", input.Demo.ID, input.Player.ID, shot.Round, episodeStart)
		previousTime = shot.Time
		previousRound = shot.Round
	}
	for _, c := range candidates {
		if episode, ok := episodesByTick[c.finding.Tick]; ok {
			c.finding.EpisodeID = episode
		}
		key := c.finding.Signal + "|" + c.cohort
		grouped[key] = append(grouped[key], c)
	}
	for _, group := range grouped {
		episodes := map[string]bool{}
		rounds := map[int]bool{}
		for _, c := range group {
			episodes[c.finding.EpisodeID] = true
			rounds[c.finding.Round] = true
		}
		minimum := 4
		if group[0].finding.Signal == "recoil" {
			minimum = 3
		}
		if len(episodes) < minimum || len(rounds) < 3 {
			continue
		}
		for _, c := range group {
			f := c.finding
			f.Measurements = append(f.Measurements, measure("Comparable episodes", float64(len(episodes)), "episodes"), measure("Rounds represented", float64(len(rounds)), "rounds"))
			f.Description += " Comparable context: " + c.cohort + "."
			result.Findings = append(result.Findings, f)
		}
	}
	sort.Slice(result.Findings, func(i, j int) bool { return result.Findings[i].Tick < result.Findings[j].Tick })
	for _, signal := range []string{"reaction", "acquisition", "aim-snap", "recoil", "shot-direction"} {
		status, reason := "insufficient", "Too few eligible observations for a stable assessment."
		n := eligible[signal]
		measured := n
		basis := "Synchronized player and opponent samples"
		if n > 0 {
			status = "limited"
			reason = fmt.Sprintf("%d eligible observations evaluated; at least 20 are needed for broader coverage. Repeated findings use separate episode thresholds.", n)
		}
		if n >= 20 {
			status = "available"
			reason = "Experimental rule evaluated on comparable observations; absence of findings is not proof of legitimate play."
		}
		switch signal {
		case "reaction":
			measured = eligible["reaction-observed"]
			basis = "Verified matching static geometry; eye-point visibility approximation"
			if input.Visibility == nil || !input.Visibility.CanScore(input.Demo) {
				status = "unsupported"
				reason = "Verified matching, complete static geometry is unavailable, or unresolved dynamic geometry remains."
				basis = "No verified geometry available for visual reaction measurements"
			}
			if len(review.spotted) > 0 {
				if n == 0 {
					basis = "Observer-specific network spotted transitions; not geometric or client visibility"
				} else {
					basis += "; observer-specific spotted transitions are a separate network proxy"
				}
				if n == 0 {
					status = "measured"
					reason = fmt.Sprintf("%d observer-specific spotted-to-shot intervals measured. Spotted state can lag or persist; these are not visual reaction times and do not generate findings. %s", len(review.spotted), reason)
				}
			}
		case "acquisition", "aim-snap":
			if signal == "acquisition" {
				measured = len(review.acquisition)
				basis = "Observed opponent eye-point alignment crossings; visual reaction is assessed separately"
			} else {
				measured = len(review.turns)
				basis = "Recorded server view-angle changes; target-aligned findings require synchronized opponent context"
			}
			if contextCount == 0 {
				reason = "Synchronized target context or sufficiently precise aim samples are unavailable."
			}
		case "recoil":
			measured = ballistics.recoilObserved
			basis = "Shot-native or network-sampled aim-punch and view-angle changes; source-specific compensation scale"
			if n == 0 {
				status = "unsupported"
				reason = "No qualifying stationary seven-shot native sprays with sufficient recoil movement were observed. Continuous CS2 aim-punch fields are never used."
				if measured > 0 {
					status = "measured"
					reason = "Recorded aim-punch and view changes were measured for review. No qualifying stationary seven-shot native sprays with sufficient recoil movement were observed, so exact compensation findings remain unavailable."
				}
			}
		case "shot-direction":
			measured = ballistics.directionObserved
			basis = "Native firing angles or origin-to-impact observations, labelled by provenance"
			if n == 0 {
				status = "unsupported"
				reason = "Validated native bullet direction, recoil and spread bounds are absent. Impact endpoints cannot prove a direction change."
				if measured > 0 {
					status = "measured"
					reason = "Recorded firing or origin-to-impact directions measured for review. Missing validated recoil/spread semantics prevent discrepancy findings; impact paths are not proof of a direction change."
				}
			}
		}
		result.Capabilities = append(result.Capabilities, model.Capability{Signal: signal, Status: status, Reason: reason, Samples: n, MeasuredSamples: measured, Basis: basis})
	}
	result.Verdict = Aggregate(result.Findings, result.Capabilities, len(assessedRounds), input.Demo.Status == "partial")
	result.Review = review.finish(input, result, ballistics)
	result.Coverage = fmt.Sprintf("%d / %d shots had precise player samples; %d had target context across %d rounds. Experimental rules %s; unavailable signals remain unassessed.", sampledShots, len(input.Shots), contextCount, len(seenRounds), Version)
	if input.Demo.Status == "partial" {
		result.Coverage += " Recording is partial; broad low-concern assessment is unavailable."
	}
	return result
}

// Aggregate deduplicates episodes and signal families. More measurements of
// the same incident do not increase confidence.
func Aggregate(findings []model.Finding, capabilities []model.Capability, roundCount int, partial bool) string {
	families := map[string]map[string]bool{}
	rounds := map[int]bool{}
	all := map[string]bool{}
	for _, f := range findings {
		family := f.Signal
		if family == "reaction" || family == "acquisition" || family == "aim-snap" {
			family = "aim-timing"
		}
		if families[family] == nil {
			families[family] = map[string]bool{}
		}
		families[family][f.EpisodeID] = true
		all[f.EpisodeID] = true
		rounds[f.Round] = true
	}
	corroborating := 0
	for _, episodes := range families {
		if len(episodes) >= 4 {
			corroborating++
		}
	}
	if corroborating >= 2 && len(all) >= 8 && len(rounds) >= 5 {
		return "Highly suspicious"
	}
	if len(all) >= 4 && len(rounds) >= 3 {
		return "Suspicious"
	}
	available := 0
	for _, c := range capabilities {
		if c.Status == "available" && c.Samples >= 40 {
			available++
		}
	}
	// A clean partial recording or a recording with unassessed signal families
	// must not receive a broad low-concern verdict.
	if !partial && available == 5 && roundCount >= 8 && len(findings) == 0 {
		return "Low concern"
	}
	// Useful supported measurements must not be presented as an empty analysis
	// simply because a different signal needs unavailable telemetry or geometry.
	if roundCount >= 3 && len(findings) == 0 {
		for _, c := range capabilities {
			if c.Status == "available" && c.Samples >= 20 {
				return "Reviewed with limits"
			}
		}
	}
	return "Insufficient data"
}
