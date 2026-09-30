import type { CapabilityStatus, MatchDetail, Player, Signal } from '../shared/types';

const signalLabel: Record<Signal, string> = { reaction: 'Visibility reaction', acquisition: 'Crosshair acquisition', 'aim-snap': 'Aim transitions', recoil: 'Recoil compensation', 'shot-direction': 'Shot direction' };
const capabilityLabel: Record<CapabilityStatus, string> = { available: 'Assessed', limited: 'Limited sample', measured: 'Measured', insufficient: 'Missing observations', unsupported: 'Unavailable' };

export const escapeHTML = (value: unknown) => String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]!));

export function reportJSON(match: MatchDetail) {
  const { path: _path, ...demo } = match.demo;
  return JSON.stringify({ formatVersion: 1, exportedAt: new Date().toISOString(), notice: 'Experimental evidence review. Assessments are not proof of cheating or a guarantee of legitimate play. Player review summaries, metrics, exclusions, and neutral review clips are descriptive context, not additional cheating findings.', demo, players: match.players, findings: match.findings, rounds: match.rounds, notes: match.notes }, null, 2);
}

function timestamp(seconds: number) {
  if (!Number.isFinite(seconds) || seconds < 0) return 'Time unavailable';
  const rounded = Math.floor(seconds);
  return `${Math.floor(rounded / 60)}:${String(rounded % 60).padStart(2, '0')}`;
}

function playerReviewHTML(player: Player) {
  const h = escapeHTML;
  const review = player.review;
  if (!review) return `<article class="player-review"><h3>${h(player.name)}</h3><p class="muted">Detailed review data was not available when this report was exported.</p></article>`;
  const metrics = review.metrics.map(metric => `<tr><th scope="row">${h(metric.label)}${metric.signal ? `<small class="metric-signal">${h(signalLabel[metric.signal])}</small>` : ''}</th><td>${Number.isFinite(metric.value) ? h(metric.value) : 'Unavailable'} ${h(metric.unit)}</td><td>${h(metric.samples)}</td><td>${h(metric.provenance || 'Source not recorded')}</td></tr>`).join('');
  const exclusions = review.exclusions.map(exclusion => `<tr><th scope="row">${h(exclusion.reason)}</th><td>${h(exclusion.count)}</td></tr>`).join('');
  const clips = review.clips.map(clip => `<section class="review-clip" data-review-kind="neutral">
    <div class="meta">NEUTRAL REVIEW CLIP · ROUND ${h(clip.round)} · TICK ${h(clip.tick)}${clip.endTick !== clip.tick ? `–${h(clip.endTick)}` : ''} · ${h(timestamp(clip.time))}${clip.signal ? ` · ${h(signalLabel[clip.signal])}` : ''}</div>
    <h5>${h(clip.title)}</h5><p>${h(clip.description)}</p>
    ${clip.measurements.length ? `<ul>${clip.measurements.map(m => `<li>${h(m.label)}: <strong>${h(m.value)} ${h(m.unit)}</strong></li>`).join('')}</ul>` : ''}
    <p class="neutral-label">Manual review bookmark; not a cheating finding and not included in the evidence count.</p>
    ${clip.provenance ? `<p class="muted"><b>Source:</b> ${h(clip.provenance)}</p>` : ''}
    ${clip.limitations.length ? `<p><b>Limits:</b></p><ul>${clip.limitations.map(limit => `<li>${h(limit)}</li>`).join('')}</ul>` : ''}
    <p class="muted identifiers">Clip ID: <code>${h(clip.id)}</code></p>
  </section>`).join('');
  return `<article class="player-review">
    <h3>${h(player.name)}</h3><p class="muted identifiers">Player ID: <code>${h(player.id)}</code> · Assessment: ${h(player.verdict)}</p>
    <p>${h(review.summary)}</p>
    <dl class="review-counts">
      <div><dt>Recorded shots</dt><dd>${h(review.totalShots)}</dd></div>
      <div><dt>Shots with player samples</dt><dd>${h(review.sampledShots)}</dd></div>
      <div><dt>Eligible aim shots</dt><dd>${h(review.eligibleAimShots)}</dd></div>
      <div><dt>Rounds with shots</dt><dd>${h(review.coveredRounds)}</dd></div>
      <div><dt>Median sample interval</dt><dd>${review.medianSampleMs === null ? 'Unavailable' : `${h(review.medianSampleMs)} ms`}</dd></div>
    </dl>
    <h4>Measured context</h4><p class="muted">These describe recorded samples. They are not cheating probabilities or independent evidence. Timing estimates remain limited by the recording’s sample interval.</p>
    ${metrics ? `<div class="table-wrap"><table><thead><tr><th>Measurement</th><th>Value</th><th>Observations</th><th>Source</th></tr></thead><tbody>${metrics}</tbody></table></div>` : '<p>No eligible context measurements were available.</p>'}
    <h4>Excluded observations</h4>
    ${exclusions ? `<div class="table-wrap"><table><thead><tr><th>Reason</th><th>Observations</th></tr></thead><tbody>${exclusions}</tbody></table></div>` : '<p>No exclusion counts were recorded.</p>'}
    <h4>Neutral review clips (${h(review.clips.length)})</h4>
    ${clips || '<p>No neutral clips were selected. This does not establish legitimate play.</p>'}
  </article>`;
}

