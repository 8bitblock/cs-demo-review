import { execFile, spawn } from 'node:child_process';
import { dirname, join, win32 } from 'node:path';
import { promisify } from 'node:util';
import type { Demo, Engine, PlaybackCommands } from '../shared/types';
import { consolePath } from './validation';

const execute = promisify(execFile);
export interface NativeGameWindow { handle: string; pid: number; title: string }
export interface NativeGameState {
  running: boolean;
  windows: NativeGameWindow[];
  processIds: number[];
  executable: string;
  reason: string;
}
type PlaybackDemo = Pick<Demo, 'engine' | 'path' | 'totalTicks'>;

function expectedName(engine: Engine) { return engine === 'cs2' ? 'cs2.exe' : 'csgo.exe'; }
export function validateGameExecutable(executable: string, engine: Engine) {
  if (!win32.isAbsolute(executable) || win32.basename(executable).toLowerCase() !== expectedName(engine)) {
    throw new Error(`Select the actual ${expectedName(engine)} game executable in Settings.`);
  }
}

export function preparePlaybackCommands(demo: PlaybackDemo, tick: number, executable: string, gameAvailable: boolean, demoAvailable: boolean): PlaybackCommands {
  if (!Number.isSafeInteger(tick) || tick < 0 || !Number.isSafeInteger(demo.totalTicks) || demo.totalTicks < 0) throw new Error('Playback requires a valid demo tick.');
  return {
    play: `playdemo "${consolePath(demo.path)}"`,
    seek: `demo_gototick ${Math.min(tick, demo.totalTicks)}`,
    executable,
    available: gameAvailable && demoAvailable,
    reason: !demoAvailable ? 'The original demo has moved. Re-import its new location to restore game playback.'
      : !gameAvailable ? `Set the ${demo.engine === 'cs2' ? 'CS2' : 'legacy CS:GO'} executable in Settings.`
      : 'Load the demo in the game console, then paste the seek command once playback has started.',
  };
}

// Fixed script only: no demo paths, player names, or other renderer strings enter PowerShell.
// EnumWindows is read-only and matches capture handles to an actual game's process ID.
const windowQuery = String.raw`
$ErrorActionPreference = 'Stop'
Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;
public class DemoGameWindow {
  public string handle;
  public int pid;
  public string title;
}
public static class DemoGameWindows {
  private delegate bool EnumWindowCallback(IntPtr hwnd, IntPtr lparam);
  [DllImport("user32.dll")] private static extern bool EnumWindows(EnumWindowCallback callback, IntPtr lparam);
  [DllImport("user32.dll")] private static extern bool IsWindowVisible(IntPtr hwnd);
  [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint processId);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] private static extern int GetWindowText(IntPtr hwnd, StringBuilder text, int count);
  public static DemoGameWindow[] Get() {
    var result = new List<DemoGameWindow>();
    EnumWindows((hwnd, unused) => {
      if (!IsWindowVisible(hwnd)) return true;
      uint pid;
      GetWindowThreadProcessId(hwnd, out pid);
      var title = new StringBuilder(1024);
      GetWindowText(hwnd, title, title.Capacity);
      result.Add(new DemoGameWindow { handle=hwnd.ToInt64().ToString(), pid=(int)pid, title=title.ToString() });
      return true;
    }, IntPtr.Zero);
    return result.ToArray();
  }
}
'@
$games = @()
foreach ($gameName in @('cs2', 'csgo')) {
  foreach ($gameProcess in [System.Diagnostics.Process]::GetProcessesByName($gameName)) {
    try {
      $games += [pscustomobject]@{ pid=$gameProcess.Id; name=$gameName + '.exe'; executable=$gameProcess.MainModule.FileName }
    } catch {
      $games += [pscustomobject]@{ pid=$gameProcess.Id; name=$gameName + '.exe'; executable='' }
    }
  }
}
$ids = @($games | ForEach-Object { $_.pid })
$windows = @([DemoGameWindows]::Get() | Where-Object { $ids -contains $_.pid })
[pscustomobject]@{ processes=$games; windows=$windows } | ConvertTo-Json -Depth 4 -Compress
`;

