# Beta verification

## Current telemetry build and packages

Verified on Windows x64 on 25 September 2026 with rules `beta-rules-1.2.0` and parser adapter `4`. The worker, renderer, Windows x64 portable ZIP and NSIS installer were rebuilt. Packaged app launch passed without developer runtimes on its PATH or renderer errors; all three worker binaries in the ZIP match the current build by SHA-256. Release checksums were refreshed. No installer installation was performed in this check.

- All 52 TypeScript tests, Go package suites, TypeScript checks and the production build passed.
- Real parser/import checks passed for Inferno, Ancient and the upstream legacy Cache fixture, including duplicate imports, malformed/truncated files, note persistence and the legacy golden event counts.
- Automatic upgrades from an isolated copy of the adapter-3 library recovered native telemetry while keeping the two CS2 recording IDs. Reanalysis checks preserved recording hashes, notes and selected maps.
- `npm run test:telemetry` verified real stored outputs: 9,797 shots, 30 players, and 28 **Reviewed with limits** results. Two players still lack sufficient eligible observations. No repeated suspicious findings were produced; these recordings are not labelled ground truth.

| Map | Native recoil/firing packets recovered | Measured recoil bursts | Spotted-to-shot proxies | Measured direction references |
| --- | ---: | ---: | ---: | ---: |
| Inferno | 2,113 | 228 | 108 | 2,057 |
| Ancient | 1,908 | 224 | 116 | 1,751 |
| Legacy Cache | 0; 5,142 sampled-recoil shots retained | 316 | 34 | 0 |

Counts above are observations, not suspicious incidents. The legacy fixture contains no supported native/impact direction references; missing directions remain unavailable. Geometry-based visual reaction remains separately gated on verified map geometry.

- In build 10924 packets with at least 0.25° of punch, the median native/view residual was 0.016° with native scale 1 versus 2.916° with scale 2 on Inferno (1,228 packets), and 0.023° versus 2.940° on Ancient (1,093 packets). This verifies the source-specific scaling correction on those recordings.
- Regression tests cover missing native packets inside sprays, source/scale changes, false zero rounding, player/weapon/round boundaries, death/flash/spectator transitions, teleports, sample gaps, spatial smoke exclusions, spotting uncertainty, cache migrations and staged partial imports.
- Real Electron UI checks passed replay-moment seeking, radar access, readable counts and compact layout without renderer errors. A second check compared the displayed visibility, recoil and direction measurements and capability counts directly to the worker output. Screenshots were visually inspected at normal and 1100×760 sizes.
- The isolated UI fixture required copying the actual extracted map assets and relocating their paths into the copied library; radar and geometry bytes were checked unchanged. No mock telemetry or IPC results were inserted.
- Reproducible fixture checks are in `scripts/test-telemetry.mjs`; local results are in `artifacts/telemetry-review-results.json`. Original demos and the primary library were not overwritten by validation.

## Earlier packaged beta.2 verification

The following checks refer to the earlier Windows x64 package verified on 25 September 2026: build `0.1.0-beta.2`, rules `beta-rules-1.1.0`, parser adapter `3`.

## Real recordings

| Recording | Format | Map | Duration | Recorded rounds | Ticks | Events |
| --- | --- | --- | --- | --- | --- | --- |
| Local Steam replay, 165 MB | CS2 | Dust II | 24:34 | 17 | 94,385 | 3,465 |
| Local Steam replay, 298 MB | CS2 | Mirage | 37:02 | 23 | 142,226 | 3,453 |
| New local Steam replay, 196 MB | CS2 | Inferno | 29:09 | 19 | 111,987 | 3,265 |
| New local Steam replay, 159 MB | CS2 | Ancient | 24:17 | 17 | 93,312 | 3,297 |
| Upstream default fixture, 69 MB | CS:GO | Cache | 55:32 | 32 | 426,607 | 7,022 |

