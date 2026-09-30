import { _electron as electron } from '@playwright/test';
import { resolve } from 'node:path';
import { mkdir, writeFile } from 'node:fs/promises';
import assert from 'node:assert/strict';
const dataDir = resolve(process.env.DEMO_TEST_DATA_DIR || '.tmp/release-validation');
const packaged = process.env.DEMO_PACKAGED_EXE;
const expectedAnalysisVersion = process.env.DEMO_EXPECTED_ANALYSIS_VERSION || 'beta-rules-1.2.0';
const app = await electron.launch({ ...(packaged ? { executablePath: resolve(packaged) } : {}), args: packaged ? [] : ['.'], env: { ...process.env, CS_DEMO_REVIEW_DATA_DIR: dataDir }, timeout: 60_000 });
const page = await app.firstWindow();
const errors = []; page.on('pageerror', error => errors.push(String(error)));
try {
  await page.waitForFunction(() => !!window.csDemo);
  const demos = await page.evaluate(() => window.csDemo.listDemos());
  const demo = demos.find(value => value.engine === 'cs2' && value.map === 'de_dust2') ?? demos.find(value => value.engine === 'cs2'); assert.ok(demo);
  await page.locator('.demo-card').filter({ has: page.locator('.demo-filename', { hasText: demo.name }) }).locator('.demo-select').click();
  let match;
  const deadline = Date.now() + 180_000;
  while (Date.now() < deadline) {
    match = await page.evaluate(id => window.csDemo.getMatch(id), demo.id);
    if (match.demo.analysisVersion === expectedAnalysisVersion && match.players.some(player => player.review?.clips?.length)) break;
    await page.waitForTimeout(1000);
  }
  assert.equal(match.demo.analysisVersion, expectedAnalysisVersion, 'Wait for the completed rules refresh before comparing UI counts');
  // Everything below uses the actual worker result. No IPC overrides, model
  // injection, sample generation, or library writes are used in this smoke test.
  const player = [...match.players].filter(value => value.review?.clips?.length).sort((a, b) => b.review.eligibleAimShots - a.review.eligibleAimShots)[0]; assert.ok(player);
  await page.locator('.player-name>span:nth-child(2)').getByText(player.name, { exact: true }).click();
  await page.locator('.review-counts').waitFor({ timeout: 30_000 });
  await page.getByText('RADAR REPLAY', { exact: true }).waitFor({ timeout: 120_000 });
  const review = player.review;
  const displayedCounts = await page.locator('.review-counts strong').allInnerTexts();
  assert.deepEqual(displayedCounts.map(value => Number(value.replaceAll(',', ''))), [review.totalShots, review.sampledShots, review.eligibleAimShots, review.coveredRounds]);
  const clip = review.clips[0];
  await page.locator('.review-clips article>button').first().click();
  assert.equal(Number(await page.getByRole('slider', { name: 'Replay position', exact: true }).inputValue()), clip.tick);
  await page.locator('.review-clip-context').waitFor();
  await page.waitForTimeout(500);
  const replay = await page.evaluate(({ id, tick }) => window.csDemo.getReplay(id, Math.max(0, tick - 64), tick + 64), { id: demo.id, tick: clip.tick });
  assert.ok(replay.samples.some(value => value.alive), 'The selected real moment must have recorded player positions');
  assert.ok(await page.locator('.map-surface canvas').evaluate(element => element.width > 0 && element.height > 0));
  const radarPixelAccess = await page.locator('.map-surface canvas').evaluate(element => { try { element.getContext('2d').getImageData(0, 0, 1, 1); return { readable: true, reason: '' }; } catch (error) { return { readable: false, reason: String(error) }; } });
  const fonts = await page.locator('.review-summary, .assessment-card>p, .capability p').evaluateAll(elements => elements.map(element => Number.parseFloat(getComputedStyle(element).fontSize)));
  assert.ok(fonts.every(value => value >= 12), 'Primary evidence copy must remain readable');
  await mkdir('artifacts', { recursive: true });
  await page.screenshot({ path: resolve('artifacts/review-moment-real.png') });
  if (await page.getByRole('button', { name: 'Clear', exact: true }).count()) await page.getByRole('button', { name: 'Clear', exact: true }).click();
  await page.locator('.inspector-scroll').evaluate(element => { element.scrollTop = 0; });
  await page.screenshot({ path: resolve('artifacts/app-preview-beta2.png') });
  await page.setViewportSize({ width: 1100, height: 760 }); await page.waitForTimeout(150);
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 2), false);
  const canvasBounds = await page.locator('.map-surface').boundingBox(); assert.ok(canvasBounds.height >= 150);
  await page.screenshot({ path: resolve('artifacts/review-real-compact.png') });
  assert.deepEqual(errors, []);
  const result = { passed: true, demo: demo.name, map: demo.map, analysisVersion: match.demo.analysisVersion, player: player.name, verdict: player.verdict, reviewCounts: { totalShots: review.totalShots, sampledShots: review.sampledShots, eligibleAimShots: review.eligibleAimShots, coveredRounds: review.coveredRounds }, moments: review.clips.length, allPlayerMoments: match.players.reduce((sum, value) => sum + (value.review?.clips?.length ?? 0), 0), selectedMoment: { title: clip.title, tick: clip.tick, round: clip.round }, radarPixelAccess, errors };
  await writeFile('artifacts/ui-review-real-results.json', JSON.stringify(result, null, 2));
  console.log(JSON.stringify(result, null, 2));
} finally { await app.close(); }
