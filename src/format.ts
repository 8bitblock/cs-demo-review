import type { Verdict, Signal } from '../shared/types';
export const time = (seconds: number) => `${Math.floor(Math.max(0, seconds) / 60).toString().padStart(2, '0')}:${Math.floor(Math.max(0, seconds) % 60).toString().padStart(2, '0')}`;
export const size = (bytes: number) => bytes >= 1024 ** 3 ? `${(bytes / 1024 ** 3).toFixed(1)} GB` : `${Math.round(bytes / 1024 ** 2)} MB`;
export const verdictClass = (verdict: Verdict) => verdict === 'Highly suspicious' ? 'danger' : verdict === 'Suspicious' ? 'warning' : verdict === 'Low concern' ? 'positive' : verdict === 'Reviewed with limits' ? 'measured' : 'neutral';
export const measurementValue = (value: number) => Number.isFinite(value) ? new Intl.NumberFormat(undefined, { maximumFractionDigits: 3 }).format(value) : '—';
export const signalLabel: Record<Signal, string> = { reaction: 'Visibility reaction', acquisition: 'Crosshair acquisition', 'aim-snap': 'Aim transitions', recoil: 'Recoil compensation', 'shot-direction': 'Shot direction' };
export const playerColor = (team: string) => /^(ct|counter[- ]?terrorists?|3)$/i.test(team) ? '#6aaef8' : /^(t|terrorists?|2)$/i.test(team) ? '#e5bd70' : '#94a2b4';
export const errorMessage = (error: unknown) => error instanceof Error ? error.message.replace(/^Error invoking remote method '[^']+': (Error: )?/, '') : String(error);
