import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type PointerEvent as ReactPointerEvent, type ReactNode } from 'react';
import { Activity, ArrowDownToLine, ArrowLeft, ArrowRight, Bookmark, Check, ChevronDown, ChevronLeft, ChevronRight, CircleHelp, Clock3, Copy, Crosshair, Database, Download, ExternalLink, Eye, FileCode2, FileText, FileUp, Flag, FolderOpen, Gauge, Layers3, ListFilter, LoaderCircle, Map as MapIcon, Maximize2, MessageSquare, MoreHorizontal, MousePointer2, PanelLeft, Pause, Play, Plus, Radar, RefreshCw, Search, Settings2, ShieldCheck, ShieldQuestion, SkipBack, SkipForward, SlidersHorizontal, Target, Trash2, TriangleAlert, Upload, Users, X } from 'lucide-react';
import type { AppSettings, Demo, Diagnostics, Finding, ImportProgress, MapAsset, MatchDetail, Player, ReplayWindow, ReviewNote } from '../shared/types';
import { errorMessage, measurementValue, playerColor, signalLabel, size, time, verdictClass } from './format';
import { ReplayMap, samplesAtTick, type MapOverlays } from './ReplayMap';
import { AimTrace } from './AimTrace';
import { buildAimTrace } from './aim-trace-data';
import { GameView } from './GameView';
import { SignalCoverage } from './SignalCoverage';
import { ReviewMeasurements } from './ReviewMeasurements';

const api = window.csDemo;
// A shared queue prevents StrictMode and rapid demo switching from launching
// duplicate VPK extraction processes. Completed assets remain in the worker cache.
const mapExtractions = new Map<string, Promise<MapAsset>>();
let extractionTail: Promise<unknown> = Promise.resolve();
function PlayerReviewPanel({ player, onSeek, onAnalyse, busy }: { player: Player; onSeek: (tick: number) => void; onAnalyse: () => void; busy: boolean }) {
  const review = player.review;
  const [openClip, setOpenClip] = useState('');
  useEffect(() => { setOpenClip(''); }, [player.id]);
  return <section className="player-review" aria-label="Measured player analysis">
    <div className="section-label">ANALYSIS RESULTS<button className="text-button" disabled={busy} onClick={onAnalyse}><RefreshCw size={13} />{busy ? 'Working…' : 'Refresh'}</button></div>
    {review ? <>
      <p className="review-summary">{review.summary}</p>
      <div className="review-counts"><div><strong>{review.totalShots.toLocaleString()}</strong><span>shots recorded</span></div><div><strong>{review.sampledShots.toLocaleString()}</strong><span>with aim samples</span></div><div><strong>{review.eligibleAimShots.toLocaleString()}</strong><span>usable aim shots</span></div><div><strong>{review.coveredRounds}</strong><span>rounds with shots</span></div></div>
      <p className="review-cadence">Recorded player samples: {review.medianSampleMs == null ? 'interval unavailable' : `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(review.medianSampleMs)} ms median interval`}. Timing resolution follows the recorded samples.</p>
      <ReviewMeasurements metrics={review.metrics} />
      {!!review.exclusions.length && <details className="review-exclusions"><summary>Why some shots cannot be assessed<ChevronDown size={14} /></summary><ul>{review.exclusions.map((exclusion, index) => <li key={index}><span>{exclusion.reason}</span><strong>{exclusion.count}</strong></li>)}</ul></details>}
      <div className="section-label">REVIEW MOMENTS<span>{review.clips.length}</span></div>
      <p className="review-moments-caption">Selected moments to inspect in context. These are not cheating findings.</p>
      {review.clips.length ? <div className="review-clips">{review.clips.map(clip => <article key={clip.id} className={openClip === clip.id ? 'selected' : ''}><button onClick={() => { setOpenClip(clip.id); onSeek(clip.tick); }}><span><Play size={14} />R{clip.round} · {time(clip.time)}</span><strong>{clip.title}</strong><p>{clip.description}</p></button>{openClip === clip.id && <div className="review-clip-context">{clip.measurements.map((measurement, index) => <div key={index}><span>{measurement.label}</span><strong>{measurementValue(measurement.value)} {measurement.unit}</strong></div>)}{clip.provenance && <p><b>Source:</b> {clip.provenance}</p>}{clip.limitations.map((limitation, index) => <p key={index}>{limitation}</p>)}</div>}</article>)}</div> : <p className="review-summary muted">No review moments selected for this player. See the measurements and signal coverage for the available observations.</p>}
    </> : <p className="review-summary">This recording needs the updated analysis to show shot counts, measurements, and review moments. Use Refresh above.</p>}
  </section>;
}

function extractInstalledMap(id: string): Promise<MapAsset> {
  const existing = mapExtractions.get(id); if (existing) return existing;
  const work = extractionTail.catch(() => {}).then(() => api.extractMap(id));
  mapExtractions.set(id, work); extractionTail = work;
  void work.finally(() => { mapExtractions.delete(id); }).catch(() => {});
  return work;
}
const emptySettings: AppSettings = { cs2Path: '', csgoPath: '', steamPath: '', source2ViewerPath: '', libraryPath: '' };
const activeStage = (stage: ImportProgress['stage']) => !['complete', 'error', 'cancelled'].includes(stage);
type Notice = { text: string; kind: 'success' | 'error' | 'info' };

