# Frontend additions and fixes

- Added visible measurements grouped by visibility reaction, recoil compensation, shot direction and aim, with observation counts and recorded sources.
- Added measured/limited capability states and “Reviewed with limits”; separated neutral telemetry counts from detector eligibility and findings.
- Added review-moment provenance, finer angular-value formatting, recoil/impact availability in aim traces, and report parity.
- Fixed misleading rounds and empty-review labels and clarified network spotting as a proxy for visibility.

- Added the React and TypeScript desktop analysis workspace with a dark local-library interface, useful empty state, import queues, cancellation, and visible recovery messages.
- Added original-tick replay controls, on-demand replay windows, Canvas radar and coordinate rendering, display-only position interpolation, team indicators, map floors, shot and event overlays, player following, round seeking, and keyboard controls.
- Added player selection, conservative assessment and capability coverage displays, evidence-linked timeline markers, recorded yaw-speed charts, notes, bookmarks, exports, native playback actions, map asset controls, and settings and diagnostics dialogs.
- Added resizable panels, accessible input labels, focus indicators, dialog keyboard handling, responsive layout adjustments, and loading and error feedback.
- Connected all imports, parsing results, findings, maps, exports, notes, settings, and game actions through the isolated typed desktop API; no demonstration findings or mock recordings are inserted into production state.
- Preserved the selected recording when native removal confirmation is cancelled; paused replay while dialogs are open; stabilized keyboard focus trapping; implemented the search shortcut and automatic scrolling to selected evidence details.
- Added Source 2 Viewer and demoinfocs attribution links in settings; verified TypeScript compilation, production renderer bundling, and the screenshot of the real desktop recording workspace.
- Clarified game and extraction-tool settings to require their executable paths; verified settings save includes only the four writable path fields and excludes the read-only library directory.
- Added an independent Aim trace tab with synchronised recorded yaw/pitch speeds, shot markers, a live playhead, click and keyboard seeking, measured sample intervals, and explicit missing-data coverage even when no findings exist.
- Broke aim traces at sampling gaps, invalid values, deaths, and teleports; removed fabricated zero values across gaps from the selected-finding chart and kept interpolated map positions out of measured charts.
- Deduplicated import completion events by job identifier so repeated progress snapshots cannot trigger library-refresh loops or repeatedly reset the selected recording.
