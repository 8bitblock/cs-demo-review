import { open, type FileHandle } from 'node:fs/promises';

// Matroska Duration is a float measured in TimestampScale units. Chromium's
// MediaRecorder omits it in streaming WebM. Keep encoded frame data unchanged.
// https://www.matroska.org/technical/elements.html
// https://www.rfc-editor.org/rfc/rfc8794.html
const EBML = 0x1a45dfa3, SEGMENT = 0x18538067, INFO = 0x1549a966, CLUSTER = 0x1f43b675;
const SEEK = 0x114d9b74, CUES = 0x1c53bb6b, VOID = 0xec, CRC = 0xbf;
const LEVEL_ONE = new Set([INFO, CLUSTER, SEEK, CUES, 0x1654ae6b, 0x1254c367, 0x1941a469, 0x1043a770]);
class UnsupportedWebM extends Error {}
interface Element { id: number; start: number; data: number; size: number | null; sizeLength: number; end: number | null }
function vint(bytes: Buffer, offset: number, id: boolean) {
  let length = 1, mask = 128;
  while (length <= 8 && !(bytes[offset] & mask)) { length++; mask >>= 1; }
  if (length > (id ? 4 : 8) || offset + length > bytes.length) throw new UnsupportedWebM();
  let value = BigInt(id ? bytes[offset] : bytes[offset] & (mask - 1));
  for (let i = 1; i < length; i++) value = value * 256n + BigInt(bytes[offset + i]);
  const unknown = !id && value === (1n << BigInt(7 * length)) - 1n;
  if (!unknown && value > BigInt(Number.MAX_SAFE_INTEGER)) throw new UnsupportedWebM();
  return { length, value: unknown ? null : Number(value) };
}
function sizeBytes(size: number, minimum = 1) {
  let length = minimum;
  while (length <= 8 && BigInt(size) >= (1n << BigInt(length * 7)) - 1n) length++;
  if (length > 8) throw new UnsupportedWebM();
  let value = BigInt(size) | (1n << BigInt(length * 7));
  const bytes = Buffer.alloc(length);
  for (let i = length - 1; i >= 0; i--) { bytes[i] = Number(value & 255n); value >>= 8n; }
  return bytes;
}
class Reader {
  private cache = Buffer.alloc(0); private start = 0;
  constructor(readonly file: FileHandle, readonly size: number) {}
  async bytes(position: number, count: number) {
    if (position < 0 || count < 0 || count > 65536 || position + count > this.size) throw new UnsupportedWebM();
    if (position < this.start || position + count > this.start + this.cache.length) {
      this.start = position; this.cache = Buffer.alloc(Math.min(65536, this.size - position));
      const { bytesRead } = await this.file.read(this.cache, 0, this.cache.length, position);
      if (bytesRead < count) throw new UnsupportedWebM();
      this.cache = this.cache.subarray(0, bytesRead);
    }
    return this.cache.subarray(position - this.start, position - this.start + count);
  }
  async element(position: number): Promise<Element> {
    const header = await this.bytes(position, Math.min(12, this.size - position));
    const id = vint(header, 0, true), size = vint(header, id.length, false);
    const data = position + id.length + size.length, end = size.value === null ? null : data + size.value;
    if (end !== null && end > this.size) throw new UnsupportedWebM();
    return { id: id.value!, start: position, data, size: size.value, sizeLength: size.length, end };
  }
}
async function writeAll(file: FileHandle, bytes: Buffer, position: number) {
  let written = 0;
  while (written < bytes.length) {
    const result = await file.write(bytes, written, bytes.length - written, position + written);
    if (!result.bytesWritten) throw new Error('Unable to finalize recorded video.');
    written += result.bytesWritten;
  }
}

/** Repairs only unindexed MediaRecorder-style files. Unsupported indexed files
 * remain byte-for-byte unchanged, so SeekHead/Cues never become stale offsets.
 * Existing finite Duration can be retained without shifting any file bytes. */
