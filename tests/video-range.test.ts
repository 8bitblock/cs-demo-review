import { describe, expect, it } from 'vitest';
import { parseVideoRange } from '../electron/video-range';
describe('recorded video byte ranges', () => {
  it('supports closed, open-ended and suffix ranges with inclusive bounds', () => {
    expect(parseVideoRange(null, 100)).toBeNull();
    expect(parseVideoRange('bytes=0-9', 100)).toEqual({ start: 0, end: 9 });
    expect(parseVideoRange('bytes=50-', 100)).toEqual({ start: 50, end: 99 });
    expect(parseVideoRange('bytes=-10', 100)).toEqual({ start: 90, end: 99 });
    expect(parseVideoRange('bytes=-200', 100)).toEqual({ start: 0, end: 99 });
    expect(parseVideoRange('bytes=95-999', 100)).toEqual({ start: 95, end: 99 });
  });
  it('rejects invalid, multiple, overflowing and out-of-file ranges', () => {
    for (const range of ['bytes=100-', 'bytes=40-30', 'bytes=-0', 'bytes=-', 'bytes=1-4,8-9', 'bytes=9007199254740992-', 'items=0-1']) expect(() => parseVideoRange(range, 100)).toThrow(RangeError);
    expect(() => parseVideoRange('bytes=0-', 0)).toThrow(RangeError);
  });
});
