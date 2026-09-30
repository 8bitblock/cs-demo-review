# CS Demo Review 0.1.0-beta.2

- Fixed CS2 firing-packet association using shooter pawn handles, recovering native recoil and firing angles; corrected source-specific recoil scaling.
- Added geometry-based visibility-to-shot timing, separate observer-specific spotted-to-shot proxies, recoil-compensation measurements, and source-labelled firing-angle and single-impact direction references.
- Added **Reviewed with limits**, measured and limited-sample coverage states, per-signal measurements and sources, and clickable telemetry review moments.
- Fixed broad smoke, scope and door exclusions suppressing unrelated aim observations while preserving visibility safeguards and repeated-finding thresholds.
- Added automatic parser-cache upgrades that preserve recording IDs, maps and notes, with missing-source recovery messages and partial-recording coverage.
- Increased text, controls, map labels and evidence panel sizes; added an expanded replay view.
- Maps now load or extract automatically, with persistent progress, actionable failures and retry. Added map dragging, wheel zoom and improved fitting.
- Added completed-review summaries, measured aim statistics, shot counts, coverage exclusions and clickable review moments. Existing libraries refresh automatically without reimporting.
- Added **Recorded clips**: capture 10–120 seconds of real game video to local WebM, import MP4/WebM footage, and replay saved clips inside the app. Clips use manual review references; load, seek and select the player in the game before recording. Video capture does not save audio.
- Fixed background analysis stealing the selected recording, stale replay displays, silent map failures and ignored native launch arguments when the game is already running.
- Added safe game-window identification, recording lifecycle cleanup, bounded video storage, and review summaries in HTML/JSON reports.

No numerical anomaly or repetition thresholds were relaxed to manufacture results. Measured telemetry remains separate from suspicious findings, and missing detector coverage does not establish legitimate play. Recorded video requires the installed game or an existing video file; this is not automatic standalone 3D reconstruction.

## Included from beta.1

- Added local CS2 and CS:GO `.dem` import, queued parsing, cancellation, duplicate detection, and a persistent SQLite library.
- Added a dark analyst workspace with resizable panels, radar/grid replay, player following, rounds, playback speeds, shot/event overlays, and original-sample aim charts.
- Added conservative evidence rules, explicit per-signal coverage, bookmarks, review notes, and HTML/JSON reports.
- Added local map extraction and historical map-pack imports, Steam detection, game playback commands, and local diagnostics.
- Added Windows x64 installer and portable ZIP, including parser binaries and Source2Viewer; no developer runtime is required to use them.
- Fixed timing-unit mismatches, old-cache upgrades, moved-file recovery, import-completion loops, per-file queue errors, and background-process cleanup on exit.

This is an unsigned experimental beta. Assessment accuracy has not been validated against independently labelled cheating recordings. Geometry-based visibility estimates require matching, complete, verified geometry; locally extracted map assets may support replay while remaining unsuitable for visibility-based assessments. Spotted-to-shot timing is a separate proxy and never establishes screen visibility or contributes suspicious findings. Native CS2 shot recoil supports compensation review, while unreliable continuously sampled CS2 aim-punch remains disabled. Native firing angles and single-impact rays support direction measurements, but neither is certified as the final spread-adjusted bullet trajectory; final-direction anomaly findings remain unavailable without validated native direction and spread data.

CS2 native playback uses the installed game. CS:GO native playback requires a separate compatible legacy installation. This build does not automatically download old game versions or publicise review reports.

Detailed verification results and remaining validation limits are in `docs/VALIDATION.md`.
