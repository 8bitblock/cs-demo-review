import { useEffect, useMemo, useRef, useState } from 'react';
import { Crosshair, LocateFixed, Minus, Plus } from 'lucide-react';
import type { GameEvent, MapAsset, Player, ReplayWindow, Sample } from '../shared/types';
import { playerColor } from './format';

export interface MapOverlays { vision: boolean; shots: boolean; deaths: boolean; utility: boolean; labels: boolean }
interface Props { replay: ReplayWindow | null; tick: number; tickRate: number; players: Player[]; selectedId: string; follow: boolean; onSelect: (id: string) => void; map: MapAsset | null; floor: number; overlays: MapOverlays; imageRevision?: number; onImageError?: () => void }

/** Position interpolation is only a display aid. Original samples feed analysis. */
export function samplesAtTick(samples: Sample[], tick: number, tickRate: number): Sample[] {
  const before = new Map<string, Sample>(); const after = new Map<string, Sample>();
  for (const sample of samples) {
    if (sample.tick <= tick && (!before.has(sample.playerId) || before.get(sample.playerId)!.tick < sample.tick)) before.set(sample.playerId, sample);
    if (sample.tick > tick && (!after.has(sample.playerId) || after.get(sample.playerId)!.tick > sample.tick)) after.set(sample.playerId, sample);
  }
  return [...before.values()].filter(s => tick - s.tick <= Math.max(tickRate, 32)).map(s => {
    const next = after.get(s.playerId);
    if (!next || next.tick - s.tick > Math.max(tickRate / 4, 8) || !s.alive || !next.alive || Math.hypot(next.x - s.x, next.y - s.y, next.z - s.z) > 128) return s;
    const ratio = (tick - s.tick) / (next.tick - s.tick);
    const deltaYaw = ((next.yaw - s.yaw + 540) % 360) - 180;
    return { ...s, x: s.x + (next.x - s.x) * ratio, y: s.y + (next.y - s.y) * ratio, z: s.z + (next.z - s.z) * ratio, yaw: s.yaw + deltaYaw * ratio };
  });
}

