/** A renderer must explicitly select one verified game window before requesting video. */
export function permitsGameCapture(permission: string, ownsWindow: boolean, hasGrant: boolean, details: { isMainFrame?: boolean; mediaTypes?: unknown }) {
  if (!ownsWindow || !hasGrant || details.isMainFrame !== true) return false;
  // Electron 44 asks for "media" with no device types before its desktop
  // capture handler. Device camera/microphone requests must stay denied.
  return permission === 'display-capture' || (permission === 'media' && Array.isArray(details.mediaTypes) && details.mediaTypes.length === 0);
}
export class CaptureGrant {
  private grant: { demoId: string; sourceId: string; expiresAt: number } | null = null;
  constructor(private readonly now: () => number = Date.now) {}
  prepare(demoId: string, sourceId: string) {
    if (!/^window:\d+:\d+$/.test(sourceId)) throw new Error('Select a game window, not a desktop screen.');
    this.grant = { demoId, sourceId, expiresAt: this.now() + 30_000 };
  }
  available() { return !!this.grant && this.grant.expiresAt > this.now(); }
  consume() { const value = this.available() ? this.grant : null; this.clear(); return value; }
  clear() { this.grant = null; }
}
