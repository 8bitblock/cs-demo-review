export type Engine = 'cs2' | 'csgo';
export type Verdict = 'Low concern' | 'Suspicious' | 'Highly suspicious' | 'Insufficient data' | 'Reviewed with limits';
export type Signal = 'reaction' | 'acquisition' | 'aim-snap' | 'recoil' | 'shot-direction';
export type CapabilityStatus = 'available' | 'limited' | 'measured' | 'insufficient' | 'unsupported';
export interface Capability { signal: Signal; status: CapabilityStatus; reason: string; samples: number; measuredSamples?: number; basis?: string }
export interface Demo { id: string; name: string; path: string; engine: Engine; map: string; date: string; duration: number; tickRate: number; totalTicks: number; status: 'ready' | 'partial'; playerCount: number; roundCount: number; fileSize: number; hash: string; parserVersion: string; analysisVersion: string; recordingType: string; mapVersion: string; warnings: string[] }
export interface ReviewMetric { label: string; value: number; unit: string; samples: number; signal?: Signal; provenance?: string }
export interface ReviewClip { id: string; round: number; tick: number; endTick: number; time: number; title: string; description: string; measurements: { label: string; value: number; unit: string }[]; limitations: string[]; signal?: Signal; provenance?: string }
export interface PlayerReview { summary: string; totalShots: number; sampledShots: number; eligibleAimShots: number; coveredRounds: number; medianSampleMs: number | null; metrics: ReviewMetric[]; exclusions: { reason: string; count: number }[]; clips: ReviewClip[] }
export interface Player { id: string; name: string; team: string; kills: number; deaths: number; assists: number; headshots: number; damage: number; verdict: Verdict; evidenceCount: number; capabilities: Capability[]; coverage: string; review?: PlayerReview }
export interface Round { number: number; startTick: number; endTick: number; winner: string; reason: string }
export interface Vec3 { x: number; y: number; z: number }
export interface Sample { tick: number; time: number; playerId: string; x: number; y: number; z: number; yaw: number; pitch: number; health: number; armor: number; team: string; alive: boolean; weapon: string; scoped: boolean; flashed: boolean; crouching: boolean; velocity: number; eye?: Vec3; recoilIndex?: number; aimPunch?: Vec3; spottedBy?: string[]; spottedKnown?: boolean }
export interface GameEvent { id: string; tick: number; time: number; round: number; kind: string; playerId?: string; targetId?: string; weapon?: string; x?: number; y?: number; z?: number; text: string; headshot?: boolean }
export interface Shot { id: string; tick: number; time: number; round: number; playerId: string; weapon: string; origin?: Vec3; viewAngles?: Vec3; nativeAngles?: Vec3; aimPunch?: Vec3; aimPunchScale?: number; recoilIndex?: number; spread?: number; inaccuracy?: number; impacts: Vec3[]; timingPrecision: number; provenance: string; ambiguous: boolean }
export interface Finding { id: string; demoId: string; playerId: string; round: number; tick: number; endTick: number; time: number; signal: Signal; severity: 'review' | 'strong'; title: string; description: string; measurements: { label: string; value: number; unit: string }[]; alternatives: string[]; limitations: string[]; episodeId: string }
export interface ReviewNote { id: string; demoId: string; tick: number; playerId: string; text: string; kind: 'note' | 'bookmark'; createdAt: string }
export interface MatchDetail { demo: Demo; players: Player[]; rounds: Round[]; findings: Finding[]; events: GameEvent[]; notes: ReviewNote[] }
export interface ReplayWindow { samples: Sample[]; events: GameEvent[]; shots: Shot[]; fromTick: number; toTick: number }
export interface ImportProgress { jobId: string; path: string; name: string; stage: 'queued' | 'hashing' | 'parsing' | 'analysing' | 'complete' | 'error' | 'cancelled'; progress: number; message: string; demoId?: string }
export interface MapFloor { name: string; minZ: number; maxZ: number; image?: string }
export interface MapAsset { id: string; map: string; engine: Engine; version: string; source: string; verified: boolean; posX: number; posY: number; scale: number; rotate: number; floors: MapFloor[]; geometryPath?: string; image?: string; warnings: string[] }
export interface AppSettings { cs2Path: string; csgoPath: string; steamPath: string; source2ViewerPath: string; libraryPath: string }
export interface PlaybackCommands { play: string; seek: string; executable: string; available: boolean; reason: string }
export interface GameWindow { id: string; name: string }
export interface GameClip { id: string; demoId: string; playerId: string; title: string; reviewTick: number; createdAt: string; duration: number; size: number; source: 'recorded' | 'imported'; url: string }
export interface ClipReference { demoId: string; playerId: string; reviewTick: number; title: string }
export interface Diagnostics { appVersion: string; workerVersion: string; libraryPath: string; platform: string }
export interface DesktopAPI {
  listDemos(): Promise<Demo[]>;
  getMatch(id: string): Promise<MatchDetail>;
  getReplay(id: string, fromTick: number, toTick: number): Promise<ReplayWindow>;
  importDemos(): Promise<string[]>;
  importDropped(files: File[]): Promise<string[]>;
  cancelImport(jobId: string): Promise<void>;
  removeDemo(id: string): Promise<boolean>;
  saveNote(note: Omit<ReviewNote, 'id' | 'createdAt'> & { id?: string }): Promise<ReviewNote>;
  deleteNote(id: string): Promise<void>;
  exportReport(id: string, format: 'html' | 'json'): Promise<string | null>;
  getSettings(): Promise<AppSettings>;
  updateSettings(settings: Partial<AppSettings>): Promise<AppSettings>;
  choosePath(kind: 'cs2' | 'csgo' | 'source2viewer'): Promise<string | null>;
  getPlaybackCommands(id: string, tick: number): Promise<PlaybackCommands>;
  openInGame(id: string, tick: number): Promise<PlaybackCommands>;
  getGameWindows(id: string): Promise<GameWindow[]>;
  prepareGameCapture(id: string, sourceId: string): Promise<void>;
  cancelGameCapture(): Promise<void>;
  listClips(id: string): Promise<GameClip[]>;
  importClip(reference: ClipReference): Promise<GameClip | null>;
  beginClipRecording(reference: ClipReference): Promise<string>;
  appendClipRecording(id: string, data: Uint8Array): Promise<void>;
  finishClipRecording(id: string, duration: number): Promise<GameClip>;
  discardClipRecording(id: string): Promise<void>;
  removeClip(id: string): Promise<boolean>;
  copyText(text: string): Promise<void>;
  getMap(id: string): Promise<MapAsset | null>;
  importMapPack(): Promise<MapAsset | null>;
  extractMap(id: string): Promise<MapAsset>;
  reanalyse(id: string): Promise<void>;
  getDiagnostics(): Promise<Diagnostics>;
  exportDiagnostics(): Promise<string | null>;
  onImportProgress(callback: (progress: ImportProgress) => void): () => void;
  onWorkerError(callback: (message: string) => void): () => void;
}
declare global { interface Window { csDemo: DesktopAPI } }