export function App() {
  const [demos, setDemos] = useState<Demo[]>([]);
  const [detail, setDetail] = useState<MatchDetail | null>(null);
  const [selectedDemo, setSelectedDemo] = useState('');
  const [selectedPlayer, setSelectedPlayer] = useState('');
  const [selectedFinding, setSelectedFinding] = useState('');
  const [replay, setReplay] = useState<ReplayWindow | null>(null);
  const [map, setMap] = useState<MapAsset | null>(null);
  const [mapState, setMapState] = useState<'loading' | 'extracting' | 'ready' | 'error'>('loading');
  const [mapError, setMapError] = useState('');
  const [mapRevision, setMapRevision] = useState(0);
  const [reviewView, setReviewView] = useState<'map' | 'game'>('map');
  const [expandedReplay, setExpandedReplay] = useState(false);
  const [floor, setFloor] = useState(0);
  const [tick, setTick] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState(1);
  const [follow, setFollow] = useState(false);
  const [overlays, setOverlays] = useState<MapOverlays>({ vision: true, shots: true, deaths: true, utility: true, labels: true });
  const [jobs, setJobs] = useState<ImportProgress[]>([]);
  const [loading, setLoading] = useState(false);
  const [replayLoading, setReplayLoading] = useState(false);
  const [busy, setBusy] = useState('');
  const [notice, setNotice] = useState<Notice | null>(null);
  const [workerError, setWorkerError] = useState(api ? '' : 'The desktop connection is unavailable. Start the app with the desktop launcher to import and analyse demos.');
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  const [showLibrary, setShowLibrary] = useState(true);
  const [search, setSearch] = useState('');
  const [engineFilter, setEngineFilter] = useState('all');
  const [sideTab, setSideTab] = useState<'evidence' | 'notes'>('evidence');
  const [bottomTab, setBottomTab] = useState<'players' | 'events' | 'aim'>('players');
  const [leftWidth, setLeftWidth] = useState(260);
  const [rightWidth, setRightWidth] = useState(365);
  const [bottomHeight, setBottomHeight] = useState(278);
  const [dragging, setDragging] = useState(false);
  const searchRef = useRef<HTMLInputElement>(null);
  const inspectorRef = useRef<HTMLDivElement>(null);
  const dragCount = useRef(0);
  const completedImports = useRef(new Set<string>());
  const currentDemoRef = useRef(selectedDemo); currentDemoRef.current = selectedDemo;
  const noticeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const notify = useCallback((text: string, kind: Notice['kind'] = 'info') => { setNotice({ text, kind }); if (noticeTimer.current) clearTimeout(noticeTimer.current); noticeTimer.current = setTimeout(() => setNotice(null), kind === 'error' ? 14000 : 6500); }, []);
  const perform = useCallback(async (action: () => Promise<unknown>, success?: string) => { try { await action(); if (success) notify(success, 'success'); } catch (error) { notify(errorMessage(error), 'error'); } }, [notify]);
  const refreshLibrary = useCallback(async () => { if (!api) return; try { const list = await api.listDemos(); setDemos(list); return list; } catch (error) { setWorkerError(errorMessage(error)); } }, []);
  useEffect(() => {
    let disposed = false;
    refreshLibrary().then(list => { if (!disposed && list?.length && !currentDemoRef.current) setSelectedDemo(list[0].id); });
    if (!api) return;
    const unsubImport = api.onImportProgress(progress => {
      if (disposed) return;
      if (progress.stage === 'complete') { if (completedImports.current.has(progress.jobId)) return; completedImports.current.add(progress.jobId); }
      setJobs(previous => progress.jobId.startsWith('refresh-') && progress.stage === 'complete' ? previous.filter(job => job.jobId !== progress.jobId) : [...previous.filter(job => job.jobId !== progress.jobId), progress]);
      if (progress.stage === 'complete') {
        void refreshLibrary();
        if (progress.demoId === currentDemoRef.current) {
          const id = progress.demoId;
          void api.getMatch(id).then(match => { if (!disposed && currentDemoRef.current === id) setDetail(match); }).catch(error => notify(errorMessage(error), 'error'));
        } else if (progress.demoId && (!progress.jobId.startsWith('refresh-') || !currentDemoRef.current)) setSelectedDemo(progress.demoId);
      }
      if (progress.stage === 'error') notify(`${progress.name}: ${progress.message}`, 'error');
    });
    const unsubWorker = api.onWorkerError(message => { setWorkerError(message); setPlaying(false); });
    return () => { disposed = true; unsubImport(); unsubWorker(); };
  }, [refreshLibrary, notify]);
  useEffect(() => {
    if (!selectedDemo || !api) { setDetail(null); setReplay(null); return; }
    let stale = false; setLoading(true); setPlaying(false); setDetail(null); setReplay(null); setMap(null); setMapState('loading'); setMapError(''); setFloor(0); setSelectedFinding('');
    api.getMatch(selectedDemo).then(match => { if (stale) return; setDetail(match); setSelectedPlayer(match.players[0]?.id ?? ''); setTick(match.rounds[0]?.startTick ?? 0); setWorkerError(''); }).catch(error => { if (!stale) notify(errorMessage(error), 'error'); }).finally(() => { if (!stale) setLoading(false); });
    api.getMap(selectedDemo).then(async asset => {
      if (stale) return;
      setMap(asset);
      if (asset?.image || asset?.floors?.some(item => item.image)) { setMapState('ready'); return; }
      setMapState('extracting');
      const extracted = await extractInstalledMap(selectedDemo);
      if (stale) return;
      setMap(extracted); setMapRevision(value => value + 1);
      if (!extracted.image && !extracted.floors?.some(item => item.image)) throw new Error('The installed map does not contain a readable radar. Import a matching map pack or check the game installation.');
      setMapState('ready');
    }).catch(error => { if (!stale) { setMapState('error'); setMapError(errorMessage(error)); } });
    return () => { stale = true; };
  }, [selectedDemo, notify]);

  const totalTicks = detail?.demo.totalTicks ?? 0;
  const tickRate = detail?.demo.tickRate || 64;
  const windowBucket = Math.floor(tick / 1024);
  useEffect(() => {
    if (!detail || !api) return;
    let stale = false; setReplayLoading(true); setReplay(null);
    const from = Math.max(0, windowBucket * 1024 - 128), to = Math.min(totalTicks, from + 2176);
    api.getReplay(detail.demo.id, from, to).then(result => { if (!stale) setReplay(result); }).catch(error => { if (!stale) { setPlaying(false); notify(errorMessage(error), 'error'); } }).finally(() => { if (!stale) setReplayLoading(false); });
    return () => { stale = true; };
  }, [detail?.demo.id, windowBucket, totalTicks, notify]);
  useEffect(() => {
    if (!playing || !detail || reviewView !== 'map') return;
    let frame = 0, last = performance.now(), accumulated = 0;
    const advance = (now: number) => {
      accumulated += Math.min(now - last, 200); last = now;
      if (accumulated >= 30) { const delta = accumulated / 1000 * tickRate * speed; accumulated = 0; setTick(previous => { const next = Math.min(totalTicks, previous + delta); if (next >= totalTicks) setPlaying(false); return next; }); }
      frame = requestAnimationFrame(advance);
    };
    frame = requestAnimationFrame(advance); return () => cancelAnimationFrame(frame);
  }, [playing, speed, tickRate, totalTicks, detail?.demo.id, reviewView]);
  const seek = useCallback((nextTick: number) => { setTick(Math.max(0, Math.min(totalTicks, nextTick))); setPlaying(false); }, [totalTicks]);
  useEffect(() => { if (settingsOpen || helpOpen) setPlaying(false); }, [settingsOpen, helpOpen]);
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement;
      if (target.matches('input, textarea, select, button, [contenteditable="true"]') || settingsOpen || helpOpen) return;
      if (event.key === '/') { event.preventDefault(); setShowLibrary(true); requestAnimationFrame(() => searchRef.current?.focus()); return; }
      if (!detail || reviewView !== 'map') return;
      if (event.code === 'Space') { event.preventDefault(); setPlaying(value => !value); }
      if (event.code === 'ArrowLeft') { event.preventDefault(); setPlaying(false); setTick(value => Math.max(0, value - (event.shiftKey ? tickRate * 5 : 1))); }
      if (event.code === 'ArrowRight') { event.preventDefault(); setPlaying(false); setTick(value => Math.min(totalTicks, value + (event.shiftKey ? tickRate * 5 : 1))); }
    };
    window.addEventListener('keydown', keydown); return () => window.removeEventListener('keydown', keydown);
  }, [detail, settingsOpen, helpOpen, tickRate, totalTicks, reviewView]);
  const importDemos = () => perform(async () => { if (!api) throw new Error('Open the desktop app to import demos.'); await api.importDemos(); });
  const player = detail?.players.find(p => p.id === selectedPlayer) ?? null;
  const finding = detail?.findings.find(f => f.id === selectedFinding) ?? null;
  const playerFindings = detail?.findings.filter(f => f.playerId === selectedPlayer) ?? [];
  const round = detail?.rounds.find(r => tick >= r.startTick && tick <= r.endTick) ?? detail?.rounds.filter(r => r.startTick <= tick).at(-1);
  const currentSamples = useMemo(() => samplesAtTick(replay?.samples ?? [], tick, tickRate), [replay, tick, tickRate]);
  const selectedSample = currentSamples.find(s => s.playerId === selectedPlayer);
  const filteredDemos = demos.filter(d => (engineFilter === 'all' || d.engine === engineFilter) && `${d.name} ${d.map}`.toLowerCase().includes(search.toLowerCase()));
  const activeJobs = jobs.filter(j => activeStage(j.stage));
  const selectPlayer = (id: string) => { setSelectedPlayer(id); setSelectedFinding(''); setSideTab('evidence'); inspectorRef.current?.scrollTo({ top: 0 }); };
  const selectFinding = (item: Finding) => { setSelectedPlayer(item.playerId); setSelectedFinding(item.id); seek(item.tick); setSideTab('evidence'); };
  const mutateNotes = (note: ReviewNote) => setDetail(current => current ? { ...current, notes: [...current.notes.filter(n => n.id !== note.id), note] } : current);
  const bookmark = () => perform(async () => { if (!detail) return; const note = await api.saveNote({ demoId: detail.demo.id, tick: Math.round(tick), playerId: selectedPlayer, kind: 'bookmark', text: `Round ${round?.number ?? '—'} · ${time(tick / tickRate)}` }); mutateNotes(note); setSideTab('notes'); }, 'Bookmark saved');
  const runMapAction = (kind: 'import' | 'extract' | 'analyse') => perform(async () => {
    if (!detail) return; const id = detail.demo.id; setBusy(kind);
    try {
      if (kind === 'import') {
        const result = await api.importMapPack();
        if (result && currentDemoRef.current === id) {
          const asset = await api.getMap(id); setMap(asset); setMapError(''); setMapState(asset?.image ? 'ready' : 'error');
          if (!asset?.image) setMapError('The imported pack does not include a radar for this recording.');
          setMapRevision(value => value + 1);
        }
      }
      if (kind === 'extract') {
        setMapState('extracting'); setMapError('');
        try {
          const asset = await extractInstalledMap(id);
          if (currentDemoRef.current !== id) return;
          setMap(asset); setMapRevision(value => value + 1);
          if (!asset.image && !asset.floors?.some(item => item.image)) throw new Error('No readable radar was found. Check your game installation or import a historical map pack.');
          setMapState('ready');
        } catch (error) { if (currentDemoRef.current === id) { setMapState('error'); setMapError(errorMessage(error)); } throw error; }
      }
      if (kind === 'analyse') {
        await api.reanalyse(id); const refreshed = await api.getMatch(id);
        if (currentDemoRef.current === id) setDetail(refreshed);
        await refreshLibrary();
      }
    } finally { setBusy(''); }
  }, kind === 'analyse' ? 'Analysis refreshed' : undefined);
  const removeDemo = (demo: Demo) => perform(async () => { const removed = await api.removeDemo(demo.id); if (!removed) return; if (selectedDemo === demo.id) setSelectedDemo(demos.find(d => d.id !== demo.id)?.id ?? ''); await refreshLibrary(); notify('Removed from library. Original file preserved.', 'success'); });
  const resize = (side: 'left' | 'right' | 'bottom', event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault(); const startX = event.clientX, startY = event.clientY; const initial = side === 'left' ? leftWidth : side === 'right' ? rightWidth : bottomHeight;
    const move = (e: PointerEvent) => { if (side === 'left') setLeftWidth(Math.max(205, Math.min(350, initial + e.clientX - startX))); else if (side === 'right') setRightWidth(Math.max(280, Math.min(480, initial - e.clientX + startX))); else setBottomHeight(Math.max(205, Math.min(window.innerHeight * .55, initial - e.clientY + startY))); };
    const stop = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', stop); document.body.style.cursor = ''; document.body.style.userSelect = ''; };
    document.body.style.cursor = side === 'bottom' ? 'row-resize' : 'col-resize'; document.body.style.userSelect = 'none'; window.addEventListener('pointermove', move); window.addEventListener('pointerup', stop);
  };

  return <div className="app" style={{ '--library-width': `${showLibrary ? leftWidth : 0}px`, '--evidence-width': `${rightWidth}px`, '--bottom-height': `${bottomHeight}px` } as CSSProperties}
    onDragEnter={e => { if (e.dataTransfer.types.includes('Files')) { e.preventDefault(); dragCount.current++; setDragging(true); } }} onDragOver={e => { if (e.dataTransfer.types.includes('Files')) { e.preventDefault(); e.dataTransfer.dropEffect = 'copy'; } }} onDragLeave={e => { e.preventDefault(); dragCount.current = Math.max(0, dragCount.current - 1); if (!dragCount.current) setDragging(false); }} onDrop={e => { e.preventDefault(); dragCount.current = 0; setDragging(false); const files = Array.from(e.dataTransfer.files); if (files.length) void perform(async () => { if (!api) throw new Error('Open the desktop app to import demos.'); await api.importDropped(files); }); }}>
    <header className="app-header">
      <div className="brand"><div className="brand-icon"><Crosshair size={21} strokeWidth={1.7} /></div><div><strong>CS <span>DEMO REVIEW</span></strong><span className="brand-sub">COUNTER-STRIKE ANALYSIS WORKSPACE</span></div><span className="beta-badge">BETA</span></div>
      <div className="header-actions"><span className="local-badge"><span className="live-dot" />Local & private</span><button className="icon-button" title="How evidence is assessed" aria-label="How evidence is assessed" onClick={() => setHelpOpen(true)}><CircleHelp size={17} /></button><button className="icon-button" title="Settings" aria-label="Settings" onClick={() => setSettingsOpen(true)}><Settings2 size={17} /></button><span className="divider" /><button className="button primary" onClick={() => void importDemos()}><Plus size={16} />Import demo</button></div>
    </header>
    {workerError && <div className="connection-banner" role="alert"><TriangleAlert size={15} /><span>{workerError}</span><button onClick={() => { setWorkerError(''); void refreshLibrary(); }}>Retry connection</button><button className="icon-button" title="Dismiss" onClick={() => setWorkerError('')}><X size={14} /></button></div>}
    <div className={`workspace ${showLibrary ? '' : 'library-hidden'} ${detail ? 'has-match' : 'no-match'}`}>
      {showLibrary && <aside className="library-panel">
        <div className="panel-heading"><span><Database size={15} />Demo library</span><span className="count-badge">{demos.length}</span><button className="icon-button" title="Hide library" aria-label="Hide library" onClick={() => setShowLibrary(false)}><PanelLeft size={15} /></button></div>
        <label className="search-field"><Search size={14} /><input ref={searchRef} value={search} onChange={e => setSearch(e.target.value)} placeholder="Search demos or maps…" aria-label="Search demos or maps" /><kbd>/</kbd></label>
        <div className="library-filter"><div className="segmented"><button className={engineFilter === 'all' ? 'active' : ''} onClick={() => setEngineFilter('all')}>All</button><button className={engineFilter === 'cs2' ? 'active' : ''} onClick={() => setEngineFilter('cs2')}>CS2</button><button className={engineFilter === 'csgo' ? 'active' : ''} onClick={() => setEngineFilter('csgo')}>CS:GO</button></div><ListFilter size={14} aria-hidden="true" /></div>
        <div className="library-list">
          {filteredDemos.length ? <><div className="eyebrow library-group">YOUR RECORDINGS</div>{filteredDemos.map(d => <div key={d.id} className={`demo-card ${selectedDemo === d.id ? 'selected' : ''}`}><button className="demo-select" onClick={() => setSelectedDemo(d.id)}><div className="demo-thumb"><MapIcon size={22} /><span>{d.map.replace(/^de_/, '').slice(0, 3).toUpperCase()}</span></div><div className="demo-card-copy"><div className="demo-map">{d.map || 'Unknown map'}<span className={`engine-tag ${d.engine}`}>{d.engine === 'cs2' ? 'CS2' : 'GO'}</span></div><div className="demo-filename" title={d.name}>{d.name}</div><div className="demo-meta"><span>{time(d.duration)}</span><span>·</span><span>{d.roundCount} rounds</span>{d.status === 'partial' && <TriangleAlert size={10} />}</div></div></button><button className="demo-delete icon-button" title={`Remove ${d.name} from library`} aria-label={`Remove ${d.name} from library`} onClick={() => void removeDemo(d)}><Trash2 size={12} /></button></div>)}</> : <div className="library-empty"><FolderOpen size={30} strokeWidth={1} /><strong>{demos.length ? 'No matching demos' : 'Your matches, in one place'}</strong><p>{demos.length ? 'Try another map or filename.' : 'Import a recording to start building your local demo library.'}</p></div>}
        </div>
        {jobs.length > 0 && <div className="import-queue"><div className="queue-title"><span>{activeJobs.length ? `IMPORTING · ${activeJobs.length}` : 'RECENT IMPORTS'}</span>{!activeJobs.length && <button className="text-button" onClick={() => setJobs([])}>Clear</button>}</div>{jobs.slice(-4).map(job => <div className="import-job" key={job.jobId}><div><span className="truncate" title={job.path}>{job.name}</span>{activeStage(job.stage) ? <button className="icon-button" title="Cancel import" aria-label={`Cancel ${job.name}`} onClick={() => void perform(() => api.cancelImport(job.jobId))}><X size={12} /></button> : job.stage === 'complete' ? <Check size={12} className="positive-text" /> : <TriangleAlert size={12} className="warning-text" />}</div><div className="progress-track"><i style={{ width: `${Math.min(100, Math.max(0, job.progress <= 1 ? job.progress * 100 : job.progress))}%` }} /></div><span className="job-stage" title={job.message}>{job.message || job.stage}</span></div>)}</div>}
        <button className="library-import" onClick={() => void importDemos()}><Upload size={17} /><span>Drop .dem files here<small>CS2 & CS:GO supported</small></span><Plus size={14} /></button>
        <div className="library-footer"><ShieldCheck size={13} /><span>Processed on your computer</span></div>
      </aside>}
      {showLibrary && <div className="resize-handle vertical library-resize" role="separator" aria-label="Resize demo library" aria-orientation="vertical" tabIndex={0} onPointerDown={e => resize('left', e)} onKeyDown={e => { if (e.key === 'ArrowLeft') setLeftWidth(w => Math.max(205, w - 10)); if (e.key === 'ArrowRight') setLeftWidth(w => Math.min(350, w + 10)); }} />}
      <main className="main-workspace">
        {!detail ? <><div className="workspace-breadcrumb">{!showLibrary && <button className="icon-button" title="Show library" onClick={() => setShowLibrary(true)}><PanelLeft size={16} /></button>}<span>WORKSPACE</span><ChevronRight size={12} /><span>Overview</span></div>{loading ? <div className="full-loading"><LoaderCircle className="spin" size={27} /><h2>Opening recording</h2><p>Loading match metadata and analysis.</p></div> : <EmptyState onImport={() => void importDemos()} onHelp={() => setHelpOpen(true)} />}</> : <>
          <div className="match-header"><div className="match-title-row">{!showLibrary && <button className="icon-button" title="Show library" aria-label="Show library" onClick={() => setShowLibrary(true)}><PanelLeft size={16} /></button>}<div className="map-monogram"><MapIcon size={20} /></div><div className="match-title"><div><h1>{detail.demo.map || 'Unknown map'}</h1><span className="engine-tag cs2">{detail.demo.engine === 'cs2' ? 'CS2' : 'CS:GO'}</span><span className="subtle-chip">{detail.demo.recordingType || 'Demo recording'}</span></div><p title={detail.demo.path}>{detail.demo.name}</p></div><div className="match-actions"><button className="button subtle compact" title="Open demo in Counter-Strike" onClick={() => void perform(async () => { const result = await api.openInGame(detail.demo.id, Math.round(tick)); notify(result.reason, result.available ? 'success' : 'info'); })}><ExternalLink size={13} /><span>Open in game</span></button><details className="dropdown"><summary className="button subtle compact"><Download size={13} />Export<ChevronDown size={12} /></summary><div className="dropdown-menu">{(['html', 'json'] as const).map(format => <button key={format} onClick={() => void perform(async () => { const path = await api.exportReport(detail.demo.id, format); if (path) notify(`Report saved: ${path}`, 'success'); })}>{format === 'html' ? <FileText size={14} /> : <FileCode2 size={14} />}{format.toUpperCase()} report</button>)}<button onClick={() => void perform(async () => { const commands = await api.getPlaybackCommands(detail.demo.id, Math.round(tick)); await api.copyText(`${commands.play}\n${commands.seek}`); }, 'Playback commands copied')}><Copy size={14} />Copy game commands</button></div></details></div></div><div className="match-stats"><span><Users size={12} />{detail.players.length} players</span><span><Flag size={12} />{detail.rounds.length} rounds</span><span><Clock3 size={12} />{time(detail.demo.duration)}</span><span><Gauge size={12} />{detail.demo.tickRate.toFixed(0)} tick</span><span className="match-file-size">{size(detail.demo.fileSize)}</span><span className="analysis-badge"><Activity size={12} />{detail.findings.length} review signals</span></div></div>
          <div className={`replay-region ${expandedReplay ? 'expanded' : ''}`}>
            <div className="replay-toolbar"><div className="toolbar-label"><Radar size={15} /><strong>Match replay</strong><div className="view-switch" aria-label="Replay view"><button className={reviewView === 'map' ? 'active' : ''} onClick={() => { setReviewView('map'); setPlaying(false); setExpandedReplay(false); }}>2D map</button><button className={reviewView === 'game' ? 'active' : ''} onClick={() => { setReviewView('game'); setPlaying(false); setExpandedReplay(true); }}>Recorded clips</button></div>{replayLoading && <LoaderCircle className="spin" size={12} />}</div><div className="toolbar-right"><button className={`icon-button ${expandedReplay ? 'active' : ''}`} title={expandedReplay ? 'Show timeline and roster' : 'Expand replay'} aria-label={expandedReplay ? 'Show timeline and roster' : 'Expand replay'} onClick={() => setExpandedReplay(value => !value)}><Maximize2 size={17} /></button><select aria-label="Select round" value={round?.number ?? ''} onChange={e => { const target = detail.rounds.find(r => r.number === Number(e.target.value)); if (target) seek(target.startTick); }}>{!round && <option value="">Select round</option>}{detail.rounds.map(r => <option key={r.number} value={r.number}>Round {String(r.number).padStart(2, '0')}</option>)}</select>{map && map.floors.length > 1 && <select aria-label="Map floor" value={floor} onChange={e => setFloor(Number(e.target.value))}>{map.floors.map((f, i) => <option key={f.name} value={i}>{f.name}</option>)}</select>}<details className="dropdown overlays-dropdown"><summary className="icon-button" title="Replay overlays" aria-label="Replay overlays"><Layers3 size={15} /></summary><div className="dropdown-menu">{Object.entries(overlays).map(([key, value]) => <label key={key}><input type="checkbox" checked={value} onChange={e => setOverlays(v => ({ ...v, [key]: e.target.checked }))} />{({ vision: 'View directions', shots: 'Shot paths', deaths: 'Death markers', utility: 'Utility & bomb', labels: 'Player names' } as Record<string, string>)[key]}</label>)}<p>Utility markers show recorded events, not exact effect durations.</p></div></details><button className={`icon-button ${follow ? 'active' : ''}`} disabled={!selectedPlayer} title={follow ? 'Stop following player' : 'Follow selected player'} aria-label="Follow selected player" aria-pressed={follow} onClick={() => setFollow(value => !value)}><Crosshair size={15} /></button></div></div>
            {reviewView === 'game' ? <GameView key={detail.demo.id} demo={detail.demo} tick={Math.round(tick)} player={player} /> : <>
            {(mapState === 'loading' || mapState === 'extracting') && <div className="map-feedback" role="status"><LoaderCircle size={18} className="spin" /><div><strong>{mapState === 'extracting' ? 'Preparing map from your game installation…' : 'Loading map…'}</strong><p>{mapState === 'extracting' ? 'The first extraction can take a minute. Replay controls and player positions remain available.' : 'Checking cached radar and matching assets.'}</p></div></div>}
            {mapState === 'error' && <div className="map-feedback error" role="alert"><MapIcon size={20} /><div><strong>Map image could not be loaded</strong><p>{mapError}</p><div className="map-feedback-actions"><button className="button compact subtle" disabled={!!busy} onClick={() => void runMapAction('extract')}><RefreshCw size={14} />Retry map</button><button className="text-button" onClick={() => setSettingsOpen(true)}>Game settings</button><button className="text-button" onClick={() => void runMapAction('import')}>Import map pack</button></div></div></div>}
            <ReplayMap key={detail.demo.id} replay={replay} tick={tick} tickRate={tickRate} players={detail.players} selectedId={selectedPlayer} follow={follow} onSelect={selectPlayer} map={map} floor={floor} overlays={overlays} imageRevision={mapRevision} onImageError={() => { setMapState('error'); setMapError('The cached radar image is missing or unreadable. Retry to extract it again from your local game installation.'); }} />
            <div className="map-status"><span><span className={`status-dot ${map?.image ? 'green' : 'amber'}`} />{map?.verified ? `Verified map assets · ${map.version}` : map?.image ? 'Map ready · local game radar' : 'Player positions · coordinate grid'}</span><details className="dropdown map-dropdown"><summary className="text-button">Map assets<ChevronDown size={13} /></summary><div className="dropdown-menu"><button disabled={!!busy || mapState === 'extracting'} onClick={() => void runMapAction('extract')}><Download size={15} />Extract installed map</button><button disabled={!!busy} onClick={() => void runMapAction('import')}><FolderOpen size={15} />Import historical map pack</button><button disabled={!!busy} onClick={() => void runMapAction('analyse')}><RefreshCw size={15} />Reanalyse recording</button>{busy && <p><LoaderCircle className="spin" size={14} /> Working…</p>}{map?.warnings?.map(warning => <p key={warning}>{warning}</p>)}</div></details></div>
            </>}
          </div>
          <div hidden={expandedReplay} className="resize-handle horizontal" role="separator" aria-label="Resize timeline and roster" aria-orientation="horizontal" tabIndex={0} onPointerDown={e => resize('bottom', e)} onKeyDown={e => { if (e.key === 'ArrowUp') setBottomHeight(h => Math.min(450, h + 15)); if (e.key === 'ArrowDown') setBottomHeight(h => Math.max(205, h - 15)); }} />
          <div className={`bottom-panel ${expandedReplay ? 'collapsed' : ''}`}><div className="transport"><div className="transport-buttons"><button className="icon-button" title="Previous round" aria-label="Previous round" onClick={() => seek(detail.rounds.filter(r => r.startTick < tick - 1).at(-1)?.startTick ?? 0)}><SkipBack size={14} /></button><button className="icon-button" title="Previous tick (Left)" aria-label="Previous tick" onClick={() => seek(Math.floor(tick) - 1)}><ChevronLeft size={17} /></button><button className="play-button" disabled={reviewView === 'game'} title={reviewView === 'game' ? 'Use the clip player controls for recorded footage' : playing ? 'Pause (Space)' : 'Play (Space)'} aria-label={playing ? 'Pause' : 'Play'} onClick={() => { if (tick >= totalTicks) setTick(0); setPlaying(value => !value); }}>{playing ? <Pause size={17} fill="currentColor" /> : <Play size={17} fill="currentColor" />}</button><button className="icon-button" title="Next tick (Right)" aria-label="Next tick" onClick={() => seek(Math.floor(tick) + 1)}><ChevronRight size={17} /></button><button className="icon-button" title="Next round" aria-label="Next round" onClick={() => seek(detail.rounds.find(r => r.startTick > tick + 1)?.startTick ?? totalTicks)}><SkipForward size={14} /></button></div><div className="playback-time">{time(tick / tickRate)}<span>/ {time(detail.demo.duration)}</span></div><span className="tick-label">TICK {Math.floor(tick).toLocaleString()}</span><div className="transport-end"><select aria-label="Playback speed" value={speed} onChange={e => setSpeed(Number(e.target.value))}>{[0.25, 0.5, 1, 2, 4, 8].map(v => <option key={v} value={v}>{v}×</option>)}</select><button className="icon-button" title="Bookmark this tick" aria-label="Bookmark this tick" onClick={() => void bookmark()}><Bookmark size={15} /></button></div></div>
            <div className="timeline"><div className="timeline-rounds">{detail.rounds.map(r => <button key={r.number} title={`Round ${r.number} · ${r.winner || 'No result'}`} className={round?.number === r.number ? 'current' : ''} style={{ left: `${r.startTick / Math.max(totalTicks, 1) * 100}%`, width: `${Math.max(0.2, (r.endTick - r.startTick) / Math.max(totalTicks, 1) * 100)}%` }} onClick={() => seek(r.startTick)}>{r.number}</button>)}</div><div className="timeline-track"><span className="timeline-progress" style={{ width: `${tick / Math.max(totalTicks, 1) * 100}%` }} />{detail.findings.map(f => <button key={f.id} className={`timeline-marker ${f.severity === 'strong' ? 'strong' : ''}`} title={`${f.title} · ${time(f.time)}`} aria-label={`Review ${f.title} at ${time(f.time)}`} style={{ left: `${f.tick / Math.max(totalTicks, 1) * 100}%` }} onClick={() => selectFinding(f)} />)}{detail.notes.filter(n => n.kind === 'bookmark').map(n => <span className="bookmark-marker" key={n.id} style={{ left: `${n.tick / Math.max(totalTicks, 1) * 100}%` }} />)}<input aria-label="Replay position" type="range" min={0} max={Math.max(totalTicks, 1)} step={1} value={Math.floor(tick)} onChange={e => seek(Number(e.target.value))} /></div><div className="timeline-caption"><span>00:00</span><span><i />Review signal <b />Bookmark</span><span>{time(detail.demo.duration)}</span></div></div>
            <div className="roster-tabs"><button className={bottomTab === 'players' ? 'active' : ''} onClick={() => setBottomTab('players')}><Users size={13} />Players<span>{detail.players.length}</span></button><button className={bottomTab === 'events' ? 'active' : ''} onClick={() => setBottomTab('events')}><Activity size={13} />Event feed</button><button className={bottomTab === 'aim' ? 'active' : ''} onClick={() => setBottomTab('aim')}><Crosshair size={13} />Aim trace</button><span className="roster-hint">{bottomTab === 'players' ? 'Select a player to inspect evidence' : bottomTab === 'aim' ? 'Original samples · click chart to seek' : 'Events around the current playhead'}</span></div>
            {bottomTab === 'players' ? <div className="roster-scroll"><table className="player-table"><thead><tr><th>PLAYER</th><th>K / D / A</th><th>HS</th><th>HEALTH</th><th>ASSESSMENT</th><th>SIGNALS</th></tr></thead><tbody>{detail.players.map(p => { const sample = currentSamples.find(s => s.playerId === p.id); return <tr key={p.id} className={selectedPlayer === p.id ? 'selected' : ''} onClick={() => selectPlayer(p.id)}><td><button className="player-name" onClick={() => selectPlayer(p.id)}><span className="player-avatar" style={{ color: playerColor(sample?.team ?? p.team), borderColor: `${playerColor(sample?.team ?? p.team)}44` }}>{p.name.slice(0, 1).toUpperCase()}</span><span>{p.name}</span>{sample && !sample.alive && <span className="dead-label">OUT</span>}</button></td><td className="mono">{p.kills}<span> / </span>{p.deaths}<span> / </span>{p.assists}</td><td className="mono">{p.kills ? Math.round(p.headshots / p.kills * 100) : 0}<span>%</span></td><td><div className="health-cell"><i style={{ width: `${Math.max(0, Math.min(100, sample?.health ?? 0))}%` }} /><span>{sample ? Math.max(0, sample.health) : '—'}</span></div></td><td><VerdictBadge player={p} /></td><td className={p.evidenceCount ? 'signal-count' : 'muted'}>{p.evidenceCount || '—'}</td></tr>; })}</tbody></table></div> : bottomTab === 'aim' ? <AimTrace replay={replay} playerId={selectedPlayer} playerName={player?.name ?? ''} tick={tick} tickRate={tickRate} onSeek={seek} /> : <EventFeed detail={detail} tick={tick} tickRate={tickRate} onSeek={seek} />}
          </div>
        </>}
      </main>
      {detail && <><div className="resize-handle vertical evidence-resize" role="separator" aria-label="Resize evidence panel" aria-orientation="vertical" tabIndex={0} onPointerDown={e => resize('right', e)} onKeyDown={e => { if (e.key === 'ArrowLeft') setRightWidth(w => Math.min(480, w + 10)); if (e.key === 'ArrowRight') setRightWidth(w => Math.max(280, w - 10)); }} /><aside className="evidence-panel"><div className="panel-heading"><span><ScanIcon />Player inspector</span><button className="icon-button" title="Assessment methodology" aria-label="Assessment methodology" onClick={() => setHelpOpen(true)}><CircleHelp size={14} /></button></div>{player ? <><div className="inspector-player"><div className="inspector-avatar" style={{ color: playerColor(player.team) }}>{player.name.slice(0, 2).toUpperCase()}</div><div><h2>{player.name}</h2><span>{selectedSample?.weapon || player.team || 'Player'}<i />{player.kills} K · {player.deaths} D</span></div><button className={`icon-button ${follow ? 'active' : ''}`} title="Follow player on map" aria-label="Follow player on map" aria-pressed={follow} onClick={() => setFollow(v => !v)}><Crosshair size={16} /></button></div><div className={`assessment-card ${verdictClass(player.verdict)}`}><div className="eyebrow">EXPERIMENTAL ASSESSMENT</div><div className="assessment-title">{(player.verdict === 'Insufficient data' || player.verdict === 'Reviewed with limits') ? <ShieldQuestion size={20} /> : player.verdict === 'Low concern' ? <ShieldCheck size={20} /> : <TriangleAlert size={20} />}<strong>{player.verdict}</strong></div><p>{player.coverage || 'This recording does not contain enough validated observations for a reliable assessment.'}</p><div className="assessment-footer"><span>{player.evidenceCount} review {player.evidenceCount === 1 ? 'signal' : 'signals'}</span><span>Measured coverage only</span></div></div><div className="inspector-tabs"><button className={sideTab === 'evidence' ? 'active' : ''} onClick={() => setSideTab('evidence')}>Evidence<span>{playerFindings.length}</span></button><button className={sideTab === 'notes' ? 'active' : ''} onClick={() => setSideTab('notes')}>Review notes<span>{detail.notes.length}</span></button></div><div className="inspector-scroll" ref={inspectorRef}>{busy === 'analyse' && <div className="analysis-refresh-status" role="status"><LoaderCircle size={16} className="spin" />Analysing all players… Results will update here.</div>}{sideTab === 'evidence' ? <><PlayerReviewPanel player={player} onSeek={seek} onAnalyse={() => void runMapAction('analyse')} busy={!!busy || activeJobs.some(job => job.demoId === detail.demo.id)} /><div className="section-label">SIGNAL COVERAGE<SlidersHorizontal size={12} /></div><SignalCoverage capabilities={player.capabilities} /><div className="section-label">REVIEW SIGNALS<span>{playerFindings.length}</span></div>{playerFindings.length ? <div className="findings-list">{playerFindings.map(f => <button key={f.id} className={`finding-card ${selectedFinding === f.id ? 'selected' : ''}`} onClick={() => selectFinding(f)}><div className="finding-top"><span className={`finding-signal ${f.severity}`}>{signalLabel[f.signal]}</span><span>R{f.round} · {time(f.time)}</span></div><strong>{f.title}</strong><p>{f.description}</p><span className="finding-link">Inspect event<ArrowRight size={12} /></span></button>)}</div> : <div className="no-evidence"><Eye size={23} strokeWidth={1.3} /><strong>No repeated signals flagged</strong><p>{player.verdict === 'Insufficient data' ? 'Coverage is limited. An absence of findings is not proof of legitimate play.' : 'No repeated patterns crossed the current review thresholds in eligible observations. Other signals may have only measured coverage.'}</p></div>}{finding && <FindingDetail finding={finding} replay={replay} tickRate={tickRate} />}</> : <NotesPanel detail={detail} selectedPlayer={selectedPlayer} tick={Math.round(tick)} tickRate={tickRate} onSeek={seek} onSave={mutateNotes} onDelete={id => setDetail(current => current ? { ...current, notes: current.notes.filter(n => n.id !== id) } : current)} notify={notify} />}{detail.demo.warnings.length > 0 && <details className="parser-warnings"><summary><TriangleAlert size={13} />{detail.demo.warnings.length} recording {detail.demo.warnings.length === 1 ? 'limitation' : 'limitations'}<ChevronDown size={12} /></summary><ul>{detail.demo.warnings.map((w, i) => <li key={i}>{w}</li>)}</ul></details>}</div><div className="inspector-bottom"><ShieldQuestion size={14} /><span>Always review the original gameplay.</span></div></> : <div className="no-evidence"><Users size={24} /><p>Select a player from the roster.</p></div>}</aside></>}
    </div>
    <footer className="app-status"><div><span className={`status-dot ${workerError ? 'amber' : 'green'}`} />{workerError ? 'Connection needs attention' : activeJobs.length ? `Processing ${activeJobs.length} recording${activeJobs.length > 1 ? 's' : ''}` : 'Ready'}<span className="status-separator">/</span><span>{detail ? `${detail.demo.engine.toUpperCase()} · ${detail.demo.parserVersion}` : 'CS2 + CS:GO'}</span></div><div><span>{detail ? `Analysis ${detail.demo.analysisVersion}` : 'Experimental analysis'}</span><span className="status-separator">/</span><span className="shortcut-hint"><kbd>SPACE</kbd> Play / pause <kbd>← →</kbd> Step</span><button className="text-button" onClick={() => setSettingsOpen(true)}>Settings</button></div></footer>
    {notice && <div className={`toast ${notice.kind}`} role={notice.kind === 'error' ? 'alert' : 'status'}>{notice.kind === 'error' ? <TriangleAlert size={17} /> : notice.kind === 'success' ? <Check size={17} /> : <Activity size={17} />}<span>{notice.text}</span><button className="icon-button" title="Dismiss notification" aria-label="Dismiss notification" onClick={() => setNotice(null)}><X size={15} /></button></div>}
    {dragging && <div className="drop-overlay"><div><FileUp size={42} /><h2>Drop your recordings</h2><p>Import CS2 and CS:GO .dem files into your local library.</p></div></div>}
    {settingsOpen && <SettingsModal onClose={() => setSettingsOpen(false)} notify={notify} />}
    {helpOpen && <Modal title="A closer look. A careful conclusion." subtitle="HOW TO READ THE EVIDENCE" onClose={() => setHelpOpen(false)}><div className="help-content"><p>CS Demo Review measures patterns in recorded gameplay. Network conditions, sampling intervals, game mechanics, anticipation, and skilled play can all produce unusual events.</p><div className="help-verdicts"><div><span className="verdict-badge positive">Low concern</span><p>Sufficient observations were assessed without repeated review signals. This is not proof of legitimate play.</p></div><div><span className="verdict-badge warning">Suspicious</span><p>Repeated observations need a closer review of context and plausible explanations.</p></div><div><span className="verdict-badge danger">Highly suspicious</span><p>Repeated evidence from independent signal families warrants close scrutiny.</p></div><div><span className="verdict-badge measured">Reviewed with limits</span><p>Enough recorded observations support a useful review, but some signal families remain unassessed. This does not establish legitimate play.</p></div><div><span className="verdict-badge neutral">Insufficient data</span><p>Missing fields, sampling gaps, unsupported features, or too few observations prevent an assessment.</p></div></div><div className="info-box"><ShieldQuestion size={18} /><p>Beta thresholds are experimental. Findings are never probabilities of cheating, and the app does not submit reports or take enforcement actions.</p></div><h3>Recording limits matter</h3><p>Visibility-to-shot timing uses verified map geometry. A labelled network spotting interval is also available as a proxy; it does not establish what appeared on screen. Recoil and shot direction show recorded telemetry when available; impact-derived directions do not prove a bullet changed direction. Detector eligibility remains separate from these measurements, with source and sample counts shown for each signal.</p><h3>Keyboard controls</h3><div className="keyboard-help"><span><kbd>Space</kbd> Play / pause</span><span><kbd>← →</kbd> One tick</span><span><kbd>Shift + ← →</kbd> Five seconds</span></div></div></Modal>}
  </div>;
}

