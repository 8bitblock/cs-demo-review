import type { Sample } from '../shared/types';

export interface AimTracePoint { tick: number; yawSpeed: number; pitchSpeed: number; intervalMs: number }
export interface AimTraceData { segments: AimTracePoint[][]; sampleCount: number; intervalMs: number | null; maxIntervalMs: number | null; gaps: number; omitted: number; maxSpeed: number }

/** Descriptive sample differences only. Never substitutes zeros for missing evidence. */
export function buildAimTrace(samples: Sample[], playerId: string, tickRate: number): AimTraceData {
  const data: AimTraceData = { segments: [], sampleCount: 0, intervalMs: null, maxIntervalMs: null, gaps: 0, omitted: 0, maxSpeed: 0 };
  if (!Number.isFinite(tickRate) || tickRate <= 0) return data;
  const ordered = samples.filter(s => s.playerId === playerId).sort((a, b) => a.tick - b.tick);
  data.sampleCount = ordered.length;
  const intervals: number[] = [];
  let segment: AimTracePoint[] = [];
  const endSegment = () => { if (segment.length) { data.segments.push(segment); segment = []; } };
  for (let i = 1; i < ordered.length; i++) {
    const a = ordered[i - 1], b = ordered[i];
    const delta = (b.tick - a.tick) / tickRate;
    if (delta > 0 && Number.isFinite(delta)) intervals.push(delta * 1000);
    if (!(delta > 0) || delta > .25 || !Number.isFinite(delta)) { endSegment(); data.gaps++; continue; }
    const finite = [a.yaw, b.yaw, a.pitch, b.pitch, a.x, a.y, a.z, b.x, b.y, b.z].every(Number.isFinite);
    const teleported = Math.hypot(b.x - a.x, b.y - a.y, b.z - a.z) > 256;
    if (!finite || !a.alive || !b.alive || teleported) { endSegment(); data.omitted++; continue; }
    const yawDelta = ((b.yaw - a.yaw + 180) % 360 + 360) % 360 - 180;
    const point = { tick: b.tick, yawSpeed: Math.abs(yawDelta) / delta, pitchSpeed: Math.abs(b.pitch - a.pitch) / delta, intervalMs: delta * 1000 };
    data.maxSpeed = Math.max(data.maxSpeed, point.yawSpeed, point.pitchSpeed);
    segment.push(point);
  }
  endSegment();
  if (intervals.length) { intervals.sort((a, b) => a - b); const middle = Math.floor(intervals.length / 2); data.intervalMs = intervals.length % 2 ? intervals[middle] : (intervals[middle - 1] + intervals[middle]) / 2; data.maxIntervalMs = intervals[intervals.length - 1]; }
  return data;
}
