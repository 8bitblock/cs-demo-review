# Recorded game clips

The full 3D picture comes from the installed Counter-Strike game. Its renderer provides the real map, materials, players, animations, weapons, and effects. You can record this game picture as a local WebM clip and watch it later inside the app, or import an existing MP4/WebM. The app's 2D timeline is a separate view of the parsed recording.

1. Select a player and review moment, open **Recorded clips**, and choose **Record from game**.
2. Open CS2 or a compatible legacy CS:GO installation. Copy **Load demo** and paste it into the game console.
3. Once the demo has loaded, copy and paste **Seek to…**, then choose the correct player with the game's spectator controls.
4. Choose **Connect game picture**. Keep the game window restored; a minimized or exclusive-fullscreen game can stop supplying frames. Use borderless/windowed mode if the preview is blank or frozen.
5. Resume playback in the game. Choose 10, 20, 30, 60, or 120 seconds and press **Record… clip**. **Stop and save** finishes early. Saved footage opens in the app with standard video play, pause, and seek controls.

Clips capture video only. Audio continues playing from the game while recording but is not saved. Leaving the recording view or changing demos discards an unfinished recording. Saved video remains available after restarting the app. The app limits a recording to 120 seconds and 160 MB.

**Import clip** copies a local MP4 or WebM video into the app's clip library. MP4 with H.264 or WebM with VP8/VP9 are supported by the player. The source must be non-empty and smaller than 4 GB. A saved clip can be removed from the library; an imported source video is kept.

If the game is already running, the app returns the play/seek commands rather than launching a second process with arguments the running game may ignore. The game process is never terminated or given simulated keyboard input. No persistent game settings or bindings are overwritten.

Window identification queries the native Windows process and window lists. A capturable window must belong to the configured `cs2.exe` or `csgo.exe` path, and its native window handle must match the Electron capture source. A similar window title is insufficient. Capture starts only after selecting **Connect game picture**, is restricted to that game window, and does not include monitors, microphone audio, or system audio. The identification helper accepts no user-provided PowerShell source. An inaccessible process is treated as running but cannot grant capture.

The preview records whatever that game window currently shows. It does not verify which demo is loaded, its timestamp, or its selected player. The saved clip's review timestamp is a manual reference, not verified synchronization. Moving the app's review timeline does not seek the game or a saved video. Use the video's own controls to seek recorded footage. Historical demos may need a compatible game build; parser support alone does not ensure the installed game can play a recording.

Recordings and imported copies remain in the local library's `clips` directory with review metadata. There is no upload or external recording service. The app does not generate a 3D scene from parsed samples; saved footage contains the geometry and effects rendered by the game during capture.

## Sources and checks

- [Electron desktopCapturer](https://www.electronjs.org/docs/latest/api/desktop-capturer) documents window sources and the `getDisplayMedia` flow through `setDisplayMediaRequestHandler`.
- [Microsoft EnumWindows](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-enumwindows) and [GetWindowThreadProcessId](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getwindowthreadprocessid) describe the native handle/process association used by the helper.
- [CS Demo Manager's action generator](https://github.com/akiver/cs-demo-manager/blob/main/src/node/counter-strike/json-actions-file/json-actions-file-generator.ts) is primary-source evidence for its own timed command system. Its JSON actions require a game server plugin; writing that JSON alone is not native CS2 automation. This app does not install that plugin or claim this scheduling capability.
- On 2026-09-25, the helper correctly identified the existing CS2 window belonging to the detected B: Steam library installation, without interacting with the game. Five focused unit tests cover Unicode/spaced demo paths, tick bounds, console injection, executable/engine validation, moved files, exact HWND/process matching, and inaccessible processes. The game was already running; automatic launch and game-side seek were not exercised in that check.
- `node scripts/test-clips.mjs` verifies the complete recording path against a running game and a test-only library under `.tmp`. The 2026-09-25 run captured a nonblank 1920 × 1080 game picture for ten seconds, saved a WebM, decoded it in the app, sought to three seconds, reloaded it, imported a copy, and verified the original file hash stayed unchanged. The game's existing inventory screen was left untouched; this check proves recording/playback plumbing, not demo identity or timestamp/player alignment. Results are written to `artifacts/clips-validation.json`; test clips created by a successful run are removed from its test library.
- The final packaged beta.2 executable passed the same test with Node/Go tools absent from its child-process PATH. Saved footage exposed a finite 10.0144-second duration and the complete seekable range, including after reload and importing a copy. No renderer errors occurred. This check exercised the shipped game-window permission handling, WebM duration finalization, and video byte-range responses.