function ScanIcon() { return <Target size={15} />; }
function VerdictBadge({ player }: { player: Player }) { return <span className={`verdict-badge ${verdictClass(player.verdict)}`}><i />{player.verdict}</span>; }

function EmptyState({ onImport, onHelp }: { onImport: () => void; onHelp: () => void }) {
  return <div className="empty-workspace"><div className="empty-visual" aria-hidden="true"><div className="visual-grid" /><div className="radar-ring ring-1" /><div className="radar-ring ring-2" /><div className="radar-ring ring-3" /><div className="radar-cross horizontal" /><div className="radar-cross vertical" /><div className="radar-scan" /><div className="radar-centre"><Crosshair size={36} strokeWidth={1.2} /></div><span className="visual-coordinate coord-one">REPLAY. INSPECT. UNDERSTAND.</span><span className="visual-coordinate coord-two">LOCAL ANALYSIS / NO UPLOADS</span><div className="visual-point point-one" /><div className="visual-point point-two" /><div className="visual-point point-three" /></div><div className="empty-intro"><span className="eyebrow accent">EVERY ROUND HAS A STORY</span><h1>Look beyond the scoreboard.</h1><p>Replay your matches. Investigate the moments that matter.<br />Make sense of the evidence, one tick at a time.</p><button className="button primary large" onClick={onImport}><Plus size={17} />Import your first demo<ArrowRight size={16} /></button><span className="import-caption">or drag & drop .dem files anywhere</span></div><div className="feature-grid"><div><div className="feature-icon"><Radar size={20} /></div><h3>See the whole round</h3><p>A synchronised 2D replay with player positions, shots, and game events.</p></div><div><div className="feature-icon"><Activity size={20} /></div><h3>Follow the evidence</h3><p>Inspect reaction timing, aim transitions, and the limits of each signal.</p></div><div><div className="feature-icon"><ShieldCheck size={20} /></div><h3>Keep it on your machine</h3><p>Your recordings and review notes stay in your local library.</p></div></div><button className="empty-methodology text-button" onClick={onHelp}><CircleHelp size={13} />Understand how assessments work<ArrowRight size={12} /></button></div>;
}

