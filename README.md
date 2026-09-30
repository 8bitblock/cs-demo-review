# CS Demo Review

A local Windows desktop workspace for CS2 and CS:GO demo playback and evidence review. Open `.dem` files, replay rounds on a 2D map, inspect measured aim/shot behaviour, and export review notes. Assessments are experimental review aids, not proof of cheating or a guarantee of legitimate play.

## Run from source

Requirements: Windows x64, Node.js 22.12+ and Go 1.25+. No game installation is needed to parse a demo. Map extraction and native playback use your installed game.

```powershell
npm ci
npm run build
npm start
```

For interface development, build the workers once with `npm run build:worker`, then run `npm run dev`. If Go is not on PATH, set `GO_BINARY` to its executable. The build also detects `.tools/go/bin/go.exe`.

## Review a match

1. Choose **Import demos** or drop uncompressed `.dem` files into the app. Parsing runs locally in separate processes.
2. Select a recording and a round. Use the timeline, playback controls, floor picker, and player list to investigate moments.
3. Inspect each player's available signals and evidence coverage before interpreting an assessment. Open a finding to see its measurements and alternatives.
4. Use Settings to confirm game paths. Extract assets from a local installation or import a versioned map pack. The grid view remains usable without assets; visibility-based evidence requires a verified match.
5. Save notes or bookmarks and export an HTML or JSON report. **Open in game** launches the recording and copies the seek command; paste it into the game console after the demo loads.
6. Open **Recorded clips** to import MP4/WebM footage or record a 10–120 second clip from the game. Load and seek the demo with the displayed console commands first. Saved video plays inside the app. Clip timestamps are manual reference points; game seeking is not automatic. See [recording instructions](docs/NATIVE-PLAYBACK.md).

The map loads automatically from a cached radar or extracts from the configured game installation, with progress and retry controls. Player analysis includes visibility/spotting timing, recoil compensation, recorded shot direction, aim statistics, exclusions and clickable review moments. **Reviewed with limits** means useful observations were evaluated while some signals remain unavailable; **Insufficient data** is reserved for inadequate assessed coverage. Each signal shows its measured and detector-eligible counts separately. Existing libraries automatically reparse available originals when telemetry support changes, preserving recording identities, notes and selected maps.

Original demo files are never overwritten. Removing a demo from the library removes only its cached analysis and notes. Demo files must remain at their original location for native playback. Local storage defaults to the Electron user-data directory's `library` folder; Settings shows its exact location.

## Verification and packaging

```powershell
npm test
npm run test:worker
npm run build
node scripts/integration.mjs 'C:\path\to\match.dem'
node scripts/smoke-electron.mjs
node scripts/setup-tools.mjs --extractor
npm run package
```

Packages appear under `release/` with SHA-256 checksums. Builds include parser executables and, when provisioned, the Source2Viewer CLI and its license. The release pipeline produces unsigned beta artifacts; public signing requires publisher credentials. No automatic upload or publishing occurs.

For telemetry fixture validation, set `DEMO_TEST_DATA_DIR` to a test-library directory before importing the Inferno, Ancient and legacy CS:GO fixtures, then run `npm run test:telemetry` with the same setting. This refreshes derived analysis, checks measured coverage and verifies that notes and selected maps survive.

Source2Viewer is an optional local extraction dependency from [s2v.app](https://s2v.app). A pinned portable CLI can be provisioned into `.tools/source2viewer` using `node scripts/setup-tools.mjs --extractor`. Users can also select an existing CLI executable in Settings.

## Analysis limits

- Reaction-time measurements are approximate and depend on available timing, visibility, eye positions, and matching map geometry.
- Network spotted-to-shot intervals are labelled proxies, not exact visual reaction times. CS2 native recoil packets and CS:GO sampled entity recoil retain different source-specific scales.
- Unsupported fields stay unavailable. Missing data is not interpreted as zero recoil or a low-risk signal.
- Repeated corroborating episodes carry more weight than individual flicks, prefires, accurate sprays, or lucky shots.
- Shot impacts do not establish curved bullets. Penetration, pellets, recoil, spread, and replay sampling affect interpretation.
- Detailed experimental thresholds and map-pack requirements are documented under `docs/`.
- Your own suspicious matches remain unlabelled examples until independently reviewed; this beta makes no accuracy-rate claims.

See [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md) and [CHANGELOG.md](CHANGELOG.md).
