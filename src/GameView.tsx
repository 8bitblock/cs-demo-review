import { useCallback, useEffect, useRef, useState } from 'react';
import { Circle, Copy, ExternalLink, Film, MonitorPlay, RefreshCw, Square, Trash2, TriangleAlert, Upload, X } from 'lucide-react';
import type { Demo, GameClip, GameWindow, PlaybackCommands, Player } from '../shared/types';
import { errorMessage, size, time } from './format';
import { useClipRecorder } from './useClipRecorder';
import './GameView.css';

type Props = { demo: Demo; tick: number; player: Player | null };
export function GameView(props: Props) {
  const { demo, tick, player } = props;
  const [clips, setClips] = useState<GameClip[]>([]), [selected, setSelected] = useState(''), [setup, setSetup] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState('');
  useEffect(() => { let stale = false; void window.csDemo.listClips(demo.id).then(items => { if (!stale) setClips(items); }).catch(e => { if (!stale) setError(errorMessage(e)); }); return () => { stale = true; }; }, [demo.id]);
  const add = (clip: GameClip) => { setClips(items => [clip, ...items]); setSelected(clip.id); setSetup(false); };
  const clip = clips.find(item => item.id === selected) ?? clips[0];
  const importClip = async () => {
    setBusy(true); setError('');
    try { const saved = await window.csDemo.importClip({ demoId: demo.id, playerId: player?.id ?? '', reviewTick: Math.round(tick), title: `${player?.name || demo.map} · ${time(tick / demo.tickRate)}` }); if (saved) add(saved); }
    catch (e) { setError(errorMessage(e)); } finally { setBusy(false); }
  };
  if (setup) return <GameRecorder {...props} onSaved={add} onClose={() => setSetup(false)} />;
  return <section className="game-view" aria-label="Recorded game clips">
    <div className="game-view-toolbar"><span><Film size={18} />Recorded game footage</span><div><button className="button subtle" disabled={busy} onClick={() => void importClip()}><Upload size={15} />Import clip</button><button className="button primary" onClick={() => setSetup(true)}><Circle size={14} />Record from game</button></div></div>
    <div className="saved-clip-stage">{clip ? <video key={clip.id} src={clip.url} controls playsInline preload="metadata" aria-label="Recorded Counter-Strike clip" onError={() => setError('This video cannot be played. Import an MP4 (H.264) or WebM (VP8/VP9) clip.')} /> : <div className="game-video-intro"><Film size={40} strokeWidth={1.3} /><h2>Watch the real scene here</h2><p>Record footage rendered by your installed game, or import an MP4 or WebM. Saved clips play here with the real map geometry, models, lighting and animations captured in the video.</p><p>Select a review moment, then choose <b>Record from game</b>. No video has been recorded for this demo yet.</p></div>}</div>
    {clip && <div className="saved-clip-meta"><div><strong>{clip.title}</strong><span>Review reference {time(clip.reviewTick / demo.tickRate)} · tick {clip.reviewTick} · {clip.source === 'recorded' ? 'Recorded here' : 'Imported video'} · {size(clip.size)}</span></div><button className="icon-button" title="Remove clip" aria-label="Remove clip" onClick={async () => { try { if (await window.csDemo.removeClip(clip.id)) { setClips(items => items.filter(item => item.id !== clip.id)); setSelected(''); } } catch (e) { setError(errorMessage(e)); } }}><Trash2 size={16} /></button></div>}
    {!!clips.length && <div className="saved-clip-list" aria-label="Saved clips">{clips.map(item => <button key={item.id} className={clip?.id === item.id ? 'selected' : ''} onClick={() => { setSelected(item.id); setError(''); }}><Film size={16} /><span>{item.title}<small>{item.duration ? `${item.duration.toFixed(1)} s · ` : ''}{new Date(item.createdAt).toLocaleDateString()}</small></span></button>)}</div>}
    <p className="game-sync-note">Clips are linked to a review timestamp manually. That reference does not verify the video’s game time or player. The review timeline does not seek the video automatically.</p>
    {error && <p className="game-error" role="alert"><TriangleAlert size={16} />{error}</p>}
  </section>;
}