function EventFeed({ detail, tick, tickRate, onSeek }: { detail: MatchDetail; tick: number; tickRate: number; onSeek: (tick: number) => void }) {
  const relevant = detail.events.filter(e => e.tick <= tick && tick - e.tick <= tickRate * 60).slice(-40).reverse();
  return <div className="event-feed">{relevant.length ? relevant.map(e => <button key={e.id} onClick={() => onSeek(e.tick)}><span className="mono">{time(e.time)}</span><span className={`event-kind ${/kill|death/i.test(e.kind) ? 'kill' : ''}`}>{e.kind.replace(/_/g, ' ')}</span><span>{e.text}</span>{e.headshot && <Crosshair size={12} />}</button>) : <div className="inline-empty">No recorded events in the last 60 seconds.</div>}</div>;
}

function FindingDetail({ finding, replay, tickRate }: { finding: Finding; replay: ReplayWindow | null; tickRate: number }) {
  const selectedRef = useRef<HTMLElement>(null);
  useEffect(() => { selectedRef.current?.scrollIntoView({ block: 'nearest' }); }, [finding.id]);
  const trace = useMemo(() => buildAimTrace((replay?.samples ?? []).filter(s => Math.abs(s.tick - finding.tick) <= tickRate * 1.5), finding.playerId, tickRate), [replay, finding, tickRate]);
  const points = trace.segments.flat();
  const max = Math.max(1, ...points.map(p => p.yawSpeed));
  const path = trace.segments.map(segment => segment.map((p, index) => `${index ? 'L' : 'M'} ${8 + (p.tick - (finding.tick - tickRate * 1.5)) / (tickRate * 3) * 264} ${78 - p.yawSpeed / max * 62}`).join(' ')).join(' ');
  return <section ref={selectedRef} className="finding-detail"><div className="section-label">SELECTED EVENT<span>R{finding.round} / {finding.tick}</span></div><h3>{finding.title}</h3><div className="measurement-grid">{finding.measurements.map((m, i) => <div key={i}><span>{m.label}</span><strong>{measurementValue(m.value)}<small>{m.unit}</small></strong></div>)}</div>{points.length > 1 && <div className="evidence-chart"><div><span>Recorded yaw speed</span><span>°/s</span></div><svg viewBox="0 0 280 92" role="img" aria-label={`Yaw speed around the finding, peak ${max.toFixed(0)} degrees per second`}><line x1="8" y1="78" x2="272" y2="78" stroke="#2c3643" /><line x1="140" y1="8" x2="140" y2="80" stroke="#687688" strokeDasharray="3 3" /><path d={path} fill="none" stroke="#69c4b0" strokeWidth="1.5" /><text x="8" y="91">−1.5 s</text><text x="133" y="91">event</text><text x="248" y="91">+1.5 s</text></svg><p>Recorded sample changes; context only.</p></div>}<h4>Possible explanations</h4><ul>{finding.alternatives.map((text, i) => <li key={i}>{text}</li>)}</ul><h4>Data limitations</h4><ul>{finding.limitations.map((text, i) => <li key={i}>{text}</li>)}</ul><div className="episode-label">Episode {finding.episodeId.slice(0, 20)}</div></section>;
}

