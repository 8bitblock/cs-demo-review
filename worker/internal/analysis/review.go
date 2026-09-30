package analysis

import (
	"csdemoreview/worker/internal/model"
	"fmt"
	"math"
	"sort"
	"strings"
)

type rankedClip struct {
	clip model.ReviewClip
	rank float64
}
type reviewBuilder struct {
	total, sampled, eligible                                           int
	rounds                                                             map[int]bool
	excluded                                                           map[string]int
	turns, speeds, cadence, acquisition, acquisitionBounds             []float64
	targetErrors, visibility, visibilityBounds, spotted, spottedBounds []float64
	aimClips, timingClips                                              []rankedClip
	visibilityClips                                                    []rankedClip
}

func newReview(input Input) *reviewBuilder {
	b := &reviewBuilder{rounds: map[int]bool{}, excluded: map[string]int{}}
	for _, shot := range input.Shots {
		if shot.PlayerID == input.Player.ID {
			b.total++
			if shot.Round > 0 {
				b.rounds[shot.Round] = true
			}
		}
	}
	for i := 1; i < len(input.Samples); i++ {
		a, z := input.Samples[i-1], input.Samples[i]
		if a.PlayerID == input.Player.ID && z.PlayerID == input.Player.ID && safeSample(a, z) {
			b.cadence = append(b.cadence, (z.Time-a.Time)*1000)
		}
	}
	return b
}
func (b *reviewBuilder) exclude(reason string) { b.excluded[reason]++ }

func (b *reviewBuilder) observeAim(input Input, shot model.Shot, sample model.Sample, index int, firstShot bool) {
	if index < 1 || !safeSample(input.Samples[index-1], sample) {
		return
	}
	before := input.Samples[index-1]
	dt := sample.Time - before.Time
	turn := angular(model.Vec3{X: sample.Pitch, Y: sample.Yaw}, model.Vec3{X: before.Pitch, Y: before.Yaw})
	speed := turn / dt
	if !finite(turn) || !finite(speed) {
		return
	}
	b.turns = append(b.turns, turn)
	b.speeds = append(b.speeds, speed)
	// Neutral bookmarks include aim changes which never pass target alignment,
	// repetition or contextual gates. They never feed Aggregate or findings.
	if !firstShot || shot.Round <= 0 || shot.Ambiguous || turn < 3 {
		return
	}
	limitations := []string{"Neutral review clip, not a cheating finding. A fast flick can be legitimate.", "Sampled server angles do not measure raw mouse input, reaction time or bullet direction.", "No visibility or target-hitbox claim is made; watch the surrounding play in game."}
	if reason := contextExclusion(input, shot, sample); reason != "" {
		limitations = append(limitations, "Excluded from aim/timing assessment: "+reason+".")
	}
	b.aimClips = append(b.aimClips, rankedClip{rank: turn, clip: model.ReviewClip{
		ID: fmt.Sprintf("%s:%s:review-aim:%d", input.Demo.ID, input.Player.ID, shot.Tick), Round: shot.Round, Tick: shot.Tick, EndTick: shot.Tick, Time: shot.Time,
		Signal: "aim-snap", Provenance: "Sampled server view angles",
		Title: "Aim turn before " + shot.Weapon + " shot", Description: fmt.Sprintf("A %.1f° recorded view change occurred across %.1f ms before this shot. Selected as one of this player's larger sampled turns for manual review.", turn, dt*1000),
		Measurements: []model.Measurement{measure("Sampled turn", turn, "°"), measure("Angular speed", speed, "°/s"), measure("Sample interval", dt*1000, "ms")}, Limitations: limitations}})
}

func (b *reviewBuilder) observeAcquisition(input Input, shot model.Shot, delay, uncertainty float64) {
	if delay < 0 || !finite(delay) || !finite(uncertainty) {
		return
	}
	b.acquisition = append(b.acquisition, delay*1000)
	b.acquisitionBounds = append(b.acquisitionBounds, (delay+uncertainty)*1000)
	b.timingClips = append(b.timingClips, rankedClip{rank: -(delay + uncertainty), clip: model.ReviewClip{
		ID: fmt.Sprintf("%s:%s:review-timing:%d", input.Demo.ID, input.Player.ID, shot.Tick), Round: shot.Round, Tick: shot.Tick, EndTick: shot.Tick, Time: shot.Time,
		Signal: "acquisition", Provenance: "Synchronized server samples; opponent eye-point approximation",
		Title: "Measured crosshair alignment before shot", Description: "The recording contains a sampled crossing toward an opponent's eye point before firing. The shortest measured intervals are bookmarked for review; this is not a visual reaction measurement.",
		Measurements: []model.Measurement{measure("Observed delay", delay*1000, "ms"), measure("Upper delay bound", (delay+uncertainty)*1000, "ms"), measure("Timing uncertainty", uncertainty*1000, "ms")},
		Limitations:  []string{"Neutral review clip, not a cheating finding. This bookmark does not establish repeated anomalous behavior.", "Opponent eye-point alignment approximates crosshair acquisition; hitboxes and earlier visibility are not established.", "Prediction, sound cues and practiced aim can explain short alignment intervals."}}})
}