function normalizedExecutable(executable: string) { return win32.normalize(executable).toLowerCase(); }
export function parseNativeGameState(raw: unknown, engine: Engine, executable: string): NativeGameState {
  validateGameExecutable(executable, engine);
  if (!raw || typeof raw !== 'object') throw new Error('Could not read running game windows.');
  const data = raw as { processes?: unknown; windows?: unknown };
  if (!Array.isArray(data.processes) || !Array.isArray(data.windows)) throw new Error('Could not read running game windows.');
  const matchingProcesses = data.processes.filter((p): p is { pid: number; name: string; executable: string } => {
    return !!p && typeof p === 'object' && Number.isInteger(p.pid) && p.pid > 0 && p.name === expectedName(engine) && typeof p.executable === 'string';
  });
  const allowedPids = new Set(matchingProcesses.filter(p => normalizedExecutable(p.executable) === normalizedExecutable(executable)).map(p => p.pid));
  const windows = data.windows.filter((w): w is NativeGameWindow => {
    return !!w && typeof w === 'object' && Number.isInteger(w.pid) && allowedPids.has(w.pid) && typeof w.handle === 'string' && /^[1-9]\d*$/.test(w.handle) && typeof w.title === 'string';
  }).map(({ handle, pid, title }) => ({ handle, pid, title: title.slice(0, 1024) }));
  const running = matchingProcesses.length > 0;
  return {
    running, windows, executable, processIds: matchingProcesses.map(p => p.pid),
    reason: windows.length ? 'Game window found. Connect it to show the live game picture in this app.'
      : running ? 'The game is running, but no visible window matches the configured executable. Restore its window and check the game path in Settings.'
      : 'Start the game and load a demo to connect its live picture.',
  };
}

export async function readNativeGameState(engine: Engine, executable: string): Promise<NativeGameState> {
  validateGameExecutable(executable, engine);
  if (process.platform !== 'win32') return { running: false, windows: [], processIds: [], executable, reason: 'Live game-window capture is available on Windows.' };
  const powershell = join(process.env.SystemRoot || 'C:\\Windows', 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe');
  const { stdout } = await execute(powershell, ['-NoLogo', '-NoProfile', '-NonInteractive', '-Command', windowQuery], { windowsHide: true, timeout: 12_000, maxBuffer: 1024 * 1024, encoding: 'utf8' });
  return parseNativeGameState(JSON.parse(stdout.replace(/^\uFEFF/, '').trim()), engine, executable);
}

export function sourceMatchesGameWindow(sourceId: string, windows: NativeGameWindow[]) {
  const handle = /^window:([1-9]\d*):\d+$/.exec(sourceId)?.[1];
  return !!handle && windows.some(window => window.handle === handle);
}

export function gameLaunchArguments(demo: PlaybackDemo, executable: string) {
  validateGameExecutable(executable, demo.engine);
  return { executable, args: ['-steam', '-console', '+playdemo', consolePath(demo.path)], cwd: dirname(executable) };
}

export async function launchNativeDemo(demo: PlaybackDemo, executable: string): Promise<{ launched: boolean; state: NativeGameState; message: string }> {
  const state = await readNativeGameState(demo.engine, executable);
  if (state.running) return {
    launched: false, state,
    message: 'The game is already running. Paste the play command into its console, wait for the demo to load, then paste the seek command. This preserves your current game session.',
  };
  const launch = gameLaunchArguments(demo, executable);
  const child = spawn(launch.executable, launch.args, { cwd: launch.cwd, detached: true, stdio: 'ignore', windowsHide: false });
  await new Promise<void>((resolve, reject) => { child.once('spawn', resolve); child.once('error', reject); });
  child.unref();
  return { launched: true, state, message: 'The game is starting. After the demo loads, paste the seek command into the game console, then connect its live picture here.' };
}