function GameRecorder({ demo, tick, player, onSaved, onClose }: Props & { onSaved: (clip: GameClip) => void; onClose: () => void }) {
  const [windows, setWindows] = useState<GameWindow[]>([]);
  const [sourceId, setSourceId] = useState('');
  const [commands, setCommands] = useState<PlaybackCommands | null>(null);
  const [status, setStatus] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [connected, setConnected] = useState(false);
  const [length, setLength] = useState(20);
  const recorder = useClipRecorder(onSaved, e => setError(errorMessage(e)));
  const video = useRef<HTMLVideoElement>(null);
  const stream = useRef<MediaStream | null>(null);
  const generation = useRef(0);
  const stop = useCallback(() => {
    generation.current++;
    stream.current?.getTracks().forEach(track => track.stop()); stream.current = null;
    if (video.current) video.current.srcObject = null;
    setConnected(false); setBusy(false);
    void window.csDemo.cancelGameCapture().catch(() => {});
  }, []);
  const refresh = useCallback(async () => {
    setBusy(true); setError('');
    try {
      const result = await window.csDemo.getGameWindows(demo.id);
      setWindows(result); setSourceId(current => result.some(w => w.id === current) ? current : result[0]?.id ?? '');
      if (!result.length) setStatus('No game window found. Open the game, use windowed or borderless mode, then refresh.');
    } catch (e) { setError(errorMessage(e)); } finally { setBusy(false); }
  }, [demo.id]);
  useEffect(() => { void refresh(); return stop; }, [refresh, stop]);
  useEffect(() => {
    let stale = false;
    const timer = setTimeout(() => window.csDemo.getPlaybackCommands(demo.id, Math.round(tick)).then(value => { if (!stale) setCommands(value); }).catch(e => { if (!stale) setError(errorMessage(e)); }), 120);
    return () => { stale = true; clearTimeout(timer); };
  }, [demo.id, tick]);
  const connect = async () => {
    stop(); setBusy(true); setError(''); setStatus('Connecting to the game window…');
    const run = generation.current;
    let media: MediaStream | null = null;
    try {
      await window.csDemo.prepareGameCapture(demo.id, sourceId);
      if (run !== generation.current) { await window.csDemo.cancelGameCapture(); return; }
      media = await navigator.mediaDevices.getDisplayMedia({ video: { frameRate: 30, width: 1920, height: 1080 }, audio: false });
      if (run !== generation.current) { media.getTracks().forEach(track => track.stop()); return; }
      stream.current = media;
      media.getVideoTracks()[0].addEventListener('ended', () => { stop(); setStatus('The game capture ended. Refresh and reconnect when the game is ready.'); }, { once: true });
      if (video.current) { video.current.srcObject = media; await video.current.play(); }
      setConnected(true); setStatus('Live game video connected. Control demo playback in the game console.');
    } catch (e) {
      media?.getTracks().forEach(track => track.stop()); stream.current = null;
      if (run === generation.current) { setConnected(false); setError(`${errorMessage(e)} Use windowed or borderless mode and keep the game window open.`); }
    } finally { if (run === generation.current) setBusy(false); }
  };
  const launch = async () => {
    setBusy(true); setError('');
    try { const result = await window.csDemo.openInGame(demo.id, Math.round(tick)); setCommands(result); setStatus(result.reason); }
    catch (e) { setError(errorMessage(e)); } finally { setBusy(false); }
  };
  const copy = async (text: string, message: string) => {
    try { await window.csDemo.copyText(text); setStatus(message); } catch (e) { setError(errorMessage(e)); }
  };
  return <section className="game-view" aria-label="Record a game clip">
    <div className="game-view-toolbar"><span><MonitorPlay size={18} />{demo.engine === 'cs2' ? 'Counter-Strike 2' : 'Legacy CS:GO'} engine</span><div>
      <button className="button subtle" disabled={busy || recorder.recording || recorder.saving} onClick={() => void launch()}><ExternalLink size={15} />Open game</button>
      <button className="button subtle" disabled={busy || recorder.recording || recorder.saving} onClick={onClose}><X size={15} />Close recorder</button>
    </div></div>
    <div className={`game-video-stage ${connected ? 'connected' : ''}`}>
      <video ref={video} muted autoPlay playsInline aria-label="Live Counter-Strike game window" />
      {!connected && <div className="game-video-intro"><MonitorPlay size={32} strokeWidth={1.3} /><h2>Capture a real game clip</h2><ol><li>Open the game and paste <b>Load demo</b> into its console.</li><li>After it loads, paste <b>Seek to {time(tick / demo.tickRate)}</b>. Choose {player?.name || 'a player'} in the spectator controls.</li><li>Connect the game picture, resume playback in the game, then record.</li></ol></div>}
      {connected && <div className="game-live-label"><i />{recorder.recording ? `RECORDING ${recorder.elapsed.toFixed(0)} / ${length} s` : 'PREVIEW · POSITION THE GAME BEFORE RECORDING'}</div>}
    </div>
    <div className="game-connection"><label>Game window<select aria-label="Game window" value={sourceId} disabled={connected} onChange={e => setSourceId(e.target.value)}>{!windows.length && <option value="">No game window detected</option>}{windows.map(w => <option value={w.id} key={w.id}>{w.name}</option>)}</select></label><button className="icon-button" title="Refresh game windows" aria-label="Refresh game windows" disabled={busy || connected} onClick={() => void refresh()}><RefreshCw size={17} /></button><span>Video only · sound plays from the game</span></div>
    <div className="game-command-row"><button className="button subtle" disabled={!commands} title={commands?.play} onClick={() => commands && void copy(commands.play, 'Load command copied. Paste it into the game console.')}><Copy size={14} />Load demo</button><button className="button subtle" disabled={!commands} title={commands?.seek} onClick={() => commands && void copy(commands.seek, 'Seek command copied. Paste it after the demo has finished loading.')}><Copy size={14} />Seek to {time(tick / demo.tickRate)}</button><code>{commands?.seek ?? 'Loading playback commands…'}</code></div>
    <div className="game-command-row">{!connected ? <button className="button primary" disabled={!sourceId || busy} onClick={() => void connect()}><MonitorPlay size={15} />Connect game picture</button> : recorder.recording ? <button className="button record-active" onClick={recorder.stop}><Square size={14} />Stop and save ({recorder.elapsed.toFixed(0)} s)</button> : <><label className="record-length">Length<select aria-label="Clip length" value={length} disabled={recorder.saving} onChange={e => setLength(Number(e.target.value))}>{[10, 20, 30, 60, 120].map(seconds => <option key={seconds} value={seconds}>{seconds} seconds</option>)}</select></label><button className="button primary" disabled={busy || recorder.saving} onClick={() => stream.current && void recorder.start(stream.current, { demoId: demo.id, playerId: player?.id ?? '', reviewTick: Math.round(tick), title: `${player?.name || demo.map} · ${time(tick / demo.tickRate)}` }, length)}><Circle size={14} />{recorder.saving ? 'Saving clip…' : `Record ${length}s clip`}</button><button className="button subtle" disabled={recorder.saving} onClick={stop}>Disconnect</button></>}</div>
    <p className="game-sync-note">Position and resume the demo in the game before recording. Use windowed or borderless mode. Clips save video only; game audio is not recorded. Changing demos or leaving this view during capture discards the unfinished clip.</p>
    {status && <p className="game-status" role="status">{status}</p>}{error && <p className="game-error" role="alert"><TriangleAlert size={16} />{error}</p>}
  </section>;
}