export async function finalizeWebMDuration(path: string, seconds: number, assertActive = () => {}): Promise<boolean> {
  if (!Number.isFinite(seconds) || seconds <= 0 || seconds > 130) throw new Error('Invalid recorded video duration.');
  const file = await open(path, 'r+');
  try {
    const length = (await file.stat()).size;
    if (length > 160 * 1024 * 1024) throw new UnsupportedWebM();
    const reader = new Reader(file, length);
    const header = await reader.element(0);
    if (header.id !== EBML || header.end === null) throw new UnsupportedWebM();
    const segment = await reader.element(header.end);
    if (segment.id !== SEGMENT || (segment.end !== null && segment.end !== length)) throw new UnsupportedWebM();
    let info: Element | undefined, firstCluster = Infinity, clusterEnd: number | null | undefined;
    let indexed = false, position = segment.data, count = 0;
    while (position < length) {
      if (++count > 1_000_000) throw new UnsupportedWebM();
      assertActive();
      const element = await reader.element(position);
      if (clusterEnd === position) clusterEnd = undefined;
      if (clusterEnd === null && LEVEL_ONE.has(element.id)) clusterEnd = undefined;
      if (clusterEnd !== undefined) {
        if (element.end === null || (clusterEnd !== null && element.end > clusterEnd)) throw new UnsupportedWebM();
        if (element.id === 0xa7) indexed = true; // Deprecated Cluster Position also contains absolute offsets.
      } else {
        if (element.id === SEEK || element.id === CUES || element.id === CRC) indexed = true;
        if (element.id === INFO) { if (info || element.size === null || element.size > 65536) throw new UnsupportedWebM(); info = element; }
        if (element.id === CLUSTER) { firstCluster = Math.min(firstCluster, position); clusterEnd = element.end; position = element.data; continue; }
        if (!LEVEL_ONE.has(element.id) && element.id !== VOID && element.id !== CRC) throw new UnsupportedWebM();
      }
      if (element.end === null || element.end <= position) throw new UnsupportedWebM();
      position = element.end;
    }
    if (!info || info.end === null || info.start > firstCluster) throw new UnsupportedWebM();
    let scale = 1_000_000, duration: Element | undefined, infoCRC = false;
    for (let offset = info.data; offset < info.end;) {
      const child = await reader.element(offset);
      if (child.end === null || child.end > info.end) throw new UnsupportedWebM();
      if (child.id === CRC) infoCRC = true;
      if (child.id === 0x4489) { if (duration || (child.size !== 4 && child.size !== 8)) throw new UnsupportedWebM(); duration = child; }
      if (child.id === 0x2ad7b1) {
        if (!child.size || child.size > 6) throw new UnsupportedWebM();
        scale = (await reader.bytes(child.data, child.size)).readUIntBE(0, child.size);
        if (!scale) throw new UnsupportedWebM();
      }
      offset = child.end;
    }
    if (infoCRC || indexed) return false;
    const ticks = seconds * 1e9 / scale;
    if (duration) {
      const bytes = Buffer.from(await reader.bytes(duration.data, duration.size!));
      const existing = bytes.length === 4 ? bytes.readFloatBE(0) : bytes.readDoubleBE(0);
      if (Number.isFinite(existing) && existing > 0) return true;
      if (bytes.length === 4) bytes.writeFloatBE(ticks, 0); else bytes.writeDoubleBE(ticks, 0);
      assertActive(); await writeAll(file, bytes, duration.data); return true;
    }
    const added = Buffer.alloc(11); added.set([0x44, 0x89, 0x88]); added.writeDoubleBE(ticks, 3);
    const encodedSize = sizeBytes(info.size! + added.length, info.sizeLength);
    const delta = added.length + encodedSize.length - info.sizeLength;
    const segmentSize = segment.size === null ? null : sizeBytes(segment.size + delta, segment.sizeLength);
    if (segmentSize && segmentSize.length !== segment.sizeLength) return false;
    const replacement = Buffer.concat([Buffer.from([0x15, 0x49, 0xa9, 0x66]), encodedSize, Buffer.from(await reader.bytes(info.data, info.size!)), added]);
    // Work backwards so an in-place insert cannot overwrite bytes still to be
    // copied. Peak data buffer is 1 MiB, independent of clip duration.
    assertActive(); await file.truncate(length + delta);
    const chunk = Buffer.alloc(1024 * 1024);
    for (let end = length; end > info.end;) {
      assertActive(); const start = Math.max(info.end, end - chunk.length), amount = end - start;
      const { bytesRead } = await file.read(chunk, 0, amount, start);
      if (bytesRead !== amount) throw new Error('Recorded video changed during finalization.');
      await writeAll(file, chunk.subarray(0, amount), start + delta); end = start;
    }
    assertActive(); await writeAll(file, replacement, info.start);
    if (segmentSize) await writeAll(file, segmentSize, segment.data - segment.sizeLength);
    return true;
  } catch (error) { if (error instanceof UnsupportedWebM) return false; throw error; }
  finally { await file.close(); }
}