function NotesPanel({ detail, selectedPlayer, tick, tickRate, onSeek, onSave, onDelete, notify }: { detail: MatchDetail; selectedPlayer: string; tick: number; tickRate: number; onSeek: (tick: number) => void; onSave: (note: ReviewNote) => void; onDelete: (id: string) => void; notify: (text: string, kind?: Notice['kind']) => void }) {
  const [text, setText] = useState(''); const [saving, setSaving] = useState(false);
  const save = async () => { if (!text.trim()) return; setSaving(true); try { const result = await api.saveNote({ demoId: detail.demo.id, tick, playerId: selectedPlayer, text: text.trim(), kind: 'note' }); onSave(result); setText(''); } catch (error) { notify(errorMessage(error), 'error'); } finally { setSaving(false); } };
  return <div className="notes-panel"><label htmlFor="review-note" className="section-label">ADD A REVIEW NOTE<span>{time(tick / tickRate)}</span></label><textarea id="review-note" value={text} maxLength={10000} onChange={e => setText(e.target.value)} placeholder="What happened? Add context or an alternative explanation…" /><div className="note-compose-footer"><span>Anchored to tick {tick}</span><button className="button subtle compact" disabled={!text.trim() || saving} onClick={() => void save()}>{saving ? <LoaderCircle className="spin" size={12} /> : <Plus size={12} />}Save note</button></div><div className="section-label">MATCH NOTES & BOOKMARKS<span>{detail.notes.length}</span></div>{!detail.notes.length && <div className="no-evidence"><MessageSquare size={23} strokeWidth={1.3} /><strong>Keep your own context</strong><p>Notes and bookmarks are saved locally and included in exports.</p></div>}{[...detail.notes].sort((a, b) => a.tick - b.tick).map(note => <div className="note-card" key={note.id}><div><button className="text-button" onClick={() => onSeek(note.tick)}>{note.kind === 'bookmark' ? <Bookmark size={12} /> : <MessageSquare size={12} />}{time(note.tick / tickRate)}<span>· tick {note.tick}</span></button><button className="icon-button" title="Delete note" aria-label="Delete note" onClick={async () => { try { await api.deleteNote(note.id); onDelete(note.id); } catch (error) { notify(errorMessage(error), 'error'); } }}><Trash2 size={12} /></button></div><p>{note.text}</p><span>{detail.players.find(p => p.id === note.playerId)?.name || 'Match note'}</span></div>)}</div>;
}

