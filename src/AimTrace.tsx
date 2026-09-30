import { useMemo } from 'react';
import { Activity, Crosshair } from 'lucide-react';
import type { ReplayWindow } from '../shared/types';
import { buildAimTrace } from './aim-trace-data';
import { time } from './format';

interface Props { replay: ReplayWindow | null; playerId: string; playerName: string; tick: number; tickRate: number; onSeek: (tick: number) => void }
export function AimTrace({ replay, playerId, playerName, tick, tickRate, onSeek }: Props) {
  const trace = useMemo(() => buildAimTrace(replay?.samples ?? [], playerId, tickRate), [replay, playerId, tickRate]);
  const from = replay?.fromTick ?? 0, to = Math.max(from + 1, replay?.toTick ?? 1);
  const shots = useMemo(() => (replay?.shots ?? []).filter(s => s.playerId === playerId), [replay, playerId]);
  const ceiling = Math.max(10, Math.ceil(trace.maxSpeed / 50) * 50);
  const x = (value: number) => 37 + (value - from) / (to - from) * 944;
  const y = (value: number) => 77 - value / ceiling * 63;
  const pathFor = (field: 'yawSpeed' | 'pitchSpeed') => trace.segments.map(segment => segment.map((point, index) => `${index ? 'L' : 'M'}${x(point.tick).toFixed(2)},${y(point[field]).toFixed(2)}`).join(' ')).join(' ');
  const recordedRecoil = shots.filter(s => s.aimPunch && Number.isFinite(s.aimPunch.x) && Number.isFinite(s.aimPunch.y)).length;
  const recordedImpacts = shots.filter(s => s.impacts?.some(impact => [impact.x, impact.y, impact.z].every(Number.isFinite))).length;
  const unavailable = !replay ? 'Loading the replay window…' : !playerId ? 'Select a player to inspect recorded aim samples.' : trace.sampleCount < 2 ? 'Too few recorded samples in this window.' : 'No continuous alive-player samples in this window.';
  return <div className="aim-trace-panel">
    <div className="aim-trace-heading"><span><Activity size={12} /><strong>{playerName || 'Selected player'}</strong><span>Recorded angular speed</span></span><div className="aim-trace-legend"><span><i className="yaw" />Yaw</span><span><i className="pitch" />Pitch</span><span><i className="shot" />Shot</span><span>°/s</span></div></div>
    <div className="aim-trace-plot">
      <svg viewBox="0 0 1000 99" preserveAspectRatio="none" role="slider" tabIndex={0} aria-label={`Aim trace for ${playerName || 'selected player'}. Click or use arrow keys to seek.`} aria-valuemin={from} aria-valuemax={to} aria-valuenow={Math.min(to, Math.max(from, Math.round(tick)))} aria-valuetext={`Tick ${Math.round(tick)}, ${time(tick / tickRate)}`} onClick={event => { const rect = event.currentTarget.getBoundingClientRect(); const coordinate = (event.clientX - rect.left) / rect.width * 1000; onSeek(Math.round(Math.max(from, Math.min(to, from + (coordinate - 37) / 944 * (to - from))))); }} onKeyDown={event => { const movement = event.shiftKey ? Math.round(tickRate * 5) : 1; if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) { event.preventDefault(); event.stopPropagation(); onSeek(event.key === 'Home' ? from : event.key === 'End' ? to : Math.max(from, Math.min(to, Math.round(tick) + (event.key === 'ArrowRight' ? movement : -movement)))); } }}>
        {[0, .5, 1].map(ratio => <g key={ratio}><line x1="37" y1={77 - ratio * 63} x2="981" y2={77 - ratio * 63} className="trace-grid-line" /><text x="29" y={80 - ratio * 63} textAnchor="end">{Math.round(ceiling * ratio)}</text></g>)}
        {shots.map(shot => <g key={shot.id}><title>{`${shot.weapon} · ${time(shot.time)} · tick ${shot.tick}\n${shot.impacts?.length ?? 0} recorded impacts${shot.ambiguous ? ' · ambiguous shot association' : ''}\nSource: ${shot.provenance || 'not recorded'}`}</title><line x1={x(shot.tick)} x2={x(shot.tick)} y1="11" y2="77" className="trace-shot-line" /><circle cx={x(shot.tick)} cy="8" r="2.1" className="trace-shot-dot" /></g>)}
        <path d={pathFor('yawSpeed')} className="trace-yaw-line" /><path d={pathFor('pitchSpeed')} className="trace-pitch-line" />
        {trace.segments.filter(segment => segment.length === 1).map((segment, index) => <g key={index}><circle cx={x(segment[0].tick)} cy={y(segment[0].yawSpeed)} r="1.5" className="trace-yaw-dot" /><circle cx={x(segment[0].tick)} cy={y(segment[0].pitchSpeed)} r="1.5" className="trace-pitch-dot" /></g>)}
        {tick >= from && tick <= to && <line x1={x(tick)} x2={x(tick)} y1="5" y2="80" className="trace-cursor" />}
        {[0, .25, .5, .75, 1].map(ratio => <text key={ratio} x={37 + ratio * 944} y="94" textAnchor={ratio === 0 ? 'start' : ratio === 1 ? 'end' : 'middle'}>{time((from + ratio * (to - from)) / tickRate)}</text>)}
      </svg>
      {!trace.segments.length && <div className="trace-empty"><Crosshair size={15} /><span>{unavailable}</span></div>}
    </div>
    <div className="aim-trace-caption"><span title={`Largest recorded frame interval: ${trace.maxIntervalMs?.toFixed(2) ?? 'unavailable'} ms. Angular speed is derived from adjacent original samples, not interpolated playback.`}>{trace.sampleCount.toLocaleString()} samples · {trace.intervalMs == null ? 'interval unavailable' : `${trace.intervalMs.toFixed(2)} ms median interval`}</span><span title="Breaks are intervals over 250 ms, duplicate or invalid ticks. Omitted intervals include dead players, invalid values, and position jumps over 256 game units.">{trace.gaps} sampling {trace.gaps === 1 ? 'gap' : 'gaps'} · {trace.omitted} omitted intervals</span><span className="trace-recoil-caption" title="Only fields present on recorded shots are counted. Their source and detector eligibility are shown in the player inspector.">{recordedRecoil}/{shots.length} shots with recoil fields</span><span title="Impact endpoints support measured shot direction context. They cannot establish an in-flight change in trajectory.">{recordedImpacts}/{shots.length} shots with impacts</span></div>
  </div>;
}
