import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { finalizeWebMDuration } from '../electron/webm';

const directories: string[] = [];
const header = Buffer.from('1a45dfa39f4286810142f7810142f2810442f381084282847765626d4287810442858102', 'hex');
const infoData = Buffer.from('2ad7b1830f42404d80864368726f6d655741864368726f6d65', 'hex');
const unknown = Buffer.from('01ffffffffffffff', 'hex');
const cluster = Buffer.from('1f43b67501ffffffffffffffe78100a38481000080', 'hex');
function fixture(options: { finite?: boolean; indexed?: boolean; duration?: number; infoPadding?: number } = {}) {
  const extra = options.duration === undefined ? Buffer.alloc(0) : Buffer.from('4489880000000000000000', 'hex');
  if (extra.length) extra.writeDoubleBE(options.duration!, 3);
  const padding = options.infoPadding ? Buffer.concat([Buffer.from([0xec, 0x80 | options.infoPadding]), Buffer.alloc(options.infoPadding)]) : Buffer.alloc(0);
  const data = Buffer.concat([infoData, padding, extra]);
  const info = Buffer.concat([Buffer.from('1549a966', 'hex'), Buffer.from([0x80 | data.length]), data]);
  const segmentData = Buffer.concat([info, options.indexed ? Buffer.from('114d9b7480', 'hex') : Buffer.alloc(0), cluster]);
  const segmentSize = options.finite ? Buffer.from([0x40 | (segmentData.length >> 8), segmentData.length & 255]) : unknown;
  return { bytes: Buffer.concat([header, Buffer.from('18538067', 'hex'), segmentSize, segmentData]), infoEnd: header.length + 4 + segmentSize.length + info.length };
}
async function file(bytes: Buffer) { const directory = await mkdtemp(join(tmpdir(), 'cs-webm-duration-')); directories.push(directory); const path = join(directory, 'clip.webm'); await writeFile(path, bytes); return path; }
afterEach(async () => { for (const directory of directories.splice(0)) await rm(directory, { recursive: true, force: true }); });

describe('recorded WebM duration finalization', () => {
  it('adds duration without changing recorded cluster bytes', async () => {
    const original = fixture(), path = await file(original.bytes);
    expect(await finalizeWebMDuration(path, 10.01)).toBe(true);
    const result = await readFile(path), duration = result.indexOf(Buffer.from('448988', 'hex'));
    expect(result.readDoubleBE(duration + 3)).toBeCloseTo(10010);
    expect(result.length).toBe(original.bytes.length + 11);
    expect(result.subarray(original.infoEnd + 11)).toEqual(original.bytes.subarray(original.infoEnd));
    expect(await finalizeWebMDuration(path, 20)).toBe(true);
    expect(await readFile(path)).toEqual(result); // Valid existing duration is preserved.
  });
  it('updates finite segment sizes and widens Info size only when needed', async () => {
    const original = fixture({ finite: true, infoPadding: 95 }), path = await file(original.bytes);
    expect(await finalizeWebMDuration(path, 2)).toBe(true);
    const result = await readFile(path);
    expect(result.length).toBe(original.bytes.length + 12);
    expect(result.subarray(original.infoEnd + 12)).toEqual(original.bytes.subarray(original.infoEnd));
    const sizeOffset = header.length + 4;
    expect(((result[sizeOffset] & 63) << 8) + result[sizeOffset + 1]).toBe(result.length - sizeOffset - 2);
  });
  it('replaces an empty existing duration without shifting bytes', async () => {
    const original = fixture({ duration: 0 }), path = await file(original.bytes);
    expect(await finalizeWebMDuration(path, 3)).toBe(true);
    const result = await readFile(path), duration = result.indexOf(Buffer.from('448988', 'hex'));
    expect(result.readDoubleBE(duration + 3)).toBe(3000);
    expect(result.length).toBe(original.bytes.length);
  });
  it('leaves indexed or malformed files unchanged', async () => {
    for (const original of [fixture({ indexed: true }).bytes, Buffer.from('1a45dfa300000000', 'hex')]) {
      const path = await file(original);
      expect(await finalizeWebMDuration(path, 3)).toBe(false);
      expect(await readFile(path)).toEqual(original);
    }
  });
  it('honors cancellation before modifying footage', async () => {
    const original = fixture(), path = await file(original.bytes);
    await expect(finalizeWebMDuration(path, 3, () => { throw new Error('cancelled'); })).rejects.toThrow('cancelled');
    expect(await readFile(path)).toEqual(original.bytes);
  });
  it.skipIf(!existsSync(resolve('artifacts/validation-existing-game-capture.webm')))('repairs the actual Chromium game capture and preserves every frame byte', async () => {
    const original = await readFile(resolve('artifacts/validation-existing-game-capture.webm')), path = await file(original);
    const originalDuration = original.subarray(0, 1024).indexOf(Buffer.from('448988', 'hex'));
    const alreadyFinalized = originalDuration >= 0 && Number.isFinite(original.readDoubleBE(originalDuration + 3)) && original.readDoubleBE(originalDuration + 3) > 0;
    const finalized = await finalizeWebMDuration(path, 10.01);
    const result = await readFile(path), index = result.indexOf(Buffer.from('448988', 'hex'));
    if (alreadyFinalized) {
      expect(result.equals(original)).toBe(true);
    } else {
      expect(finalized).toBe(true);
      expect(result.readDoubleBE(index + 3)).toBeCloseTo(10010);
      expect(result.subarray(index + 11).equals(original.subarray(index))).toBe(true);
      expect(result.length).toBe(original.length + 11);
    }
  });
});
