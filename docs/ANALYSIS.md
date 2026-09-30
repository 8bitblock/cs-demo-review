# Evidence rules and limitations

The beta implements deterministic review heuristics, not a trained cheating classifier. A recording cannot establish intent, software running on the player's computer, or exactly what the player saw. Thresholds below are **experimental engineering defaults**, not validated boundaries of human ability. There is no probability-of-cheating score.

## Inputs and time units

Rules version: `beta-rules-1.2.0`; parser adapter: `4`. Every stored demo records these versions; findings retain the recording identity, player, round, tick, event time, measurements, alternatives, limitations and shot provenance. Opening an existing library refreshes older parser caches from the original DEM and re-runs analysis while preserving recording identity, map assignments and notes. If the original file is unavailable, the usable cached review remains available with a warning; missing telemetry is not invented.

The internal protocol uses seconds for event/sample `time` and shot `timingPrecision`. Display measurements explicitly convert to milliseconds. DEM files contain recorded network state and game events; a 64 Hz server tick rate does not guarantee every field or every player's state was recorded 64 times per second. The parser retains each available recorded game-tick sample. Multiple frames at one game tick update that tick's sample. Recorded cadence is measured and displayed; missing ticks are not recreated for analysis.

Contiguous analysis samples may be up to 65 ms apart, which accommodates the observed approximately 25.6 fps CS:GO fixture. For acquisition/reaction timing, uncertainty is the larger of shot precision and measured sample cadence, plus the interval spanning the threshold crossing. The **upper bound**, rather than a rounded point estimate, must pass the threshold.

Targets require synchronized opponent samples, valid eye positions and a live player on an opposing playing team. Missing target context, missing fields or gaps limit the applicable detector; other recorded measurements remain available. Names, kills, rank, accuracy percentages and the number of headshots do not drive the assessment.

## Detectors

| Signal | Eligible observations and experimental threshold | Interpretation |
| --- | --- | --- |
| Aim snap | First shots after at least 400 ms without firing; turn at least 18°, speed at least 1,200°/s, final opponent-eye error at most 0.3°, shot within 20 ms of the final sample | Abrupt target alignment deserving review; skilled flicks can match it |
| Acquisition | Known crossing from outside 0.8° to within the acquisition cone; final alignment within 0.45°; upper delay bound below 85 ms | Crosshair acquisition to firing, **not visual reaction time** |
| Reaction | Acquisition transition plus a known visibility transition using matching complete static geometry, unscoped nominal field of view, valid eye points; upper delay bound below 100 ms | Approximate geometry-based visibility to firing; target body parts may have been visible earlier |
| Recoil | At least seven same-weapon shots in a continuous stationary spray, increasing native recoil index, at least 1° of native recoil motion, mean cancellation residual below 0.025° and maximum below 0.06° | Near-identical cancellation across repeated sprays; does not flag ordinary imperfect spray control |
| Shot direction | Explicitly validated native bullet direction, view angles, aim punch and spread/inaccuracy; discrepancy exceeds the conservative spread cone by more than 5° | A native field discrepancy; impact endpoints and penetration segments are never interpreted as bullet trajectory changes |

Recoil calculations use an explicit, source-specific `aimPunchScale` carried with each shot. The supported CS2 native `CMsgTEFireBullets.Extra.aim_punch` is already the effective angular offset, so the parser records scale **1**. This interpretation was checked on the build-10924 Inferno and Ancient fixtures; it is not a validation of every game build. CS:GO sampled entity aim-punch uses the nominal entity multiplier **2**, labelled as an assumption. Missing or invalid scales do not silently fall back to a value. CS2's problematic continuously sampled aim-punch data is **never consumed**. Recoil findings require explicit `shot-native` provenance; shot-direction findings additionally require `validated-shot-direction`. CS2 fire-packet fields are associated with a unique same-tick weapon-fire event using the shooter pawn handle. Their firing orientation is useful for review, but is not certified as the final spread-adjusted bullet direction. The current parser therefore does not assert `validated-shot-direction`. Native shot fields are optional and their absence does not count as clean evidence.