func (b *reviewBuilder) observeVisibility(input Input, shot model.Shot, delay, uncertainty float64, proxy bool) {
	if delay < 0 || !finite(delay) || !finite(uncertainty) {
		return
	}
	title := "Estimated visibility before shot"
	description := "A sampled hidden-to-visible eye-point transition was observed in verified matching static geometry. This is a timing bookmark, not a finding of cheating."
	provenance := "Verified matching static geometry and synchronized eye positions"
	prefix := "visibility"
	limitations := []string{"Eye-point visibility and a conservative unscoped field of view approximate what could be seen; body parts may be visible earlier.", "Client rendering, display latency and player intent are unavailable. Held angles and prefires are legitimate explanations."}
	if proxy {
		b.spotted = append(b.spotted, delay*1000)
		b.spottedBounds = append(b.spottedBounds, (delay+uncertainty)*1000)
		title = "Observer-spotted state before shot"
		description = "The target's recorded spotted-by state changed from false to true for this player before firing. This network-state interval is a visibility proxy, not a visual reaction time."
		provenance = "Observer-specific network spotted-by state; no geometry claim"
		prefix = "spotted"
		limitations = []string{"Network spotted state can lag or persist and does not reproduce client rendering or line of sight.", "This proxy is measured for manual review only and never contributes to cheating findings or the overall assessment."}
	} else {
		b.visibility = append(b.visibility, delay*1000)
		b.visibilityBounds = append(b.visibilityBounds, (delay+uncertainty)*1000)
	}
	b.visibilityClips = append(b.visibilityClips, rankedClip{rank: -(delay + uncertainty), clip: model.ReviewClip{
		ID: fmt.Sprintf("%s:%s:review-%s:%d", input.Demo.ID, input.Player.ID, prefix, shot.Tick), Signal: "reaction", Provenance: provenance,
		Round: shot.Round, Tick: shot.Tick, EndTick: shot.Tick, Time: shot.Time, Title: title, Description: description,
		Measurements: []model.Measurement{measure("Observed delay", delay*1000, "ms"), measure("Upper delay bound", (delay+uncertainty)*1000, "ms"), measure("Sampling uncertainty", uncertainty*1000, "ms")}, Limitations: limitations}})
}

