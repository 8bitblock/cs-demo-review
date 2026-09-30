import { readFile, writeFile, mkdir, access } from 'node:fs/promises';
import { join, dirname, resolve, basename } from 'node:path';
import { promisify } from 'node:util';
import { execFile } from 'node:child_process';
import type { AppSettings } from '../shared/types';
const execute = promisify(execFile);
export const exists = async (path: string) => { if (!path) return false; try { await access(path); return true; } catch { return false; } };
export async function discoverInstallations(): Promise<Pick<AppSettings,'cs2Path'|'csgoPath'|'steamPath'>> {
  let steamPath=join(process.env['ProgramFiles(x86)'] || 'C:\\Program Files (x86)', 'Steam');
  try { const {stdout}=await execute('reg.exe',['query','HKCU\\Software\\Valve\\Steam','/v','SteamPath'],{windowsHide:true}); const value=stdout.match(/SteamPath\s+REG_SZ\s+(.+)/i)?.[1].trim(); if(value) steamPath=value; } catch { /* Steam may not be installed. */ }
  const libraries=new Set([steamPath]);
  try { const vdf=await readFile(join(steamPath,'steamapps','libraryfolders.vdf'),'utf8'); for(const match of vdf.matchAll(/"path"\s*"([^"]+)"/g)) libraries.add(match[1].replaceAll('\\\\','\\')); } catch { /* Manual paths remain available. */ }
  let cs2Path='',csgoPath='';
  for(const library of libraries){
    let folder='Counter-Strike Global Offensive';
    try { const manifest=await readFile(join(library,'steamapps','appmanifest_730.acf'),'utf8'); folder=manifest.match(/"installdir"\s*"([^"]+)"/)?.[1] || folder; } catch { /* Try standard directory. */ }
    const base=join(library,'steamapps','common',folder);
    const cs2=join(base,'game','bin','win64','cs2.exe'),csgo=join(base,'csgo.exe');
    if(!cs2Path && await exists(cs2)) cs2Path=cs2;
    if(!csgoPath && await exists(csgo)) csgoPath=csgo;
  }
  return {steamPath,cs2Path,csgoPath};
}
export async function loadSettings(dataDir:string, bundledExtractor:string):Promise<AppSettings>{
  const detected=await discoverInstallations();
  let saved:Partial<AppSettings>={};
  try { saved=JSON.parse(await readFile(join(dataDir,'settings.json'),'utf8')); } catch { /* First run. */ }
  const extractor=typeof saved.source2ViewerPath==='string' && await exists(saved.source2ViewerPath)?saved.source2ViewerPath:await exists(bundledExtractor)?bundledExtractor:'';
  return {...detected,...saved,source2ViewerPath:extractor,libraryPath:dataDir};
}
export async function writeSettings(dataDir:string,settings:AppSettings){
  await mkdir(dataDir,{recursive:true});await writeFile(join(dataDir,'settings.json'),JSON.stringify(settings,null,2));
}
export function gameRoot(executable:string,engine:'cs2'|'csgo'){
  if(engine==='cs2' && basename(executable).toLowerCase()==='cs2.exe') return resolve(dirname(executable),'../../..');
  if(engine==='csgo' && basename(executable).toLowerCase()==='csgo.exe') return dirname(executable);
  return executable;
}