export function reportHTML(match: MatchDetail) {
  const h = escapeHTML;
  const rows = match.players.map(p => `<tr><td>${h(p.name)}</td><td>${h(p.team)}</td><td>${h(p.kills)} / ${h(p.deaths)}</td><td>${h(p.verdict)}</td><td>${h(p.evidenceCount)}</td><td>${h(p.coverage)}</td></tr>`).join('');
  const findings = match.findings.map(f => `<article><div class="meta">ROUND ${h(f.round)} · TICK ${h(f.tick)} · ${h(match.players.find(p => p.id === f.playerId)?.name || f.playerId)} · ${h(f.signal)}</div><h3>${h(f.title)}</h3><p>${h(f.description)}</p><ul>${f.measurements.map(m => `<li>${h(m.label)}: <strong>${h(m.value)} ${h(m.unit)}</strong></li>`).join('')}</ul><p><b>Other explanations:</b> ${f.alternatives.map(h).join('; ') || 'See recording context.'}</p><p><b>Limits:</b> ${f.limitations.map(h).join('; ') || 'Experimental rule; accuracy unvalidated.'}</p></article>`).join('');
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'">
<title>CS Demo Review — ${h(match.demo.name)}</title>
<style>
body{font:15px/1.6 system-ui,sans-serif;color:#182334;background:#f3f5f8;max-width:1100px;margin:48px auto;padding:0 28px}h1{font-size:34px;letter-spacing:-1px}h2{margin-top:36px}h3{margin:8px 0;font-size:21px}h4{font-size:16px;margin:24px 0 8px}h5{font-size:16px;margin:8px 0}.eyebrow,.meta{font-size:12px;letter-spacing:.5px;color:#657285}.notice{padding:18px;background:#fff3d8;border-left:3px solid #d99722}.table-wrap{overflow:auto}table{width:100%;border-collapse:collapse;background:white}td,th{text-align:left;padding:12px;border-bottom:1px solid #e0e6ec;font-size:14px}tbody th{font-weight:400}article{background:white;border:1px solid #dce3eb;padding:22px;margin:16px 0;border-radius:8px}.muted{color:#657285}.metric-signal{display:block;color:#657285;font-size:12px;margin-top:3px}.identifiers{font-size:13px}code{word-break:break-all}.review-counts{display:flex;flex-wrap:wrap;gap:12px;margin:18px 0}.review-counts div{flex:1 1 140px;background:#f3f6fa;padding:12px}.review-counts dt{font-size:12px;color:#657285}.review-counts dd{font-size:20px;font-weight:600;margin:3px 0 0}.review-clip{border-left:3px solid #718eae;background:#f4f7fb;padding:16px 20px;margin:12px 0}.neutral-label{color:#355572;font-size:14px;font-weight:600}.review-clip p{margin:8px 0}li{overflow-wrap:anywhere}@media print{body{margin:0;max-width:none;background:white}article,.review-clip{break-inside:avoid}.player-review{break-inside:auto}.table-wrap{overflow:visible}}
</style></head><body>
<div class="eyebrow">CS DEMO REVIEW / EVIDENCE REPORT</div><h1>${h(match.demo.name)}</h1>
<p>${h(match.demo.engine.toUpperCase())} · ${h(match.demo.map)} · ${h(match.demo.roundCount)} rounds · ${h(match.demo.date)}</p>
<div class="notice">Experimental assessments describe signals in this recording. They are not proof of cheating, a probability of cheating, or a guarantee of legitimate play. Missing data is not evidence of innocence.</div>
<p class="muted identifiers">Parser ${h(match.demo.parserVersion)} · Analysis ${h(match.demo.analysisVersion)} · Recording ${h(match.demo.recordingType)}<br>Demo ID: <code>${h(match.demo.id)}</code><br>Demo SHA-256: <code>${h(match.demo.hash)}</code></p>
<h2>Players</h2><div class="table-wrap"><table><thead><tr><th>Player</th><th>Team</th><th>K / D</th><th>Assessment</th><th>Findings</th><th>Coverage</th></tr></thead><tbody>${rows}</tbody></table></div>
<h2>Player review summaries</h2><p>Descriptive measurements and selected moments explain what the analysis could inspect. Neutral clips are manual review bookmarks; they do not add findings or change a player’s assessment.</p>
${match.players.map(playerReviewHTML).join('') || '<p>No player review data.</p>'}
<h2>Evidence (${h(match.findings.length)})</h2>${findings || '<p>No qualifying evidence found. Check each player’s data coverage before interpreting this result.</p>'}
<h2>Signal coverage</h2><p>Measured observations may support review even when a detector cannot assess them. Detector eligibility is reported separately; neither count is a count of suspicious findings. Reviewed with limits means a useful review is available while some signal families remain unassessed.</p>${match.players.map(p => `<article><h3>${h(p.name)}</h3><ul>${p.capabilities.map(c => `<li><b>${h(signalLabel[c.signal])}</b> — ${h(capabilityLabel[c.status])}<br>${c.measuredSamples != null ? `${h(c.measuredSamples)} measured observations · ` : ''}${h(c.samples)} detector-eligible observations<br>${h(c.reason)}${c.basis ? `<br><span class="muted">Source: ${h(c.basis)}</span>` : ''}</li>`).join('')}</ul></article>`).join('')}
<h2>Review notes</h2>${match.notes.map(n => `<article><div class="meta">${h(n.kind)} · TICK ${h(n.tick)}</div><p>${h(n.text)}</p></article>`).join('') || '<p>No review notes.</p>'}
<h2>Recording warnings</h2><ul>${match.demo.warnings.map(w => `<li>${h(w)}</li>`).join('') || '<li>No parser warnings recorded.</li>'}</ul>
<p class="muted">Exported ${h(new Date().toISOString())}. All analysis performed locally.</p>
</body></html>`;
}
