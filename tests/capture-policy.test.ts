import { describe, expect, it } from 'vitest';
import { CaptureGrant, permitsGameCapture } from '../electron/capture-policy';
describe('game-window capture consent', () => {
  it('allows the verified desktop request without enabling camera or microphone access', () => {
    expect(permitsGameCapture('media', true, true, { isMainFrame: true, mediaTypes: [] })).toBe(true);
    for (const mediaTypes of [undefined, ['video'], ['audio'], ['video', 'audio']]) expect(permitsGameCapture('media', true, true, { isMainFrame: true, mediaTypes })).toBe(false);
    expect(permitsGameCapture('media', true, false, { isMainFrame: true, mediaTypes: [] })).toBe(false);
    expect(permitsGameCapture('media', false, true, { isMainFrame: true, mediaTypes: [] })).toBe(false);
    expect(permitsGameCapture('media', true, true, { isMainFrame: false, mediaTypes: [] })).toBe(false);
  });
  it('requires an explicit, single-use game-window grant', () => {
    const grant = new CaptureGrant();
    expect(grant.consume()).toBeNull();
    expect(() => grant.prepare('demo', 'screen:0:0')).toThrow();
    grant.prepare('demo', 'window:54321:0');
    expect(grant.consume()?.sourceId).toBe('window:54321:0');
    expect(grant.consume()).toBeNull();
  });
  it('expires and can be revoked when changing views', () => {
    let now = 100; const grant = new CaptureGrant(() => now);
    grant.prepare('demo', 'window:42:0'); now += 30_001;
    expect(grant.consume()).toBeNull();
    grant.prepare('demo', 'window:42:0'); grant.clear();
    expect(grant.available()).toBe(false);
  });
});