func percentile(values []float64, q float64) float64 {
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	position := q * float64(len(copyValues)-1)
	left := int(math.Floor(position))
	right := int(math.Ceil(position))
	return copyValues[left] + (copyValues[right]-copyValues[left])*(position-float64(left))
}
func (b *reviewBuilder) finish(input Input, result Result, ballistics ballisticReviewResult) *model.ReviewSummary {
	review := &model.ReviewSummary{TotalShots: b.total, SampledShots: b.sampled, EligibleAimShots: b.eligible, CoveredRounds: len(b.rounds), Metrics: []model.ReviewMetric{}, Exclusions: []model.ReviewExclusion{}, Clips: []model.ReviewClip{}}
	if len(b.cadence) > 0 {
		ms := math.Round(percentile(b.cadence, .5)*100) / 100
		review.MedianSampleMS = &ms
	}
	addMetric := func(signal, provenance, label string, values []float64, q float64, unit string) {
		if len(values) > 0 {
			review.Metrics = append(review.Metrics, model.ReviewMetric{Signal: signal, Provenance: provenance, Label: label, Value: measure("", percentile(values, q), unit).Value, Unit: unit, Samples: len(values)})
		}
	}
	addMetric("aim-snap", "Sampled server view angles", "Median aim turn before a shot", b.turns, .5, "°")
	addMetric("aim-snap", "Sampled server view angles", "95th percentile aim turn", b.turns, .95, "°")
	addMetric("aim-snap", "Sampled server view angles", "95th percentile sampled aim speed", b.speeds, .95, "°/s")
	addMetric("aim-snap", "Nearest live opponent eye-point approximation", "Median opponent eye-point aim error", b.targetErrors, .5, "°")
	addMetric("acquisition", "Synchronized opponent eye-point acquisition", "Median alignment-to-shot delay", b.acquisition, .5, "ms")
	addMetric("acquisition", "Synchronized opponent eye-point acquisition", "Median alignment upper delay bound", b.acquisitionBounds, .5, "ms")
	addMetric("reaction", "Verified static map geometry; eye-point approximation", "Median estimated visibility-to-shot delay", b.visibility, .5, "ms")
	addMetric("reaction", "Verified static map geometry; eye-point approximation", "Median visibility upper delay bound", b.visibilityBounds, .5, "ms")
	addMetric("reaction", "Observer-specific network spotted state; not visual reaction", "Median spotted-to-shot proxy delay", b.spotted, .5, "ms")
	addMetric("reaction", "Observer-specific network spotted state; not visual reaction", "Median spotted proxy upper delay bound", b.spottedBounds, .5, "ms")
	review.Metrics = append(review.Metrics, ballistics.metrics...)
	for reason, count := range b.excluded {
		review.Exclusions = append(review.Exclusions, model.ReviewExclusion{Reason: reason, Count: count})
	}
	sort.Slice(review.Exclusions, func(i, j int) bool {
		if review.Exclusions[i].Count == review.Exclusions[j].Count {
			return review.Exclusions[i].Reason < review.Exclusions[j].Reason
		}
		return review.Exclusions[i].Count > review.Exclusions[j].Count
	})
	addClips := func(items []rankedClip, limit int) {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].rank == items[j].rank {
				return items[i].clip.Tick < items[j].clip.Tick
			}
			return items[i].rank > items[j].rank
		})
		added := 0
		for _, item := range items {
			if len(review.Clips) >= 16 {
				break
			}
			nearby := false
			for _, existing := range review.Clips {
				if existing.Round == item.clip.Round && math.Abs(existing.Time-item.clip.Time) < 2 {
					nearby = true
					break
				}
			}
			if nearby {
				continue
			}
			review.Clips = append(review.Clips, item.clip)
			added++
			if added >= limit {
				break
			}
		}
	}
	addClips(b.visibilityClips, 3)
	ballisticClips := make([]rankedClip, 0, len(ballistics.clips))
	for _, clip := range ballistics.clips {
		ballisticClips = append(ballisticClips, rankedClip{clip: clip})
	}
	addClips(ballisticClips, 4)
	addClips(b.timingClips, 3)
	addClips(b.aimClips, 6)
	kills := []rankedClip{}
	for _, e := range input.Events {
		if e.PlayerID != input.Player.ID || e.Kind != "kill" || e.Round <= 0 {
			continue
		}
		title := "Kill with " + e.Weapon
		if e.Headshot {
			title = "Headshot with " + e.Weapon
		}
		kills = append(kills, rankedClip{clip: model.ReviewClip{ID: input.Demo.ID + ":" + input.Player.ID + ":review-event:" + e.ID, Round: e.Round, Tick: e.Tick, EndTick: e.Tick, Time: e.Time, Title: title, Description: e.Text, Measurements: []model.Measurement{}, Limitations: []string{"Event bookmark for manual review; kills and headshots are not evidence of cheating."}}})
	}
	addClips(kills, 4)
	sort.Slice(review.Clips, func(i, j int) bool { return review.Clips[i].Tick < review.Clips[j].Tick })
	status := "No repeated rule triggers."
	if len(result.Findings) > 0 {
		status = fmt.Sprintf("%d repeated-rule measurements need review.", len(result.Findings))
	}
	unassessed := []string{}
	for _, c := range result.Capabilities {
		if c.Status != "available" {
			label := c.Signal
			if c.Signal == "reaction" {
				label = "visual reaction"
			}
			if c.Signal == "shot-direction" {
				label = "bullet direction"
			}
			if c.Signal == "acquisition" {
				label = "crosshair timing"
			}
			if c.Signal == "aim-snap" {
				label = "target-aligned aim"
			}
			if c.Signal == "recoil" {
				label = "recoil compensation"
			}
			unassessed = append(unassessed, label)
		}
	}
	review.Summary = fmt.Sprintf("Review complete: %d shots across %d rounds; %d have precise player samples. %s", b.total, len(b.rounds), b.sampled, status)
	if len(unassessed) > 0 {
		review.Summary += " Unassessed or limited: " + strings.Join(unassessed, ", ") + "."
	}
	return review
}