function Modal({ title, subtitle, onClose, children }: { title: string; subtitle: string; onClose: () => void; children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose); closeRef.current = onClose;
  useEffect(() => { const previous = document.activeElement as HTMLElement | null; ref.current?.focus(); const handler = (event: KeyboardEvent) => { if (event.key === 'Escape') closeRef.current(); if (event.key === 'Tab') { const items = ref.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select, textarea, a[href], [tabindex="0"]'); if (!items?.length) return; const first = items[0], last = items[items.length - 1]; if (event.shiftKey && (document.activeElement === first || document.activeElement === ref.current)) { event.preventDefault(); last.focus(); } else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); } } }; window.addEventListener('keydown', handler); return () => { window.removeEventListener('keydown', handler); previous?.focus(); }; }, []);
  return <div className="modal-backdrop" onMouseDown={e => { if (e.target === e.currentTarget) onClose(); }}><div className="modal" role="dialog" aria-modal="true" aria-label={title} tabIndex={-1} ref={ref}><div className="modal-header"><div><span className="eyebrow accent">{subtitle}</span><h2>{title}</h2></div><button className="icon-button" title="Close" aria-label="Close dialog" onClick={onClose}><X size={20} /></button></div><div className="modal-body">{children}</div></div></div>;
}

function SettingsModal({ onClose, notify }: { onClose: () => void; notify: (text: string, kind?: Notice['kind']) => void }) {
  const [settings, setSettings] = useState<AppSettings>(emptySettings); const [diagnostics, setDiagnostics] = useState<Diagnostics | null>(null); const [loading, setLoading] = useState(true); const [saving, setSaving] = useState(false);
  useEffect(() => { if (!api) { setLoading(false); return; } Promise.allSettled([api.getSettings(), api.getDiagnostics()]).then(results => { if (results[0].status === 'fulfilled') setSettings(results[0].value); else notify(errorMessage(results[0].reason), 'error'); if (results[1].status === 'fulfilled') setDiagnostics(results[1].value); else notify(errorMessage(results[1].reason), 'error'); setLoading(false); }); }, [notify]);
  const choose = async (kind: 'cs2' | 'csgo' | 'source2viewer', key: keyof AppSettings) => { try { const path = await api.choosePath(kind); if (path) setSettings(current => ({ ...current, [key]: path })); } catch (error) { notify(errorMessage(error), 'error'); } };
  const save = async () => { setSaving(true); try { setSettings(await api.updateSettings({ cs2Path: settings.cs2Path, csgoPath: settings.csgoPath, steamPath: settings.steamPath, source2ViewerPath: settings.source2ViewerPath })); notify('Settings saved', 'success'); onClose(); } catch (error) { notify(errorMessage(error), 'error'); } finally { setSaving(false); } };
  return <Modal title="Workspace settings" subtitle="LOCAL CONFIGURATION" onClose={onClose}>{loading ? <div className="full-loading"><LoaderCircle className="spin" size={24} /></div> : <div className="settings-content"><h3>Game installations</h3><p>Detected Steam paths are filled automatically. Override a path if you use a different or legacy installation.</p>{([{ key: 'cs2Path', label: 'Counter-Strike 2', kind: 'cs2', hint: 'cs2.exe game executable' }, { key: 'csgoPath', label: 'Counter-Strike: Global Offensive', kind: 'csgo', hint: 'csgo.exe from a compatible legacy installation' }, { key: 'source2ViewerPath', label: 'Source 2 Viewer', kind: 'source2viewer', hint: 'Source2Viewer-CLI.exe for installed CS2 map asset extraction' }] as const).map(field => <label className="settings-field" key={field.key}><span>{field.label}</span><div><input value={settings[field.key]} onChange={e => setSettings(value => ({ ...value, [field.key]: e.target.value }))} placeholder="Not configured" /><button className="button subtle" disabled={!api} onClick={() => void choose(field.kind, field.key)}><FolderOpen size={14} />Browse</button></div><small>{field.hint}</small></label>)}<label className="settings-field"><span>Steam directory</span><input value={settings.steamPath} onChange={e => setSettings(value => ({ ...value, steamPath: e.target.value }))} placeholder="Automatically detected" /></label><h3>Storage & diagnostics</h3><div className="storage-card"><Database size={19} /><div><strong>Local demo library</strong><code>{settings.libraryPath || diagnostics?.libraryPath || 'Available in the desktop app'}</code><span>Original recordings remain in their original locations.</span></div></div><div className="diagnostics-row"><span>App {diagnostics?.appVersion ?? '—'} · Worker {diagnostics?.workerVersion ?? '—'}</span><button className="text-button" disabled={!api} onClick={async () => { try { const result = await api.exportDiagnostics(); if (result) notify(`Diagnostics saved: ${result}`, 'success'); } catch (error) { notify(errorMessage(error), 'error'); } }}><ArrowDownToLine size={13} />Export diagnostics</button></div><div className="info-box"><ShieldCheck size={17} /><p>No accounts, telemetry, or automatic uploads. Reports and diagnostics are only saved when you choose to export them.</p></div><div className="settings-attribution">Map extraction powered by <a href="https://s2v.app/" target="_blank" rel="noreferrer">Source 2 Viewer <ExternalLink size={10} /></a>. Demo parsing by <a href="https://github.com/markus-wa/demoinfocs-golang" target="_blank" rel="noreferrer">demoinfocs <ExternalLink size={10} /></a>.</div><div className="modal-actions"><button className="button subtle" onClick={onClose}>Cancel</button><button className="button primary" disabled={saving || !api} onClick={() => void save()}>{saving && <LoaderCircle size={14} className="spin" />}Save settings</button></div></div>}</Modal>;
}
