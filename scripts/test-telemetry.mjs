// Integration contract for a fixture library imported by scripts/integration.mjs.
// Reanalysis writes only derived results in the explicitly selected test library.
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { randomUUID } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import assert from 'node:assert/strict';

const dataDir = resolve(process.env.DEMO_TEST_DATA_DIR || '.tmp/telemetry-validation');
const worker = spawn(resolve('worker/bin/demo-worker.exe'), ['--data-dir', dataDir], { stdio: 'pipe', windowsHide: true });
const pending = new Map();
const failures = [];
worker.stderr.on('data', data => failures.push(data.toString()));
createInterface({ input: worker.stdout }).on('line', line => {
  const message = JSON.parse(line);
  if (!message.id) return;
  const request = pending.get(message.id);
  if (!request) return;
  pending.delete(message.id);
  message.error ? request.reject(new Error(message.error.message)) : request.resolve(message.result);
});
worker.on('error', error => { for (const p of pending.values()) p.reject(error); });
const closed = new Promise(resolve => worker.on('close', resolve));
function rpc(method, params = {}) {
  return new Promise((resolve, reject) => {
    const id = randomUUID(); pending.set(id, { resolve, reject });
    worker.stdin.write(`${JSON.stringify({ id, method, params })}\n`);
  });
}
const timer = setTimeout(() => { for (const p of pending.values()) p.reject(new Error('Telemetry validation timed out')); worker.kill(); }, 180_000);
let db;
try {
  const demos = await rpc('listDemos');
  assert.ok(demos.length > 0, 'Import the fixture demos before telemetry validation');
  const results = [];
  for (const demo of demos) {
    const before = await rpc('getMatch', { id: demo.id });
    const beforeMap = await rpc('getMap', { id: demo.id });
    await rpc('reanalyse', { id: demo.id });
    const match = await rpc('getMatch', { id: demo.id });
    assert.equal(match.demo.id, before.demo.id);
    assert.equal(match.demo.hash, before.demo.hash);
    assert.deepEqual(match.notes, before.notes);
    assert.deepEqual(await rpc('getMap', { id: demo.id }), beforeMap);
    assert.equal(match.demo.analysisVersion, 'beta-rules-1.2.0');
    assert.match(match.demo.parserVersion, /adapter:4$/);
    const counts = { reaction: 0, recoil: 0, 'shot-direction': 0 };
    const verdicts = {};
    for (const player of match.players) {
      assert.ok(player.review);
      verdicts[player.verdict] = (verdicts[player.verdict] || 0) + 1;
      assert.equal(player.review.eligibleAimShots + player.review.exclusions.reduce((n, e) => n + e.count, 0), player.review.totalShots);
      for (const metric of player.review.metrics) {
        assert.ok(Number.isFinite(metric.value) && metric.samples > 0);
        assert.ok(metric.provenance && metric.signal);
      }
      for (const capability of player.capabilities) {
        if (capability.signal in counts) counts[capability.signal] += capability.measuredSamples ?? 0;
        if (capability.status === 'measured') assert.equal(capability.samples, 0);
      }
    }
    assert.ok(counts.recoil > 0, `${demo.map}: no measured recoil from fixture`);
    assert.ok(counts.reaction > 0, `${demo.map}: no recorded spotting or visibility intervals`);
    if (demo.engine === 'cs2') assert.ok(counts['shot-direction'] > 0, `${demo.map}: no recovered native firing directions`);
    results.push({ map: demo.map, engine: demo.engine, players: match.players.length, findings: match.findings.length, verdicts, measuredObservations: counts, cacheIdentityNotesAndMapPreserved: true });
  }
  db = new DatabaseSync(resolve(dataDir, 'library.db'), { readOnly: true });
  for (const result of results) {
    const demo = demos.find(d => d.map === result.map && d.engine === result.engine);
    const shots = db.prepare('SELECT data FROM shots WHERE demo_id=?').all(demo.id).map(row => JSON.parse(row.data));
    result.recordedShots = shots.length;
    result.nativeRecoilShots = shots.filter(s => s.aimPunch && s.aimPunchScale === 1 && s.provenance.includes('shot-native')).length;
    result.legacySampledRecoilShots = shots.filter(s => s.aimPunch && s.aimPunchScale === 2 && s.provenance.includes('sampled-recoil')).length;
    result.nativeFiringDirections = shots.filter(s => s.nativeAngles).length;
    for (const shot of shots) if (shot.aimPunch) assert.equal(shot.aimPunchScale, result.engine === 'cs2' ? 1 : 2);
  }
  await mkdir('artifacts', { recursive: true });
  await writeFile('artifacts/telemetry-review-results.json', JSON.stringify({ passed: true, results }, null, 2));
  console.log(JSON.stringify({ passed: true, results }, null, 2));
} catch (error) {
  console.error(error); console.error(failures.join('').slice(-2000)); process.exitCode = 1;
} finally {
  db?.close(); clearTimeout(timer); worker.stdin.end(); await closed;
}