export function ReplayMap({ replay, tick, tickRate, players, selectedId, follow, onSelect, map, floor, overlays, imageRevision = 0, onImageError }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const hostRef = useRef<HTMLDivElement>(null);
  const hitRef = useRef<{ id: string; x: number; y: number }[]>([]);
  const [dimensions, setDimensions] = useState({ width: 700, height: 440 });
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const drag = useRef<{ x: number; y: number; panX: number; panY: number; moved: boolean } | null>(null);
  const onImageErrorRef = useRef(onImageError); onImageErrorRef.current = onImageError;
  const [radar, setRadar] = useState<HTMLImageElement | null>(null);
  const [radarBounds, setRadarBounds] = useState<{ x: number; y: number; width: number; height: number } | null>(null);
  const [imageFailed, setImageFailed] = useState(false);
  const samples = useMemo(() => samplesAtTick(replay?.samples ?? [], tick, tickRate), [replay, tick, tickRate]);
  const names = useMemo(() => new Map(players.map(p => [p.id, p.name])), [players]);
  const image = map?.floors?.[floor]?.image || map?.image;
  useEffect(() => { setZoom(1); setPan({ x: 0, y: 0 }); }, [map?.id, floor]);
  useEffect(() => { setPan({ x: 0, y: 0 }); }, [follow, selectedId]);
  useEffect(() => {
    const element = hostRef.current; if (!element) return;
    const observer = new ResizeObserver(entries => { const r = entries[0].contentRect; setDimensions({ width: r.width, height: r.height }); });
    observer.observe(element); return () => observer.disconnect();
  }, []);
  useEffect(() => {
    setRadar(null); setRadarBounds(null); setImageFailed(false); if (!image) return;
    let cancelled = false; const img = new Image(); img.crossOrigin = 'anonymous';
    img.onload = () => {
      if (cancelled) return;
      setRadar(img);
      // Extracted radars often have large transparent borders. Fit their visible
      // area while retaining the original pixel transform for player positions.
      try {
        const preview = document.createElement('canvas'); preview.width = 256; preview.height = 256;
        const context = preview.getContext('2d', { willReadFrequently: true }); if (!context) return;
        context.drawImage(img, 0, 0, 256, 256);
        const bounds = visibleRadarBounds(context.getImageData(0, 0, 256, 256).data, 256, 256);
        if (bounds) setRadarBounds({ x: bounds.x * img.naturalWidth / 256, y: bounds.y * img.naturalHeight / 256, width: bounds.width * img.naturalWidth / 256, height: bounds.height * img.naturalHeight / 256 });
      } catch { /* Images without readable pixels still render at their full size. */ }
    };
    img.onerror = () => { if (!cancelled) { setImageFailed(true); onImageErrorRef.current?.(); } };
    img.src = image;
    return () => { cancelled = true; };
  }, [image, imageRevision]);

  const bounds = useMemo(() => {
    const all = replay?.samples.filter(s => Number.isFinite(s.x) && Number.isFinite(s.y) && (s.x !== 0 || s.y !== 0)) ?? [];
    if (!all.length) return { minX: -3000, maxX: 3000, minY: -3000, maxY: 3000 };
    let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
    for (const s of all) { minX = Math.min(minX, s.x); maxX = Math.max(maxX, s.x); minY = Math.min(minY, s.y); maxY = Math.max(maxY, s.y); }
    return { minX: Math.floor((minX - 450) / 1000) * 1000, maxX: Math.ceil((maxX + 450) / 1000) * 1000, minY: Math.floor((minY - 450) / 1000) * 1000, maxY: Math.ceil((maxY + 450) / 1000) * 1000 };
  }, [replay]);

  useEffect(() => {
    const canvas = canvasRef.current; if (!canvas) return;
    const ctx = canvas.getContext('2d'); if (!ctx) return;
    const { width, height } = dimensions; if (width < 1 || height < 1) return; const dpr = window.devicePixelRatio || 1;
    canvas.width = Math.round(width * dpr); canvas.height = Math.round(height * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0); ctx.clearRect(0, 0, width, height);
    const imageWidth = radar?.naturalWidth ?? 1024; const imageHeight = radar?.naturalHeight ?? 1024;
    const baseScale = radar && map ? Math.max(0.001, Math.min((width - 36) / (radarBounds?.width ?? imageWidth), (height - 36) / (radarBounds?.height ?? imageHeight))) : Math.max(0.001, Math.min((width - 64) / (bounds.maxX - bounds.minX), (height - 64) / (bounds.maxY - bounds.minY)));
    const scale = baseScale * zoom;
    const selected = samples.find(s => s.playerId === selectedId);
    const rotate = map?.rotate ? map.rotate * Math.PI / 180 : 0;
    const radarPoint = (x: number, y: number) => {
      if (!map) return { x, y };
      const px = (x - map.posX) / map.scale - imageWidth / 2;
      const py = (map.posY - y) / map.scale - imageHeight / 2;
      return { x: px * Math.cos(rotate) - py * Math.sin(rotate), y: px * Math.sin(rotate) + py * Math.cos(rotate) };
    };
    const centerX = (bounds.minX + bounds.maxX) / 2; const centerY = (bounds.minY + bounds.maxY) / 2;
    const contentCenter = radarBounds ? { x: radarBounds.x + radarBounds.width / 2 - imageWidth / 2, y: radarBounds.y + radarBounds.height / 2 - imageHeight / 2 } : { x: 0, y: 0 };
    const followedPoint = selected && follow ? (radar && map ? radarPoint(selected.x, selected.y) : { x: selected.x - centerX, y: centerY - selected.y }) : { x: contentCenter.x * Math.cos(rotate) - contentCenter.y * Math.sin(rotate), y: contentCenter.x * Math.sin(rotate) + contentCenter.y * Math.cos(rotate) };
    const project = (x: number, y: number) => {
      const p = radar && map ? radarPoint(x, y) : { x: x - centerX, y: centerY - y };
      return { x: width / 2 + pan.x + (p.x - followedPoint.x) * scale, y: height / 2 + pan.y + (p.y - followedPoint.y) * scale };
    };
    ctx.fillStyle = '#0d131b'; ctx.fillRect(0, 0, width, height);
    if (radar && map) {
      ctx.save(); ctx.translate(width / 2 + pan.x - followedPoint.x * scale, height / 2 + pan.y - followedPoint.y * scale); ctx.scale(scale, scale); ctx.rotate(rotate); ctx.globalAlpha = 0.95;
      ctx.drawImage(radar, -imageWidth / 2, -imageHeight / 2); ctx.restore();
    } else {
      ctx.strokeStyle = '#1a2530'; ctx.lineWidth = 1;
      const gridStep = Math.max(500, Math.ceil(40 / scale / 500) * 500);
      ctx.font = '12px ui-monospace, monospace'; ctx.fillStyle = '#475367';
      for (let x = bounds.minX; x <= bounds.maxX; x += gridStep) { const p = project(x, 0); ctx.beginPath(); ctx.moveTo(p.x, 0); ctx.lineTo(p.x, height); ctx.stroke(); if (p.x > 16 && p.x < width - 50) ctx.fillText(String(x), p.x + 4, height - 12); }
      for (let y = bounds.minY; y <= bounds.maxY; y += gridStep) { const p = project(0, y); ctx.beginPath(); ctx.moveTo(0, p.y); ctx.lineTo(width, p.y); ctx.stroke(); if (p.y > 30 && p.y < height - 25) ctx.fillText(String(y), 12, p.y - 6); }
    }
    const chosenFloor = map?.floors?.[floor];
    const onFloor = (z: number | undefined) => !chosenFloor || z == null || (z >= chosenFloor.minZ && z <= chosenFloor.maxZ);
    if (overlays.shots) for (const shot of replay?.shots ?? []) {
      const age = (tick - shot.tick) / tickRate; if (age < 0 || age > 0.5 || !shot.origin || !onFloor(shot.origin.z)) continue;
      const a = project(shot.origin.x, shot.origin.y); ctx.strokeStyle = `rgba(239,191,100,${0.8 * (1 - age / 0.5)})`; ctx.lineWidth = 1.3;
      for (const impact of shot.impacts) { const b = project(impact.x, impact.y); ctx.beginPath(); ctx.moveTo(a.x, a.y); ctx.lineTo(b.x, b.y); ctx.stroke(); ctx.fillStyle = '#f9d58a'; ctx.fillRect(b.x - 2, b.y - 2, 4, 4); }
    }
    for (const event of replay?.events ?? []) {
      const age = (tick - event.tick) / tickRate; if (age < 0 || age > 5 || event.x == null || event.y == null || !onFloor(event.z)) continue;
      const p = project(event.x, event.y); drawEvent(ctx, event, p, overlays, age);
    }
    hitRef.current = [];
    // Draw the followed/selected player last so nearby labels cannot hide it.
    for (const s of [...samples].sort((a, b) => Number(a.playerId === selectedId) - Number(b.playerId === selectedId))) {
      if (!onFloor(s.z)) continue;
      const p = project(s.x, s.y); const color = playerColor(s.team); const isSelected = s.playerId === selectedId;
      hitRef.current.push({ id: s.playerId, ...p });
      ctx.globalAlpha = s.alive ? 1 : 0.28;
      const yaw = -s.yaw * Math.PI / 180 + (radar ? rotate : 0);
      if (s.alive && overlays.vision) {
        const coneSize = isSelected ? 62 : 34;
        const gradient = ctx.createRadialGradient(p.x, p.y, 3, p.x, p.y, coneSize); gradient.addColorStop(0, color + '38'); gradient.addColorStop(1, color + '00');
        ctx.fillStyle = gradient; ctx.beginPath(); ctx.moveTo(p.x, p.y); ctx.arc(p.x, p.y, coneSize, yaw - 0.63, yaw + 0.63); ctx.closePath(); ctx.fill();
        ctx.strokeStyle = color + '90'; ctx.lineWidth = 1; ctx.beginPath(); ctx.moveTo(p.x, p.y); ctx.lineTo(p.x + Math.cos(yaw) * 23, p.y + Math.sin(yaw) * 23); ctx.stroke();
      }
      if (isSelected) { ctx.strokeStyle = '#f3f7fa'; ctx.lineWidth = 1.5; ctx.beginPath(); ctx.arc(p.x, p.y, 13, 0, Math.PI * 2); ctx.stroke(); }
      ctx.fillStyle = color; ctx.strokeStyle = '#0b1118'; ctx.lineWidth = 2; ctx.beginPath(); ctx.arc(p.x, p.y, isSelected ? 7 : 5.5, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
      if (s.flashed) { ctx.fillStyle = '#ffffff'; ctx.beginPath(); ctx.arc(p.x, p.y, 3, 0, Math.PI * 2); ctx.fill(); }
      if (overlays.labels || isSelected) {
        const label = (names.get(s.playerId) || s.playerId).slice(0, 20); ctx.font = `${isSelected ? 600 : 500} 12px "Segoe UI", sans-serif`; ctx.textAlign = 'center'; ctx.textBaseline = 'middle';
        const labelWidth = ctx.measureText(label).width + 10; ctx.fillStyle = '#0b1118dd'; ctx.fillRect(p.x - labelWidth / 2, p.y + 15, labelWidth, 20); ctx.fillStyle = isSelected ? '#f3f7fa' : '#bac7d6'; ctx.fillText(label, p.x, p.y + 25);
      }
      ctx.globalAlpha = 1;
    }
    ctx.textAlign = 'left'; ctx.textBaseline = 'alphabetic';
  }, [dimensions, samples, bounds, tick, tickRate, map, radar, zoom, selectedId, follow, floor, overlays, replay, names, pan, radarBounds]);

  return <div className="map-surface" ref={hostRef}>
    <canvas ref={canvasRef} aria-label="Interactive overhead replay. Drag to pan, scroll to zoom, or select a player." onWheel={event => { setZoom(value => Math.max(.5, Math.min(5, value * (event.deltaY > 0 ? .9 : 1.1)))); }}
      onPointerDown={event => { if (event.button !== 0) return; drag.current = { x: event.clientX, y: event.clientY, panX: pan.x, panY: pan.y, moved: false }; event.currentTarget.setPointerCapture(event.pointerId); }}
      onPointerMove={event => { const current = drag.current; if (!current) return; const dx = event.clientX - current.x, dy = event.clientY - current.y; if (Math.hypot(dx, dy) > 4) current.moved = true; if (current.moved) setPan({ x: current.panX + dx, y: current.panY + dy }); }}
      onPointerCancel={() => { drag.current = null; }}
      onPointerUp={event => { const current = drag.current; drag.current = null; if (!current || current.moved) return; const rect = event.currentTarget.getBoundingClientRect(); const x = event.clientX - rect.left, y = event.clientY - rect.top; const hit = hitRef.current.reduce<{ id: string; distance: number } | null>((nearest, p) => { const distance = Math.hypot(p.x - x, p.y - y); return distance < 24 && (!nearest || distance < nearest.distance) ? { id: p.id, distance } : nearest; }, null); if (hit) onSelect(hit.id); }} />
    <div className="map-mode"><span className="live-dot" />{radar ? 'RADAR REPLAY' : 'COORDINATE REPLAY'}{imageFailed && <span className="map-warning"> · Radar image unavailable</span>}</div>
    <div className="map-legend"><span><i style={{ background: '#6aaef8' }} />Counter-terrorists</span><span><i style={{ background: '#e5bd70' }} />Terrorists</span></div>
    <div className="map-controls"><button className="icon-button" title="Zoom in" aria-label="Zoom in" onClick={() => setZoom(z => Math.min(z + 0.25, 4))}><Plus size={15} /></button><span>{Math.round(zoom * 100)}%</span><button className="icon-button" title="Zoom out" aria-label="Zoom out" onClick={() => setZoom(z => Math.max(z - 0.25, 0.5))}><Minus size={15} /></button><button className="icon-button" title="Reset zoom" aria-label="Reset zoom" onClick={() => { setZoom(1); setPan({ x: 0, y: 0 }); }}><LocateFixed size={15} /></button></div>
    {!samples.length && <div className="map-wait"><Crosshair size={27} /><span>{replay ? 'No positions at this tick. Press Play or choose another round.' : 'Loading replay samples…'}</span></div>}
  </div>;
}

function drawEvent(ctx: CanvasRenderingContext2D, event: GameEvent, p: { x: number; y: number }, overlays: MapOverlays, age: number) {
  ctx.save(); ctx.globalAlpha = Math.max(0.15, 1 - age / 5);
  if (/kill|death/i.test(event.kind) && overlays.deaths) { ctx.strokeStyle = '#e17979'; ctx.lineWidth = 2; ctx.beginPath(); ctx.moveTo(p.x - 5, p.y - 5); ctx.lineTo(p.x + 5, p.y + 5); ctx.moveTo(p.x + 5, p.y - 5); ctx.lineTo(p.x - 5, p.y + 5); ctx.stroke(); }
  if (/smoke|flash|grenade|inferno|molotov|bomb/i.test(event.kind) && overlays.utility) {
    ctx.strokeStyle = /smoke/i.test(event.kind) ? '#95a7b9' : /bomb/i.test(event.kind) ? '#f19778' : '#77cdb6'; ctx.fillStyle = ctx.strokeStyle + '20'; ctx.lineWidth = 1.5;
    ctx.beginPath(); ctx.arc(p.x, p.y, /smoke|inferno/i.test(event.kind) ? 18 : 6, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
  }
  ctx.restore();
}

/** Ignore transparent padding only; never infer playable geometry from the radar. */
export function visibleRadarBounds(pixels: Uint8ClampedArray, width: number, height: number) {
  let left = width, top = height, right = -1, bottom = -1;
  for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) {
    if (pixels[(y * width + x) * 4 + 3] < 32) continue;
    left = Math.min(left, x); top = Math.min(top, y); right = Math.max(right, x); bottom = Math.max(bottom, y);
  }
  if (right < left || bottom < top) return null;
  const padding = Math.ceil(Math.max(width, height) * .03);
  left = Math.max(0, left - padding); top = Math.max(0, top - padding);
  right = Math.min(width - 1, right + padding); bottom = Math.min(height - 1, bottom + padding);
  return { x: left, y: top, width: right - left + 1, height: bottom - top + 1 };
}
