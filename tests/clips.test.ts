import { mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { afterEach, describe, expect, it } from 'vitest';
import { ClipStore } from '../electron/clips';
const dirs: string[] = [];
const reference = { demoId: 'match', playerId: 'player', reviewTick: 1234, title: 'A review moment' };
async function store() { const dir = await mkdtemp(join(tmpdir(), 'cs-review-clips-')); dirs.push(dir); const clips = new ClipStore(join(dir, 'clips')); await clips.initialize(); return { dir, clips }; }
afterEach(async () => { for (const dir of dirs.splice(0)) await rm(dir, { recursive: true, force: true }); });
describe('local recorded footage', () => {
  it('persists bounded recording chunks and rejects traversal', async () => {
    const { clips } = await store(); const id = await clips.begin(reference);
    const bytes = Buffer.alloc(2048); bytes.set([0x1a, 0x45, 0xdf, 0xa3]);
    await clips.append(id, bytes); const clip = await clips.finish(id, 2);
    expect(clip.reviewTick).toBe(1234); expect(clip.url).toBe(`democlip://video/${id}`);
    expect(await readFile(await clips.mediaPath(id))).toEqual(bytes);
    expect(await clips.list('other-match')).toEqual([]); expect(await clips.list('match')).toHaveLength(1);
    await expect(clips.mediaPath('../outside')).rejects.toThrow();
  });
  it('keeps imported originals and removes only managed copies', async () => {
    const { dir, clips } = await store(); const original = join(dir, 'original ü.mp4'); await writeFile(original, 'video');
    const clip = await clips.import(original, reference); await clips.remove(clip.id);
    expect(await readFile(original, 'utf8')).toBe('video'); expect(await clips.list('match')).toEqual([]);
  });
  it('cleans cancelled recordings and rejects invalid recordings', async () => {
    const { clips } = await store(); const id = await clips.begin(reference);
    await expect(clips.begin(reference)).rejects.toThrow(); await expect(clips.append(id, new Uint8Array())).rejects.toThrow();
    await clips.append(id, Buffer.alloc(2048)); await expect(clips.finish(id, 1)).rejects.toThrow('valid WebM');
    await clips.discard(id); expect(await clips.list('match')).toEqual([]); await clips.close();
  });
  it('reserves the only recording before asynchronous file creation', async () => {
    const { dir, clips } = await store();
    const starts = await Promise.allSettled([clips.begin(reference), clips.begin(reference)]);
    expect(starts.filter(result => result.status === 'fulfilled')).toHaveLength(1);
    expect(starts.filter(result => result.status === 'rejected')).toHaveLength(1);
    await clips.close(); expect(await readdir(join(dir, 'clips'))).toEqual([]);
  });
  it('seals chunks at finish and rejects a second finalization', async () => {
    const { dir, clips } = await store(); const id = await clips.begin(reference);
    const bytes = Buffer.alloc(2048); bytes.set([0x1a, 0x45, 0xdf, 0xa3]);
    const writing = clips.append(id, bytes);
    const finishing = clips.finish(id, 3);
    await expect(clips.append(id, Buffer.alloc(1024))).rejects.toThrow('no longer active');
    await expect(clips.finish(id, 3)).rejects.toThrow('no longer active');
    await writing; const clip = await finishing;
    expect(clip.size).toBe(bytes.byteLength);
    expect(await readFile(await clips.mediaPath(id))).toEqual(bytes);
    expect((await readdir(join(dir, 'clips'))).sort()).toEqual([`${id}.json`, `${id}.webm`]);
  });
  it('cancels in-flight writes and finalization without leaving footage', async () => {
    const { dir, clips } = await store(); const id = await clips.begin(reference);
    const bytes = Buffer.alloc(1024 * 1024); bytes.set([0x1a, 0x45, 0xdf, 0xa3]);
    const writing = clips.append(id, bytes);
    const finishing = clips.finish(id, 3);
    // Attach a rejection handler before discard waits on the finalization.
    const cancelled = expect(finishing).rejects.toThrow('cancelled');
    await clips.discard(id); await writing; await cancelled;
    expect(await readdir(join(dir, 'clips'))).toEqual([]);
    expect(await clips.list('match')).toEqual([]);
    const next = await clips.begin(reference); await clips.discard(next);
  });
  it('closes during recording creation and prevents later writes', async () => {
    const { dir, clips } = await store();
    const starting = clips.begin(reference);
    const cancelled = expect(starting).rejects.toThrow('cancelled');
    await clips.close(); await cancelled;
    expect(await readdir(join(dir, 'clips'))).toEqual([]);
    await expect(clips.begin(reference)).rejects.toThrow('closing');
    await expect(clips.import(join(dir, 'anything.mp4'), reference)).rejects.toThrow('closing');
  });
  it('closes an in-flight import without removing the original', async () => {
    const { dir, clips } = await store(); const original = join(dir, 'original.mp4'); await writeFile(original, 'video');
    const importing = clips.import(original, reference);
    const cancelled = expect(importing).rejects.toThrow('closing');
    await clips.close(); await cancelled;
    expect(await readdir(join(dir, 'clips'))).toEqual([]);
    expect(await readFile(original, 'utf8')).toBe('video');
  });
  it('owns queued chunk bytes and removes failed finalization artifacts', async () => {
    const { dir, clips } = await store(); const id = await clips.begin(reference);
    const bytes = Buffer.alloc(2048); bytes.set([0x1a, 0x45, 0xdf, 0xa3]);
    const writing = clips.append(id, bytes); bytes.fill(0);
    await writing; const clip = await clips.finish(id, 2);
    expect((await readFile(await clips.mediaPath(clip.id))).subarray(0, 4).toString('hex')).toBe('1a45dfa3');
    await clips.remove(clip.id);
    const invalid = await clips.begin(reference); await clips.append(invalid, Buffer.alloc(2048));
    await expect(clips.finish(invalid, 2)).rejects.toThrow('valid WebM');
    expect(await readdir(join(dir, 'clips'))).toEqual([]);
    const next = await clips.begin(reference); await clips.discard(next);
  });
  it('enforces chunk, recording and duration bounds', async () => {
    const { dir, clips } = await store(); const id = await clips.begin(reference);
    const chunk = Buffer.alloc(8 * 1024 * 1024); chunk.set([0x1a, 0x45, 0xdf, 0xa3]);
    await expect(clips.append(id, Buffer.alloc(chunk.length + 1))).rejects.toThrow('Invalid video chunk');
    for (let index = 0; index < 20; index++) await clips.append(id, chunk);
    await expect(clips.append(id, Buffer.alloc(1))).rejects.toThrow('160 MB');
    for (const duration of [0, -1, Number.NaN, Number.POSITIVE_INFINITY, 131]) await expect(clips.finish(id, duration)).rejects.toThrow('duration');
    await clips.discard(id); expect(await readdir(join(dir, 'clips'))).toEqual([]);
  }, 20_000);
  it('rejects traversal in metadata and keeps good clips visible beside corrupted metadata', async () => {
    const { dir, clips } = await store(); const source = join(dir, 'original.mp4'); await writeFile(source, 'video');
    const clip = await clips.import(source, reference);
    const metadata = join(dir, 'clips', `${clip.id}.json`);
    const goodMetadata = JSON.parse(await readFile(metadata, 'utf8'));
    await writeFile(metadata, JSON.stringify({ ...goodMetadata, extension: '/../original.mp4' }));
    await expect(clips.mediaPath(clip.id)).rejects.toThrow('metadata');
    await expect(clips.remove(clip.id)).rejects.toThrow('metadata');
    expect(await readFile(source, 'utf8')).toBe('video');
    expect(await clips.list('match')).toEqual([]);
    await writeFile(metadata, JSON.stringify(goodMetadata));
    await writeFile(join(dir, 'clips', '00000000-0000-0000-0000-000000000000.json'), '{invalid');
    expect(await clips.list('match')).toHaveLength(1);
    const broken = await clips.import(source, reference);
    await writeFile(join(dir, 'clips', `${broken.id}.json`), JSON.stringify({ ...goodMetadata, id: broken.id, createdAt: 42 }));
    expect(await clips.list('match')).toHaveLength(1);
    await expect(clips.mediaPath('------------------------------------')).rejects.toThrow('identifier');
  });
});
