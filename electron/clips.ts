import { mkdir, readFile, writeFile, rename, stat, copyFile, appendFile, rm, readdir, open } from 'node:fs/promises';
import { join, extname } from 'node:path';
import { randomUUID } from 'node:crypto';
import { z } from 'zod';
import type { ClipReference, GameClip } from '../shared/types';
import { finalizeWebMDuration } from './webm';

type StoredClip = Omit<GameClip, 'url'> & { extension: '.webm' | '.mp4' };
const MAX_RECORDING_BYTES = 160 * 1024 * 1024;
const isId = (id: string) => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(id);
const storedClip = z.object({
  id: z.string().refine(isId), demoId: z.string().min(1).max(160), playerId: z.string().max(160),
  title: z.string().min(1).max(200), reviewTick: z.number().int().min(0).max(2_147_483_647),
  createdAt: z.string().datetime(), duration: z.number().finite().min(0).max(130),
  size: z.number().int().positive().max(4 * 1024 ** 3), source: z.enum(['imported', 'recorded']),
  extension: z.enum(['.webm', '.mp4']),
});
type Recording = { reference: ClipReference; size: number; chain: Promise<void>; state: 'recording' | 'finishing' | 'discarding'; finalizing?: Promise<GameClip> };
export class ClipStore {
  private pending = new Map<string, Recording>();
  private closed = false;
  private imports = new Set<Promise<GameClip>>();
  constructor(private readonly directory: string) {}
  async initialize() {
    await mkdir(this.directory, { recursive: true });
    // Only remove our unfinished recordings, never imported source files.
    for (const name of await readdir(this.directory)) if (/^[0-9a-f-]{36}\.part$/.test(name)) await rm(join(this.directory, name), { force: true });
  }
  private path(id: string, suffix: string) {
    if (!isId(id)) throw new Error('Invalid clip identifier.');
    return join(this.directory, id + suffix);
  }
  private async read(id: string): Promise<StoredClip> {
    const metadata = this.path(id, '.json');
    if ((await stat(metadata)).size > 64 * 1024) throw new Error('Invalid clip metadata.');
    const parsed = storedClip.safeParse(JSON.parse(await readFile(metadata, 'utf8')));
    if (!parsed.success || parsed.data.id !== id) throw new Error('Invalid clip metadata.');
    return parsed.data;
  }
  private publish(clip: StoredClip): GameClip {
    const { extension: _extension, ...info } = clip;
    return { ...info, url: `democlip://video/${clip.id}` };
  }
  async list(demoId: string) {
    const clips: GameClip[] = [];
    for (const name of await readdir(this.directory)) {
      if (!/^[0-9a-f-]{36}\.json$/.test(name)) continue;
      try { const clip = await this.read(name.slice(0, -5)); if (clip.demoId === demoId) { await stat(this.path(clip.id, clip.extension)); clips.push(this.publish(clip)); } } catch { /* A missing clip must not hide other saved footage. */ }
    }
    return clips.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
  }
  async mediaPath(id: string) { const clip = await this.read(id); return this.path(id, clip.extension); }
  private async save(clip: StoredClip) {
    await writeFile(this.path(clip.id, '.json.tmp'), JSON.stringify(clip, null, 2));
    await rename(this.path(clip.id, '.json.tmp'), this.path(clip.id, '.json'));
    return this.publish(clip);
  }
  private async clearFiles(id: string) {
    await Promise.all(['.part', '.webm', '.mp4', '.json', '.json.tmp'].map(suffix => rm(this.path(id, suffix), { force: true })));
  }
  async import(source: string, reference: ClipReference) {
    if (this.closed) throw new Error('Clip storage is closing.');
    const operation = this.importFile(source, { ...reference });
    this.imports.add(operation);
    try { return await operation; } finally { this.imports.delete(operation); }
  }
  private async importFile(source: string, reference: ClipReference) {
    const extension = extname(source).toLowerCase();
    if (extension !== '.webm' && extension !== '.mp4') throw new Error('Choose an MP4 or WebM video.');
    const info = await stat(source);
    if (!info.isFile() || info.size === 0 || info.size > 4 * 1024 ** 3) throw new Error('Choose a non-empty clip smaller than 4 GB.');
    const id = randomUUID();
    try {
      if (this.closed) throw new Error('Clip storage is closing.');
      await copyFile(source, this.path(id, extension));
      if (this.closed) throw new Error('Clip storage is closing.');
      const copied = await stat(this.path(id, extension));
      if (!copied.isFile() || copied.size === 0 || copied.size > 4 * 1024 ** 3) throw new Error('The copied video is empty or exceeds 4 GB.');
      const result = await this.save({ ...reference, id, extension, source: 'imported', createdAt: new Date().toISOString(), size: copied.size, duration: 0 });
      if (this.closed) throw new Error('Clip storage is closing.');
      return result;
    } catch (e) { await this.clearFiles(id); throw e; }
  }
  async begin(reference: ClipReference) {
    if (this.closed) throw new Error('Clip storage is closing.');
    if (this.pending.size) throw new Error('Finish the current recording first.');
    const id = randomUUID();
    const recording: Recording = { reference: { ...reference }, size: 0, chain: Promise.resolve(), state: 'recording' };
    // Reserve synchronously: a second begin or shutdown can arrive while the
    // initial file creation is in flight.
    this.pending.set(id, recording);
    recording.chain = writeFile(this.path(id, '.part'), new Uint8Array(), { flag: 'wx' });
    try {
      await recording.chain;
      if (recording.state !== 'recording' || this.closed) throw new Error('Recording was cancelled.');
      return id;
    } catch (e) { this.pending.delete(id); await this.clearFiles(id); throw e; }
  }
  async append(id: string, data: Uint8Array) {
    const recording = this.pending.get(id);
    if (!recording || recording.state !== 'recording' || this.closed) throw new Error('Recording is no longer active.');
    if (!(data instanceof Uint8Array) || data.byteLength === 0 || data.byteLength > 8 * 1024 * 1024) throw new Error('Invalid video chunk.');
    if (recording.size + data.byteLength > MAX_RECORDING_BYTES) throw new Error('Recording reached its 160 MB limit.');
    recording.size += data.byteLength;
    const bytes = Buffer.from(data);
    recording.chain = recording.chain.then(() => appendFile(this.path(id, '.part'), bytes));
    await recording.chain;
  }
  async finish(id: string, duration: number) {
    const recording = this.pending.get(id);
    if (!recording || recording.state !== 'recording' || this.closed) throw new Error('Recording is no longer active.');
    // The recorder stops at 120 seconds; allow its final data event to arrive
    // slightly later without throwing away an otherwise valid recording.
    if (!Number.isFinite(duration) || duration <= 0 || duration > 130) throw new Error('Recording duration is invalid or exceeded the 120-second limit and finalization allowance.');
    recording.state = 'finishing';
    const active = () => { if (recording.state === 'discarding' || this.closed) throw new Error('Recording was cancelled.'); };
    recording.finalizing = (async () => {
      try {
        // State was sealed before awaiting, so no later append can escape the
        // awaited chain or recreate a .part file after its rename.
        await recording.chain; active();
        const file = await open(this.path(id, '.part'), 'r'); const header = Buffer.alloc(4);
        try { await file.read(header, 0, 4, 0); } finally { await file.close(); }
        active();
        if (recording.size < 1024 || header.toString('hex') !== '1a45dfa3') throw new Error('The game did not produce a valid WebM video.');
        await finalizeWebMDuration(this.path(id, '.part'), duration, active); active();
        recording.size = (await stat(this.path(id, '.part'))).size;
        await rename(this.path(id, '.part'), this.path(id, '.webm')); active();
        const result = await this.save({ ...recording.reference, id, extension: '.webm', source: 'recorded', createdAt: new Date().toISOString(), size: recording.size, duration });
        active(); return result;
      } catch (e) { await this.clearFiles(id); throw e; }
      finally { this.pending.delete(id); }
    })();
    return recording.finalizing;
  }
  async discard(id: string) {
    const recording = this.pending.get(id); if (!recording) return;
    recording.state = 'discarding';
    await recording.chain.catch(() => {});
    await recording.finalizing?.catch(() => {});
    await this.clearFiles(id); this.pending.delete(id);
  }
  async remove(id: string) {
    const clip = await this.read(id);
    await rm(this.path(id, clip.extension), { force: true }); await rm(this.path(id, '.json'), { force: true });
  }
  async close() {
    this.closed = true;
    await Promise.all([...this.pending.keys()].map(id => this.discard(id)));
    await Promise.allSettled([...this.imports]);
  }
}
