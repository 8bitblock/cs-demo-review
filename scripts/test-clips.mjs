import { _electron as electron } from '@playwright/test';
import { isAbsolute, relative, resolve, sep } from 'node:path';
import { copyFile, mkdir, readFile, stat, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import assert from 'node:assert/strict';

// This captures the already-running game window without loading a demo,
// sending keys, changing player, or seeking. It tests video plumbing only.
// Its review timestamp does not claim that the source game matches the demo.
const workspace = resolve('.');
const dataDir = resolve(process.env.DEMO_TEST_DATA_DIR || '.tmp/packaged-validation');
const withinTemp = relative(resolve(workspace, '.tmp'), dataDir);
assert.ok(withinTemp && !isAbsolute(withinTemp) && !withinTemp.startsWith(`..${sep}`) && withinTemp !== '..', 'Use a dedicated .tmp test library.');
const artifactDir = resolve('artifacts');
await mkdir(artifactDir, { recursive: true });
const packaged = process.env.DEMO_PACKAGED_EXE;
const app = await electron.launch({
  ...(packaged ? { executablePath: resolve(packaged) } : {}),
  args: packaged ? [] : ['.'],
  env: { ...process.env, CS_DEMO_REVIEW_DATA_DIR: dataDir, ...(packaged ? { PATH: `${process.env.SystemRoot}\\System32;${process.env.SystemRoot}` } : {}) },
  timeout: 60_000,
});
const page = await app.firstWindow();
const errors = [], createdIds = [];
const result = { passed: false, scenario: 'Capture the existing game picture; this does not verify demo identity, game timestamp, or selected player.', dataDir, packaged: !!packaged, errors };
page.on('pageerror', error => errors.push(String(error)));
if (process.env.DEMO_CAPTURE_TRACE === '1') {
  await app.evaluate(({ session, BrowserWindow }, allowDisplayMediaShape) => {
    const owner = BrowserWindow.getAllWindows()[0].webContents;
    globalThis.__captureTrace = [];
    const record = (kind, wc, permission, details) => {
      globalThis.__captureTrace.push({ kind, owner: wc === owner, permission, details });
      const desktopRequest = allowDisplayMediaShape && kind === 'request' && permission === 'media' && details.isMainFrame === true && Array.isArray(details.mediaTypes) && details.mediaTypes.length === 0;
      return (permission === 'display-capture' || desktopRequest) && wc === owner;
    };
    // Diagnostic permission callbacks still deny microphone/camera/all other
    // permissions, and leave the product's exact game-source grant intact.
    session.defaultSession.setPermissionCheckHandler((wc, permission, origin, details) => record('check', wc, permission, { origin, ...details }));
    session.defaultSession.setPermissionRequestHandler((wc, permission, callback, details) => callback(record('request', wc, permission, details)));
  }, process.env.DEMO_CAPTURE_TRACE_ALLOW_MEDIA === '1');
}
const savedVideo = () => page.getByRole('region', { name: 'Recorded game clips' }).locator('video');
const hashFile = async file => createHash('sha256').update(await readFile(file)).digest('hex');

async function videoState(locator) {
  return locator.evaluate(video => {
    const canvas = document.createElement('canvas'); canvas.width = 64; canvas.height = 36;
    const context = canvas.getContext('2d', { willReadFrequently: true });
    let mean = null, variance = null, pixelError = null;
    try {
      if (video.readyState >= 2) context.drawImage(video, 0, 0, canvas.width, canvas.height);
      const pixels = context.getImageData(0, 0, canvas.width, canvas.height).data;
      const values = []; for (let i = 0; i < pixels.length; i += 4) values.push((pixels[i] + pixels[i + 1] + pixels[i + 2]) / 3);
      mean = values.reduce((sum, value) => sum + value, 0) / values.length;
      variance = values.reduce((sum, value) => sum + (value - mean) ** 2, 0) / values.length;
    } catch (error) { pixelError = String(error); }
    const ranges = value => Array.from({ length: value.length }, (_, i) => [value.start(i), value.end(i)]);
    return { readyState: video.readyState, networkState: video.networkState, paused: video.paused, seeking: video.seeking, buffered: ranges(video.buffered), seekable: ranges(video.seekable), width: video.videoWidth, height: video.videoHeight, time: video.currentTime, duration: Number.isFinite(video.duration) ? video.duration : String(video.duration), decodedFrames: video.getVideoPlaybackQuality().totalVideoFrames, pixelMean: mean, pixelVariance: variance, pixelError, error: video.error?.message ?? null };
  });
}

async function selectDemo(demo) {
  await page.locator('.demo-card').filter({ hasText: demo.name }).locator('.demo-select').click();
  await page.getByRole('button', { name: 'Recorded clips', exact: true }).waitFor({ timeout: 30_000 });
  await page.getByRole('button', { name: 'Recorded clips', exact: true }).click();
  await page.getByRole('region', { name: 'Recorded game clips' }).waitFor();
}

try {
  result.phase = 'desktop startup';
  await page.waitForFunction(() => !!window.csDemo?.beginClipRecording, undefined, { timeout: 30_000 });
  const demos = await page.evaluate(() => window.csDemo.listDemos());
  const demo = demos.find(item => item.engine === 'cs2');
  assert.ok(demo, 'A CS2 demo must already exist in the test library.');
  result.demoId = demo.id;
  const before = await page.evaluate(id => window.csDemo.listClips(id), demo.id);
  const beforeIds = new Set(before.map(clip => clip.id));
  result.phase = 'game window selection';
  await selectDemo(demo);
  await page.getByRole('button', { name: 'Record from game', exact: true }).click();
  await page.getByRole('button', { name: 'Connect game picture', exact: true }).click({ timeout: 30_000 });
  result.phase = 'live video frames';
  await page.waitForFunction(() => {
    const video = document.querySelector('video[aria-label="Live Counter-Strike game window"]');
    return (!!video && video.readyState >= 2 && video.videoWidth > 0) || !!document.querySelector('.game-error');
  }, undefined, { timeout: 30_000 });
  if (await page.locator('.game-error').count()) throw new Error(await page.locator('.game-error').textContent() || 'Game capture failed.');
  const live = page.getByLabel('Live Counter-Strike game window');
  result.live = await videoState(live);
  assert.ok(result.live.pixelMean > 1 && result.live.pixelVariance > 4, `Capture has no usable game picture: ${JSON.stringify(result.live)}. Restore the game or use borderless/windowed mode.`);
  await page.screenshot({ path: resolve(artifactDir, 'clip-capture-existing-game.png') });
  await page.getByLabel('Clip length', { exact: true }).selectOption('10');
  await page.getByRole('button', { name: 'Record 10s clip', exact: true }).click();
  result.phase = 'ten-second recording and save';
  await page.getByRole('region', { name: 'Recorded game clips' }).waitFor({ timeout: 30_000 });
  const after = await page.evaluate(id => window.csDemo.listClips(id), demo.id);
  const recorded = after.find(clip => !beforeIds.has(clip.id));
  assert.ok(recorded && recorded.source === 'recorded', 'The clip must persist as a real recorded video.');
  createdIds.push(recorded.id);
  assert.ok(recorded.size > 1024 && recorded.duration >= 9 && recorded.duration < 14, 'Recording must have data and about ten seconds of duration.');
  result.recorded = recorded;
  const recordedPath = resolve(dataDir, 'clips', `${recorded.id}.webm`);
  const exportedPath = resolve(artifactDir, 'validation-existing-game-capture.webm');
  await copyFile(recordedPath, exportedPath);
  result.phase = 'saved video playback';
  await savedVideo().evaluate(video => video.play());
  await page.waitForFunction(() => {
    const video = document.querySelector('video[aria-label="Recorded Counter-Strike clip"]');
    return !!video && video.currentTime > 1 && video.getVideoPlaybackQuality().totalVideoFrames > 0;
  }, undefined, { timeout: 20_000 });
  result.playback = await videoState(savedVideo());
  assert.equal(result.playback.error, null);
  assert.ok(Number.isFinite(result.playback.duration), 'Saved WebM must expose a finite duration for the native video scrubber.');
  assert.ok(Math.abs(result.playback.duration - recorded.duration) < 0.5, 'Saved video duration must agree with the recorded duration.');
  assert.ok(result.playback.seekable.some(([from, to]) => from <= 3 && to >= 3), `Saved video must advertise a seekable range covering three seconds; got ${JSON.stringify(result.playback.seekable)}.`);
  assert.ok(result.playback.width > 0 && result.playback.decodedFrames > 0, 'Saved video must decode real frames.');
  if (!result.playback.pixelError) assert.ok(result.playback.pixelMean > 1 && result.playback.pixelVariance > 4, 'Saved video must decode a non-blank picture.');
  await savedVideo().evaluate(video => { video.pause(); video.currentTime = 3; });
  result.phase = 'saved video seek';
  await page.waitForFunction(() => {
    const video = document.querySelector('video[aria-label="Recorded Counter-Strike clip"]');
    return !!video && !video.seeking && video.currentTime >= 2.9 && video.currentTime <= 3.2;
  }, undefined, { timeout: 15_000 });
  result.seek = await videoState(savedVideo());
  await page.screenshot({ path: resolve(artifactDir, 'clip-playback-existing-game.png') });
  const originalHash = await hashFile(recordedPath), originalSize = (await stat(recordedPath)).size;
  result.phase = 'reload persisted clip';
  await page.reload();
  await page.waitForFunction(() => !!window.csDemo);
  await selectDemo(demo);
  await savedVideo().evaluate(video => video.play());
  await page.waitForFunction(() => {
    const video = document.querySelector('video[aria-label="Recorded Counter-Strike clip"]');
    return !!video && video.readyState >= 2 && video.currentTime > .2;
  }, undefined, { timeout: 20_000 });
  result.reload = await videoState(savedVideo());
  assert.ok(Number.isFinite(result.reload.duration), 'Reopened WebM must retain a finite duration.');
  assert.ok(Math.abs(result.reload.duration - recorded.duration) < 0.5, 'Reopened video duration must agree with the recorded duration.');
  await savedVideo().evaluate(video => video.pause());
  // The open dialog is the only mocked piece: the source is our just-recorded file.
  await app.evaluate(({ dialog }, source) => { dialog.showOpenDialog = async () => ({ canceled: false, filePaths: [source] }); }, exportedPath);
  result.phase = 'import recorded WebM copy';
  await page.getByRole('button', { name: 'Import clip', exact: true }).click();
  let imported;
  const importDeadline = Date.now() + 20_000;
  while (Date.now() < importDeadline) {
    imported = (await page.evaluate(id => window.csDemo.listClips(id), demo.id)).find(clip => !beforeIds.has(clip.id) && clip.id !== recorded.id);
    if (imported) { createdIds.push(imported.id); break; }
    await new Promise(resolve => setTimeout(resolve, 200));
  }
  assert.ok(imported && imported.source === 'imported');
  assert.equal(await hashFile(recordedPath), originalHash);
  assert.equal((await stat(recordedPath)).size, originalSize);
  assert.equal(await hashFile(exportedPath), originalHash);
  assert.equal(await hashFile(resolve(dataDir, 'clips', `${imported.id}.webm`)), originalHash);
  result.imported = imported;
  await page.waitForFunction(url => document.querySelector('video[aria-label="Recorded Counter-Strike clip"]')?.getAttribute('src') === url, imported.url, { timeout: 10_000 });
  await savedVideo().evaluate(video => video.play());
  await page.waitForFunction(() => {
    const video = document.querySelector('video[aria-label="Recorded Counter-Strike clip"]');
    return !!video && video.readyState >= 2 && video.currentTime > .2;
  }, undefined, { timeout: 20_000 });
  result.importPlayback = await videoState(savedVideo());
  assert.equal(result.importPlayback.error, null);
  assert.ok(Number.isFinite(result.importPlayback.duration), 'An imported copy must retain the fixed WebM duration.');
  assert.ok(Math.abs(result.importPlayback.duration - recorded.duration) < 0.5, 'Imported video duration must agree with the source recording.');
  assert.deepEqual(errors, []);
  result.phase = 'complete';
  result.passed = true;
} catch (error) {
  result.failure = String(error);
  if (await savedVideo().count()) result.failedVideo = await videoState(savedVideo()).catch(error => ({ error: String(error) }));
  result.body = (await page.locator('body').innerText().catch(() => '')).slice(-9000);
  await page.screenshot({ path: resolve(artifactDir, 'clip-test-failure.png') }).catch(() => {});
  process.exitCode = 1;
} finally {
  if (process.env.DEMO_CAPTURE_TRACE === '1') result.permissionTrace = await app.evaluate(() => globalThis.__captureTrace ?? []);
  // Remove only IDs created by this run. Existing clips are never removed.
  if (createdIds.length) {
    await app.evaluate(({ dialog }) => { dialog.showMessageBox = async () => ({ response: 1, checkboxChecked: false }); });
    for (const id of createdIds) await page.evaluate(id => window.csDemo.removeClip(id), id).catch(error => errors.push(`Cleanup ${id}: ${error}`));
  }
  await writeFile(resolve(artifactDir, 'clips-validation.json'), JSON.stringify(result, null, 2));
  await app.close();
  console.log(JSON.stringify(result, null, 2));
}