Timing/aim candidates exclude player flash, ambiguous shot association, shotgun/pellet weapons, unknown weapons and ongoing sprays. Recent spawn/connect/team transitions, dead/spectator transitions and discontinuous position or eye movements also suppress candidates. Smoke, doors and scope state no longer discard unrelated recorded aim-angle observations. Geometry-based visibility remains unscoped and excludes smoke within a conservative 256-unit envelope around the observer-to-target ray for up to 22 seconds (or until a matching expiry event), and nearby door/breakable events within one second. Missing event positions remain conservative visibility exclusions. This envelope is not a simulation of smoke volume. Held crosshairs without a known acquisition transition cannot produce reaction findings.

## Measured support for visibility, recoil and shot direction

- **Visibility reaction:** verified geometry can produce hidden-to-visible timing measurements even when the stricter crosshair acquisition requirement for a finding is not met. The target must be near the shot's aim direction (within 5° of its eye point), and timing bounds and visibility exclusions still apply. Recorded observer-specific spotted-by transitions provide a separate **spotted-to-shot proxy** when available. A known false-to-true transition is required. Spotting can lag or persist and does not establish client screen visibility; proxy observations never generate findings or satisfy detector coverage.
- **Recoil compensation:** same-weapon bursts with at least three shots, no more than 200 ms between shots and measurable aim-punch movement show the view movement projected against recoil and the remaining angular residual. These descriptive observations do not require stationarity. A value of 100% means average cancellation along the recoil component, negative values follow it, and values above 100% overcompensate. This is not hit accuracy. Sources distinguish CS2 shot-native effective punch (scale 1) from CS:GO network-sampled entity punch captured at weapon fire (assumed scale 2); a burst must keep a consistent source and scale. Ordinary tracking and spray control affect the measurements. Only the stricter stationary seven-shot detector described above can produce a finding.
- **Shot direction:** native firing orientation is compared with recorded view angles when present. Otherwise, a single unambiguously associated impact can provide an origin-to-impact ray. Multiple-impact and too-short rays are excluded. The report keeps firing-angle and impact-ray distributions separate. Recoil, spread and event timing can explain offsets; neither source establishes a bent bullet or silent aim. Final-direction anomaly findings retain the independent validated-direction and spread requirements.

Every measurement and selected review moment carries its signal, source and sample count or timing context. These observations remain visible even when the associated detector cannot run.

## Repetition and assessments

Comparable cohorts use weapon, stationary/moving state, stance, scope state and first-shot/spray mode. Candidates must repeat in a cohort across at least three rounds: four independent episodes for timing/aim/direction or three sprays for recoil. Findings retain individual timestamps instead of replacing the evidence with a single opaque score.

Adjacent shots in the same round separated by no more than 1.5 seconds belong to one episode. Aim snap, acquisition and reaction are one correlated signal family. Multiple measurements of an episode do not create independent evidence.

- **Insufficient data** applies when there are too few eligible observations or covered rounds for an assessment. Measured context may still be available.
- **Reviewed with limits** requires no repeated findings, at least three covered rounds and at least one available detector with 20 eligible observations. Other families may remain unassessed; partial recordings can qualify. This status does not claim low concern or legitimate play.
- **Suspicious** requires at least four independent reported episodes across at least three rounds.
- **Highly suspicious** requires at least eight reported episodes across five rounds and at least two independent signal families, each supported by four episodes.
- **Low concern** requires a complete recording, at least eight covered rounds, no findings and at least 40 eligible observations for **each** of the five signals. It is concern within measured coverage, not proof of legitimate play. Most current beta recordings cannot meet this strict complete-coverage requirement.

Capability states are separate from findings and the overall assessment:

| Protocol state | Display | Meaning |
| --- | --- | --- |
| `available` | Assessed | At least 20 detector-eligible observations |
| `limited` | Limited sample | Between 1 and 19 detector-eligible observations |
| `measured` | Measured | Descriptive or proxy observations exist, but the detector cannot assess them |
| `insufficient` | Missing observations | No eligible observations were found for the supported analysis |
| `unsupported` | Unavailable | Required geometry or validated native fields are absent |