- All five parsed through real isolated parser executables and SQLite storage. Fresh beta.2 imports of the new Inferno and Ancient recordings took 29 and 24 seconds respectively on this machine; this is not a hardware-independent performance promise.
- The CS:GO fixture matches the upstream v3.3.0 golden reference: **220 kills, 811 damage events, 32 rounds**. Source: `markus-wa/demoinfocs-golang` v3.3.0 `test/default.golden`, with `cs-demos-2` submodule commit `e97009976cfcaee540870293c9b7c2825a2b8557`.
- Both formats produced indexed replay windows and persistent review notes. Reimports identified duplicate content, and moved-file/old-adapter regressions preserve review identity and notes.
- Warmup/unstarted-match shots were excluded from assessments while retained in replay data. The tested CS:GO map CRC is `crc32:71743acf`.
- These recordings produced **Insufficient data** assessments. That result does not establish whether any player cheated. None is treated as a confirmed cheating/legitimate training label.
- The four local CS2 recordings contain 40 players and 9,559 recorded shots. Updated analysis produced 159 measured metric summaries and 217 neutral review moments. No repeated-rule findings were detected. Review moments are navigational bookmarks, not additional cheating findings or recorded videos.
- Versioned background analysis upgrades existing libraries without reparsing or deleting notes. Completed background upgrades preserve the user's selected recording.

## Automated and desktop checks

- Forty-eight TypeScript tests pass, including output escaping, neutral-report separation, native window identity, single-use capture grants, recording lifecycle races, corrupt clip metadata, path and size bounds, WebM duration finalization, video byte ranges, replay interpolation, worker-pipe failure and shutdown.
- Go parser, storage, service, analysis and map suites pass. They cover synthetic positive/counterexamples, missing data, map/version checks, visibility geometry, recoil gates, migrations, atomic cache upgrades and historical-pack selection.
- Real import tests passed duplicate handling, note save/delete, malformed signature rejection, truncated stream rejection, cancellation, responsive library reads, partial-cache cleanup and original-file preservation.
- Electron interaction tests passed native duplicate import without completion loops, playback, scrubbing, aim-chart seeking, notes/bookmarks, settings save, reports, cancelled deletion and compact layout.
- UI regressions exercise automatic map loading, visible extraction errors and retry, measured results, neutral-moment seeking, larger typography, map pan/zoom and replay expansion. Mocked UI-contract screenshots are kept separate from actual recording screenshots.
- Actual game-window recording tests captured a nonblank 1920×1080 picture, saved a 10-second WebM, played and sought it, reopened it after reload, and imported a copy without changing source hashes. The existing game was showing its inventory; the test did not load or seek a demo and does not establish synchronization with the selected recording.
- The final beta.2 packaged executable passed the same complete clip test with developer tools removed from its child-process PATH. The saved 8.31 MB video reported a finite **10.0144-second** duration and a full seekable range from 0 to 10.0144 seconds; seeking to three seconds, reloading, and importing a copy all passed. Source hashes stayed unchanged and there were no renderer errors. This verifies the shipped WebM finalization and byte-range response fixes. The existing game session remained untouched; its relationship to the selected demo/player/timestamp was not verified.
- Closing the desktop app during a live import left no worker/parser process running.
- Local Source2Viewer extraction produced working Dust II, Mirage, Ancient and Inferno radars. The new Ancient recording passed actual UI checks for radar rendering, player positions, review-moment seeking and larger text. Radar transform and complete-geometry/version gating were checked. Local partial/unverified geometry remains ineligible for reaction findings.
- The packaged executable launches with Node/Go developer tools removed from its child-process PATH. Fresh Dust II and CS:GO imports passed using the shipped parser executables. The packaged desktop passed Dust II extraction, replay controls, aim-chart seeking, notes/bookmarks, settings, HTML export and compact layout with no renderer errors. These checks used the executables shipped under `resources/`, not source builds.
- `npm audit` reports no known dependency vulnerabilities in the resolved dependency tree at verification time.

## Explicit remaining limits

- No clean Windows VM or separate test PC was available. This is a packaged-runtime smoke test on the development machine, not certification of every Windows configuration or a clean-machine installer run.
- Installer and portable artifacts are unsigned. Public signing and distribution credentials were not supplied.
- Native game commands were generated and validated, but frame-by-frame comparison inside compatible CS2/legacy CS:GO clients was not completed.
- Cheating detection accuracy, precision and false-positive rate are unvalidated. Independent labelled recordings are required before making numerical accuracy claims.
- Full historic/dynamic map reconstruction and validated native bullet-direction semantics are unavailable in this beta. Their dependent assessments remain disabled; the UI explains why.
- Recorded clips require a game-rendered source or an imported video. Game loading, seeking and player selection are manual; the app does not automatically render selected demo ticks into synchronized video. In-app capture saves video without audio.

Detailed output is saved locally under `.tmp/release-validation/`, `.tmp/packaged-validation/`, and `artifacts/`. Demo files and private caches are excluded from distribution and Git.
