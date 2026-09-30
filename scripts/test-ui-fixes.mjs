import { _electron as electron } from '@playwright/test';
import { resolve } from 'node:path';
import { mkdir } from 'node:fs/promises';
import assert from 'node:assert/strict';
const dataDir = resolve(process.env.DEMO_TEST_DATA_DIR || '.tmp/release-validation');
const packaged = process.env.DEMO_PACKAGED_EXE;
const app = await electron.launch({ ...(packaged ? { executablePath: resolve(packaged) } : {}), args: packaged ? [] : ['.'], env: { ...process.env, CS_DEMO_REVIEW_DATA_DIR: dataDir }, timeout: 60_000 });
const page = await app.firstWindow();
const errors = []; page.on('pageerror', error => errors.push(String(error)));
try {
  await page.waitForFunction(() => !!window.csDemo);
  const demos = await page.evaluate(() => window.csDemo.listDemos());
  const demo = demos.find(value => value.engine === 'cs2'); assert.ok(demo);
  const map = await page.evaluate(id => window.csDemo.getMap(id), demo.id); assert.ok(map?.image);
  const match = await page.evaluate(id => window.csDemo.getMatch(id), demo.id);
  // These are UI contract fixtures, injected only into this test process. They
  // exercise missing-assets and neutral review states without changing a library.
  match.players[0].review = {
    summary: 'Test fixture: 100 shots were sampled for manual review.', totalShots: 120, sampledShots: 100, eligibleAimShots: 80, coveredRounds: 9, medianSampleMs: 31.25,
    metrics: [{ label: 'Median turn speed', value: 42.6, unit: 'degrees/s', samples: 100 }],
    exclusions: [{ reason: 'Missing original samples', count: 20 }],
    clips: [{ id: 'ui-test-moment', round: 2, tick: Math.round(demo.tickRate * 180), endTick: Math.round(demo.tickRate * 182), time: 180, title: 'Test fixture: sampled aim change', description: 'A neutral review moment, not a finding.', measurements: [{ label: 'Turn speed', value: 82.5, unit: 'degrees/s' }], limitations: ['No visibility claim is made.'] }],
  };
  await app.evaluate(({ ipcMain }, { map, match }) => {
    globalThis.__mapAttempts = 0;
    ipcMain.removeHandler('demo:getMap'); ipcMain.handle('demo:getMap', () => null);
    ipcMain.removeHandler('demo:getMatch'); ipcMain.handle('demo:getMatch', () => match);
    ipcMain.removeHandler('demo:extractMap'); ipcMain.handle('demo:extractMap', async () => {
      globalThis.__mapAttempts++;
      await new Promise(resolve => setTimeout(resolve, 250));
      if (globalThis.__mapAttempts === 1) throw new Error('Test fixture: game installation needs attention.');
      return map;
    });
  }, { map, match });
  await page.reload();
  await page.getByText('Map image could not be loaded', { exact: true }).waitFor();
  assert.equal(await app.evaluate(() => globalThis.__mapAttempts), 1, 'Autoload must run exactly once');
  await page.getByRole('button', { name: 'Retry map', exact: true }).click();
  await page.getByText('RADAR REPLAY', { exact: true }).waitFor();
  const otherDemo = demos.find(value => value.id !== demo.id);
  if (otherDemo) {
    const selectedBefore = await page.locator('.demo-card.selected .demo-filename').innerText();
    await app.evaluate(({ BrowserWindow }, other) => BrowserWindow.getAllWindows()[0].webContents.send('demo:import-progress', { jobId: 'refresh-regression', path: other.path, name: other.name, stage: 'complete', progress: 1, message: 'Background rules refreshed', demoId: other.id }), otherDemo);
    await page.waitForTimeout(150);
    assert.equal(await page.locator('.demo-card.selected .demo-filename').innerText(), selectedBefore, 'Background analysis must preserve the selected demo');
  }
  assert.equal(await app.evaluate(() => globalThis.__mapAttempts), 2);
  await page.getByRole('button', { name: /Test fixture: sampled aim change/ }).click();
  assert.equal(Number(await page.getByRole('slider', { name: 'Replay position', exact: true }).inputValue()), Math.round(demo.tickRate * 180));
  await page.getByText('No visibility claim is made.', { exact: true }).waitFor();
  await page.getByRole('button', { name: 'Expand replay', exact: true }).click();
  assert.equal(await page.locator('.bottom-panel').isVisible(), false);
  await page.getByRole('button', { name: 'Show timeline and roster', exact: true }).click();
  const font = await page.locator('.review-summary').first().evaluate(element => Number.parseFloat(getComputedStyle(element).fontSize));
  assert.ok(font >= 13, 'Analysis copy must remain readable');
  const canvas = page.locator('.map-surface canvas'); const box = await canvas.boundingBox(); assert.ok(box);
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2); await page.mouse.wheel(0, -120);
  await page.waitForTimeout(100); assert.notEqual(await page.locator('.map-controls>span').innerText(), '100%');
  await page.mouse.down(); await page.mouse.move(box.x + box.width / 2 + 35, box.y + box.height / 2 + 20); await page.mouse.up();
  await page.getByRole('button', { name: 'Reset zoom', exact: true }).click();
  await mkdir('artifacts', { recursive: true });
  await page.screenshot({ path: resolve('artifacts/readability-review.png') });
  await page.setViewportSize({ width: 1100, height: 760 }); await page.waitForTimeout(100);
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 2), false);
  await page.screenshot({ path: resolve('artifacts/readability-review-compact.png') });
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ passed: true, scenarios: ['automatic map loading', 'visible failure', 'retry', 'review results', 'neutral moment seeking', 'expand replay', 'readable typography', 'zoom and pan', 'compact layout', 'background analysis selection preservation'], errors }, null, 2));
} finally { await app.close(); }
