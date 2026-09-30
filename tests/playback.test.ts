import { describe, expect, it } from 'vitest';
import { gameLaunchArguments, parseNativeGameState, preparePlaybackCommands, sourceMatchesGameWindow } from '../electron/playback';

const executable = 'B:\\SteamLibrary\\cs2\\game\\bin\\win64\\cs2.exe';
const demo = { engine: 'cs2' as const, path: 'B:\\Demo copies\\match ü.dem', totalTicks: 12000 };

describe('native playback and capture boundaries', () => {
  it('preserves spaced Unicode paths and bounds seek to the demo', () => {
    const commands = preparePlaybackCommands(demo, 15000, executable, true, true);
    expect(commands.play).toBe('playdemo "B:/Demo copies/match ü.dem"');
    expect(commands.seek).toBe('demo_gototick 12000');
    expect(commands.available).toBe(true);
    expect(gameLaunchArguments(demo, executable).args).toEqual(['-steam', '-console', '+playdemo', 'B:/Demo copies/match ü.dem']);
  });
  it('rejects unsafe commands and wrong-engine executables', () => {
    expect(() => preparePlaybackCommands({ ...demo, path: 'B:\\match";quit.dem' }, 0, executable, true, true)).toThrow();
    expect(() => preparePlaybackCommands(demo, -1, executable, true, true)).toThrow();
    expect(() => gameLaunchArguments(demo, 'B:\\csgo.exe')).toThrow();
    expect(() => gameLaunchArguments(demo, 'cs2.exe')).toThrow();
  });
  it('keeps moved-demo and missing-game reasons actionable', () => {
    expect(preparePlaybackCommands(demo, 0, executable, true, false)).toMatchObject({ available: false, reason: expect.stringContaining('moved') });
    expect(preparePlaybackCommands(demo, 0, '', false, true)).toMatchObject({ available: false, reason: expect.stringContaining('Settings') });
  });
  it('matches capture to the executable and native window handle, never the title', () => {
    const state = parseNativeGameState({
      processes: [{ pid: 20, name: 'cs2.exe', executable: executable.toUpperCase() }, { pid: 21, name: 'cs2.exe', executable: 'B:\\Different copy\\cs2.exe' }],
      windows: [{ pid: 20, handle: '592054', title: 'Counter-Strike 2' }, { pid: 21, handle: '999', title: 'Counter-Strike 2' }, { pid: 55, handle: '777', title: 'Counter-Strike 2' }, { pid: 20, handle: '0', title: 'Hidden' }],
    }, 'cs2', executable);
    expect(state.running).toBe(true);
    expect(state.windows).toEqual([{ pid: 20, handle: '592054', title: 'Counter-Strike 2' }]);
    expect(sourceMatchesGameWindow('window:592054:0', state.windows)).toBe(true);
    expect(sourceMatchesGameWindow('screen:592054:0', state.windows)).toBe(false);
    expect(sourceMatchesGameWindow('window:777:0', state.windows)).toBe(false);
  });
  it('detects an existing inaccessible process without granting capture', () => {
    const state = parseNativeGameState({ processes: [{ pid: 20, name: 'cs2.exe', executable: '' }], windows: [{ pid: 20, handle: '592054', title: 'Counter-Strike 2' }] }, 'cs2', executable);
    expect(state.running).toBe(true);
    expect(state.windows).toEqual([]);
    expect(() => parseNativeGameState({ processes: {}, windows: [] }, 'cs2', executable)).toThrow();
  });
});
