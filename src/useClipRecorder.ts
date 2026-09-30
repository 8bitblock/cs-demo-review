import { useEffect, useRef, useState } from 'react';
import type { ClipReference, GameClip } from '../shared/types';
type Active = { id: string; recorder: MediaRecorder; started: number; abort: boolean; chunks: Promise<void>; error: unknown; timer: ReturnType<typeof setTimeout> };
export function useClipRecorder(onSaved: (clip: GameClip) => void, onError: (error: unknown) => void) {
  const [recording, setRecording] = useState(false), [saving, setSaving] = useState(false), [elapsed, setElapsed] = useState(0);
  const active = useRef<Active | null>(null), alive = useRef(true), callbacks = useRef({ onSaved, onError }); callbacks.current = { onSaved, onError };
  useEffect(() => {
    alive.current = true;
    return () => { alive.current = false; const value = active.current; if (value) { value.abort = true; clearTimeout(value.timer); if (value.recorder.state !== 'inactive') value.recorder.stop(); } };
  }, []);
  useEffect(() => { if (!recording) return; const timer = setInterval(() => setElapsed((performance.now() - (active.current?.started ?? performance.now())) / 1000), 250); return () => clearInterval(timer); }, [recording]);
  const stop = () => { if (active.current?.recorder.state === 'recording') active.current.recorder.stop(); };
  const start = async (media: MediaStream, reference: ClipReference, seconds: number) => {
    if (active.current || saving) return;
    setSaving(true); let id = '';
    try {
      const mimeType = ['video/webm;codecs=vp9', 'video/webm;codecs=vp8', 'video/webm'].find(type => MediaRecorder.isTypeSupported(type));
      if (!mimeType) throw new Error('WebM recording is unavailable. Import an existing MP4 or WebM clip.');
      id = await window.csDemo.beginClipRecording(reference);
      if (!alive.current || !media.active) { await window.csDemo.discardClipRecording(id); return; }
      const recorder = new MediaRecorder(media, { mimeType, videoBitsPerSecond: 6_000_000 });
      const value: Active = { id, recorder, started: performance.now(), abort: false, chunks: Promise.resolve(), error: null, timer: setTimeout(() => { if (recorder.state !== 'inactive') recorder.stop(); }, seconds * 1000) };
      active.current = value;
      recorder.ondataavailable = event => {
        if (!event.data.size) return;
        value.chunks = value.chunks.then(async () => {
          if (value.abort || value.error) return;
          const bytes = new Uint8Array(await event.data.arrayBuffer());
          for (let offset = 0; offset < bytes.length; offset += 4 * 1024 * 1024) await window.csDemo.appendClipRecording(id, bytes.slice(offset, offset + 4 * 1024 * 1024));
        }).catch(e => { value.error = e; if (recorder.state !== 'inactive') recorder.stop(); });
      };
      recorder.onerror = () => { value.error = new Error('The game video encoder failed.'); if (recorder.state !== 'inactive') recorder.stop(); };
      recorder.onstop = () => {
        clearTimeout(value.timer); const duration = (performance.now() - value.started) / 1000;
        if (alive.current) { setRecording(false); setSaving(true); }
        void value.chunks.then(async () => {
          if (value.abort || !alive.current || value.error) { await window.csDemo.discardClipRecording(id); if (value.error) throw value.error; return; }
          const clip = await window.csDemo.finishClipRecording(id, duration);
          if (alive.current) callbacks.current.onSaved(clip);
        }).catch(async e => { await window.csDemo.discardClipRecording(id).catch(() => {}); if (alive.current) callbacks.current.onError(e); }).finally(() => { if (active.current === value) active.current = null; if (alive.current) setSaving(false); });
      };
      recorder.start(1000); setRecording(true); setElapsed(0);
    } catch (e) {
      if (active.current?.id === id) { clearTimeout(active.current.timer); active.current = null; }
      if (id) await window.csDemo.discardClipRecording(id).catch(() => {});
      if (alive.current) { setRecording(false); callbacks.current.onError(e); }
    }
    finally { if (alive.current) setSaving(false); }
  };
  return { recording, saving, elapsed, start, stop };
}
