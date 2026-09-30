# Changes

## Session additions and fixes — demo telemetry review

- Rebuilt the Windows x64 portable ZIP and installer with the current telemetry fixes, verified packaged worker hashes and app launch, and refreshed release notes and SHA-256 checksums.
- Fixed CS2 fire-packet association using pawn handles, recovering native recoil and firing angles that were previously discarded.
- Added visibility-to-shot measurements from verified geometry and separate observer-specific spotting proxies from both demo formats.
- Added measured recoil compensation, residuals, firing-angle offsets and single-impact direction references with source-specific scaling and navigable review moments.
- Fixed broad smoke/scope/door exclusions suppressing ordinary aim coverage; preserved visibility safeguards and repeated-finding thresholds.
- Added measured and limited coverage states, visible per-signal metrics and sources, and “Reviewed with limits” for useful incomplete assessments.
- Added automatic parser-cache upgrades preserving demo IDs, maps and notes, missing-source recovery messages, and incomplete-recording coverage during staged imports.
- Added parser, analysis, migration, report and UI regression coverage; documented the distinction between recorded telemetry, proxies and suspicious findings.

## 0.1.0-beta.2

- Fixed recorded WebM duration metadata so saved clips expose a finite duration and usable video seeking without re-encoding their frames.
- Fixed recorded-video range responses so the in-app scrubber can seek to the requested position; added closed, open-ended, suffix and invalid-range regression tests.
- Validated the new Inferno and Ancient demos, their extracted radars, and actual game-window recording, playback, seeking, reload and video import.
- Added safe native-playback helpers that detect an existing game session, preserve it, and provide separate load and seek instructions.
- Added read-only Windows game-window identification using exact executable paths, process IDs, and native handles; unrelated windows cannot qualify by title alone.
- Added five playback/capture-boundary tests and documented native viewing setup, synchronization limits, and plugin-free behavior.
- Enlarged workspace typography, controls, evidence copy, and map labels, with responsive panel sizing and an expandable replay view.
- Added automatic cached-map loading and installed-map extraction, persistent progress, visible failures, and retry/settings actions in the replay pane.
- Added map drag-to-pan, wheel zoom, readable player labels, transparent-radar fitting, and spawn/teleport-safe display interpolation.
- Added measured shot counts, sampling coverage, exclusions, neutral review moments, and refresh feedback alongside conservative assessments.
- Fixed same-recording analysis refreshes, stale replay windows after seeking, and player-inspector navigation after changing players.
- Added a Recorded clips view hook and UI regression checks for automatic maps, error recovery, review-moment seeking, typography, and compact layouts.
- Added completed-review summaries, measured aim distributions, contextual exclusion counts, and neutral timestamps without changing verdict thresholds.
- Added automatic analysis upgrades for existing libraries while preserving notes and the selected recording.
- Added local game-window recording to WebM, imported MP4/WebM clips, persistent clip playback, and explicitly manual review-time references.
- Hardened clip recording, cancellation, shutdown, corrupt metadata, and size/path limits; imported originals remain untouched.
- Added review measurements and neutral moments to exported reports, with injection and assessment-separation tests.

- Added real-worker UI validation and screenshots for sampled shot counts, completed analysis versions, neutral-moment seeking, radar pixel access, and compact layouts; added a regression check preserving the current demo during background analysis refreshes.
- Fixed map-image CORS for content fitting and kept the selected player label above nearby markers.
- Fixed desktop video permission handling while keeping camera and microphone access disabled.

## 0.1.0-beta.1

- Added the Windows desktop shell, isolated renderer, validated local IPC, and background worker transport.
- Added native multi-file imports, local progress events, cancellation plumbing, and duplicate-aware library integration.
- Added Steam installation discovery, explicit game/extractor settings, safe playback commands, and native game launch.
- Added standalone HTML and JSON evidence reports with escaped player names and review notes.
- Added local diagnostics export, packaged worker resources, and Windows installer/portable build configuration.
- Added tests for report injection, private source-path omission, bounded replay queries, review-note validation, and safe game-console paths.
- Updated Electron and testing dependencies to patched releases found during dependency verification.
- Added real CS2 and CS:GO parser checks, upstream CS:GO golden event-count comparisons, cancellation/cache cleanup checks, and Electron interaction tests.
- Added map extraction, playback, bookmark/note, report export, settings, deletion-cancellation, and compact-layout smoke coverage.
- Added the app icon, bundled third-party license texts, and repeatable checksummed tool provisioning.
- Fixed worker stream error handling and graceful shutdown so background imports and extraction stop with the desktop app.
- Fixed bundled extractor discovery after moving or reinstalling the app.
- Fixed import-completion feedback loops and added regression coverage for native duplicate imports and broken worker pipes.
- Added live aim-trace inspection, Steam map extraction verification, and real CS:GO golden-reference event checks.
- Built unsigned Windows installer and portable ZIP with application icon/metadata, bundled runtimes, and SHA-256 checksums.
- Verified shipped parser binaries against fresh CS2/CS:GO imports and the packaged desktop app against real map extraction, replay controls, aim charts, notes, and exports.

## Session additions and fixes — GitHub repository setup

- Prepared the initial source snapshot for the private `8bitblock/cs-demo-review` GitHub repository, including application code, workers, documentation, tests, and packaging assets.
- Verified TypeScript checks and all 52 application tests before the initial push.