Cards and exports report **measured observations** separately from **detector-eligible observations**. Recoil observations count bursts/sprays; other signals count applicable shot observations. Neither count is a count of suspicious findings. Availability does not imply scientifically established sensitivity or specificity, and repeated findings can exist with fewer than 20 eligible observations.

## Useful review when an overall assessment is unavailable

Each player receives a completed-review summary independently of the verdict. It reports total shots, shots with precise live player samples, rounds containing shots, timing-eligible aim observations, measured sample cadence and an explicit breakdown of excluded shots. Exclusion counts are mutually exclusive (the first failing aim gate) and, together with eligible aim observations, account for every recorded shot. Per-signal gates remain separate, so that breakdown is not a claim that the same shot was eligible for every detector.

Neutral statistics include the median and 95th percentile view-angle change before sampled shots, 95th percentile sampled angular speed, and measured crosshair-acquisition timing where an observed transition exists. Each statistic carries its observation count. Acquisition statistics cover eligible first-shot transitions in the preceding 250 ms, and are accompanied by upper timing bounds. They are not visual reaction times or measurements of mouse input.

Neutral review moments (the `review.clips` data field) include visibility/spotting transitions, recoil bursts, shot directions, measured alignment transitions, larger sampled pre-shot aim changes and kill events. The selection is capped at 16 moments per player and deduplicated within two seconds. These are timeline links, distinct from videos in the **Recorded clips** tab. Their sources and limitations remain visible when opened. Single rapid alignments, proxy measurements and ordinary kills remain navigable without being promoted into suspicious findings. These bookmarks never feed verdict aggregation, evidence counts or independent-family counts.

This separates **review completed** from **coverage of every detector**. The current parser's unsupported final bullet-direction semantics and uncertified geometry still prevent broad low-concern claims. Numerical anomaly and repetition thresholds are unchanged; exclusions now apply to the signals they actually limit.

## Validation

Unit tests cover angle wrapping, long gaps, teleport/respawn/spectator exclusions, low-cadence uncertainty, held angles, flash/smoke/door exclusions, isolated skilled flicks, repeated synthetic alignment, native recoil versus ordinary imperfect compensation, missing recoil fields, impact endpoints, correlated-family deduplication, partial recordings and unknown map versions. Synthetic positives test the implemented rules only. They do not demonstrate real-world cheating detection accuracy.

Parser fixture validation and real local game extraction are separate integration checks. Independently labelled legitimate/suspicious demo evaluation and native-playback alignment checks remain necessary before presenting accuracy claims. The app makes none.

## Session additions and fixes

- Added five explainable, independently gated evidence detectors and conservative four-state assessments.
- Added provenance, timing bounds, contextual exclusions, cohort repetition and episode/family deduplication.
- Fixed timing units to protocol seconds and used measured recording cadence for CS:GO uncertainty.
- Added test cases for legitimate counterexamples, missing fields, discontinuities, repeated synthetic anomalies and incomplete coverage.
- Fixed spectator-team handling, smoke-expiry treatment and repeated-encounter grouping across arbitrary time boundaries.
- Added completed-review summaries, measured distributions, explicit exclusion counts and timestamped neutral review clips even when the overall assessment remains insufficient.
- Kept isolated flicks and kill bookmarks separate from suspicious findings; added tests that observations cannot change verdicts or invent missing measurements.
- Added read-only validation of existing real libraries and automatic cached-review refresh coverage preserving notes.
- Added geometry-based visibility measurements, observer-specific spotting proxies, burst recoil compensation and source-labelled shot direction observations.
- Added `Reviewed with limits`, limited-sample and measured-only capability states without treating missing detector coverage as legitimate play.
- Separated measured observations from detector-eligible counts in the inspector and HTML/JSON reports, with source details and default-visible statistics.
- Preserved small angular residuals to three decimal places and added report/presentation checks for provenance, missing values and neutral observations.
- Fixed CS2 native-fire packet association to use pawn handles, retained every available tick sample and added adapter-4 cache reparsing with identity, notes and map preservation.
- Corrected recoil calculations to retain source-specific scales: native CS2 effective punch uses 1, while sampled CS:GO entity punch uses the labelled nominal multiplier 2.
