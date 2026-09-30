import { app, BrowserWindow, ipcMain, dialog, clipboard, protocol, net, shell, session, desktopCapturer } from 'electron';
import { join, resolve, basename, extname } from 'node:path';
import { mkdir, readFile, stat, writeFile, realpath } from 'node:fs/promises';
import { mkdirSync } from 'node:fs';
import { createHash, randomUUID } from 'node:crypto';
import { pathToFileURL } from 'node:url';
import { z } from 'zod';
import { WorkerClient } from './worker-client';
import { identifier, tickValue, replayRequest, noteRequest, settingsUpdate } from './validation';
import { preparePlaybackCommands, readNativeGameState, sourceMatchesGameWindow, launchNativeDemo } from './playback';
import { CaptureGrant, permitsGameCapture } from './capture-policy';
import { ClipStore } from './clips';
import { parseVideoRange } from './video-range';
import { loadSettings, writeSettings, exists, gameRoot } from './settings';
import { reportHTML, reportJSON } from './report';
import { developmentOrigin } from './development';
import type { AppSettings, Demo, MapAsset, MatchDetail, PlaybackCommands, ImportProgress } from '../shared/types';

protocol.registerSchemesAsPrivileged([{scheme:'demomap',privileges:{standard:true,secure:true,supportFetchAPI:true,corsEnabled:true}},{scheme:'democlip',privileges:{standard:true,secure:true,supportFetchAPI:true,stream:true}}]);
// Each source checkout owns its library, Chromium profile and instance lock.
// Set both paths before requesting the lock or creating any browser sessions.
if(!app.isPackaged){
  const sourceUserData=resolve(__dirname,'../.tmp/electron-user-data');
  mkdirSync(sourceUserData,{recursive:true});
  app.setPath('userData',sourceUserData);
  app.setPath('sessionData',sourceUserData);
}
const devOrigin=developmentOrigin(process.env.DEMO_REVIEW_DEV_URL,app.isPackaged);
let window:BrowserWindow|null=null;
let worker:WorkerClient;
let settings:AppSettings;
let dataDir:string;
let clips:ClipStore;
let capturedDemoId:string|null=null;
const assetPaths=new Map<string,string>();
const activeJobs=new Map<string,ImportProgress>();
const captureGrant=new CaptureGrant();
const pendingEvents:Array<{event:string;data:unknown}>=[];
let pageLoaded=false;
const send=(event:string,data:unknown)=>{if(window&&!window.isDestroyed()&&pageLoaded)window.webContents.send(`demo:${event}`,data);else pendingEvents.push({event,data});};
const request=(method:string,params:unknown={},timeout?:number)=>worker.request(method,params,timeout);
function handle(name:string,handler:(...args:any[])=>unknown){
  ipcMain.handle(`demo:${name}`,async(event,...args)=>{
    if(!window || event.sender!==window.webContents || event.senderFrame!==window.webContents.mainFrame)throw new Error('Invalid request origin.');
    return handler(...args);
  });
}
async function assetURL(file:string|undefined):Promise<string|undefined>{
  if(!file)return undefined;
  const absolute=await realpath(file);
  const cache=await realpath(join(dataDir,'maps'));
  if(!absolute.toLowerCase().startsWith(cache.toLowerCase()+'\\') && !absolute.toLowerCase().startsWith(cache.toLowerCase()+'/'))throw new Error('Map image is outside the managed asset directory.');
  if(!['.png','.jpg','.jpeg','.webp'].includes(extname(absolute).toLowerCase()))return undefined;
  const token=createHash('sha256').update(absolute).digest('hex');assetPaths.set(token,absolute);return `demomap://asset/${token}`;
}
async function publicMap(asset:MapAsset|null):Promise<MapAsset|null>{
  if(!asset)return null;
  return {...asset,warnings:asset.warnings||[],image:await assetURL(asset.image),floors:await Promise.all((asset.floors||[]).map(async floor=>({...floor,image:await assetURL(floor.image)})))};
}
async function queueImports(paths:string[]){
  const jobs:string[]=[];
  for(const file of paths){
    const source=resolve(file);
    const jobId=randomUUID();
    const progress:ImportProgress={jobId,path:source,name:basename(source),stage:'queued',progress:0,message:'Waiting to analyse'};
    activeJobs.set(jobId,progress);send('import-progress',progress);
    try{const info=await stat(source);if(!info.isFile()||extname(source).toLowerCase()!=='.dem')throw new Error('Choose an uncompressed .dem file.');await request('importDemo',{path:source,jobId});jobs.push(jobId);}catch(error){const failed={...progress,stage:'error' as const,message:error instanceof Error?error.message:String(error)};activeJobs.set(jobId,failed);send('import-progress',failed);}
  }
  return jobs;
}
async function playback(id:string,tick:number):Promise<PlaybackCommands>{
  const {demo}=await request('getMatch',{id}) as MatchDetail;
  const executable=demo.engine==='cs2'?settings.cs2Path:settings.csgoPath;
  const gameAvailable=await exists(executable),demoAvailable=await exists(demo.path);
  return preparePlaybackCommands(demo,tick,executable,gameAvailable,demoAvailable);
}
async function gameSources(id:string){
  const {demo}=await request('getMatch',{id}) as MatchDetail;
  const executable=demo.engine==='cs2'?settings.cs2Path:settings.csgoPath;
  if(!executable)return [];
  const state=await readNativeGameState(demo.engine,executable);
  if(!state.windows.length)return [];
  const sources=await desktopCapturer.getSources({types:['window'],thumbnailSize:{width:0,height:0},fetchWindowIcons:false});
  return sources.filter(source=>sourceMatchesGameWindow(source.id,state.windows));
}
function registerHandlers(){
  handle('listDemos',async()=>{const demos=await request('listDemos');setTimeout(()=>{for(const p of activeJobs.values())if(!['complete','error','cancelled'].includes(p.stage))send('import-progress',p);},30);return demos;});
  handle('getMatch',(id:unknown)=>request('getMatch',{id:identifier.parse(id)}));
  handle('getReplay',(args:unknown)=>request('getReplay',replayRequest.parse(args)));
  handle('importDemos',async()=>{const result=await dialog.showOpenDialog(window!,{title:'Open Counter-Strike demos',properties:['openFile','multiSelections'],filters:[{name:'Counter-Strike demo',extensions:['dem']}]});return result.canceled?[]:queueImports(result.filePaths);});
  handle('importPaths',(paths:unknown)=>queueImports(z.array(z.string().min(1).max(4096)).max(100).parse(paths)));
  handle('cancelImport',(jobId:unknown)=>request('cancelImport',{jobId:identifier.parse(jobId)}));
  handle('removeDemo',async(id:unknown)=>{const demoId=identifier.parse(id);const result=await dialog.showMessageBox(window!,{type:'question',title:'Remove from library',message:'Remove this demo’s cached analysis and review notes?',detail:'The original .dem file will be kept.',buttons:['Keep demo','Remove cache'],defaultId:0,cancelId:0});if(result.response!==1)return false;await request('removeDemo',{id:demoId});return true;});
  handle('saveNote',(note:unknown)=>request('saveNote',noteRequest.parse(note)));
  handle('deleteNote',(id:unknown)=>request('deleteNote',{id:identifier.parse(id)}));
  handle('exportReport',async(args:unknown)=>{
    const {id,format}=z.object({id:identifier,format:z.enum(['html','json'])}).parse(args);const match=await request('getMatch',{id}) as MatchDetail;
    const name=match.demo.name.replace(/[^\p{L}\p{N}_.-]/gu,'_').slice(0,100);
    const file=await dialog.showSaveDialog(window!,{title:'Export evidence report',defaultPath:`${name}-review.${format}`,filters:[{name:format==='html'?'HTML report':'JSON report',extensions:[format]}]});
    if(file.canceled||!file.filePath)return null;await writeFile(file.filePath,format==='html'?reportHTML(match):reportJSON(match),'utf8');return file.filePath;
  });
  handle('getSettings',()=>settings);
  handle('updateSettings',async(value:unknown)=>{settings={...settings,...settingsUpdate.parse(value)};await writeSettings(dataDir,settings);return settings;});
  handle('choosePath',async(value:unknown)=>{const kind=z.enum(['cs2','csgo','source2viewer']).parse(value);const result=await dialog.showOpenDialog(window!,{title:`Select ${kind==='source2viewer'?'Source2Viewer CLI':kind} executable`,properties:['openFile'],filters:[{name:'Windows executable',extensions:['exe']}]});return result.canceled?null:result.filePaths[0];});
  const playbackArgs=z.object({id:identifier,tick:tickValue});
  handle('getPlaybackCommands',(args:unknown)=>{const {id,tick}=playbackArgs.parse(args);return playback(id,tick);});
  handle('openInGame',async(args:unknown)=>{
    const {id,tick}=playbackArgs.parse(args),commands=await playback(id,tick);if(!commands.available)throw new Error(commands.reason);
    const {demo}=await request('getMatch',{id}) as MatchDetail;
    const launch=await launchNativeDemo(demo,commands.executable);
    clipboard.writeText(launch.launched?commands.seek:commands.play);return {...commands,reason:launch.message};
  });
  handle('getGameWindows',async(id:unknown)=>(await gameSources(identifier.parse(id))).map(source=>({id:source.id,name:source.name})));
  handle('prepareGameCapture',async(args:unknown)=>{
    captureGrant.clear();
    const {id,sourceId}=z.object({id:identifier,sourceId:z.string().regex(/^window:\d+:\d+$/).max(100)}).parse(args);
    const sources=await gameSources(id);
    if(!sources.some(source=>source.id===sourceId))throw new Error('The selected game window is no longer available. Refresh the window list.');
    captureGrant.prepare(id,sourceId);
  });
  handle('cancelGameCapture',()=>{captureGrant.clear();capturedDemoId=null;});
  const clipReference=z.object({demoId:identifier,playerId:z.string().max(160),reviewTick:tickValue,title:z.string().min(1).max(200)}).strict();
  handle('listClips',(id:unknown)=>clips.list(identifier.parse(id)));
  handle('importClip',async(value:unknown)=>{
    const reference=clipReference.parse(value);
    await request('getMatch',{id:reference.demoId});
    const result=await dialog.showOpenDialog(window!,{title:'Import a recorded game clip',properties:['openFile'],filters:[{name:'Video clips',extensions:['mp4','webm']}]});
    return result.canceled?null:clips.import(result.filePaths[0],reference);
  });
  handle('beginClipRecording',async(value:unknown)=>{
    const reference=clipReference.parse(value);
    if(capturedDemoId!==reference.demoId)throw new Error('Connect the game video before recording a clip.');
    return clips.begin(reference);
  });
  handle('appendClipRecording',(value:unknown)=>{
    const {id,data}=z.object({id:identifier,data:z.instanceof(Uint8Array)}).parse(value);return clips.append(id,data);
  });
  handle('finishClipRecording',(value:unknown)=>{
    const {id,duration}=z.object({id:identifier,duration:z.number().positive().max(130)}).parse(value);return clips.finish(id,duration);
  });
  handle('discardClipRecording',(id:unknown)=>clips.discard(identifier.parse(id)));
  handle('removeClip',async(id:unknown)=>{
    const clipId=identifier.parse(id);
    const result=await dialog.showMessageBox(window!,{type:'question',title:'Remove recorded clip',message:'Delete this clip from the local library?',detail:'This removes the saved video and its review link. Imported source videos are kept.',buttons:['Keep clip','Delete clip'],defaultId:0,cancelId:0});
    if(result.response!==1)return false;await clips.remove(clipId);return true;
  });
  handle('copyText',(text:unknown)=>clipboard.writeText(z.string().max(100_000).parse(text)));
  handle('getMap',async(id:unknown)=>publicMap(await request('getMap',{id:identifier.parse(id)})));
  handle('importMapPack',async()=>{const result=await dialog.showOpenDialog(window!,{title:'Import map asset manifest',properties:['openFile'],filters:[{name:'Map manifest',extensions:['json']}]});if(result.canceled)return null;return publicMap(await request('importMapPack',{path:result.filePaths[0]},600_000));});
  handle('extractMap',async(id:unknown)=>{const demoId=identifier.parse(id);const {demo}=await request('getMatch',{id:demoId}) as MatchDetail;const executable=demo.engine==='cs2'?settings.cs2Path:settings.csgoPath;if(!executable)throw new Error('Set the game installation in Settings first.');return publicMap(await request('extractMap',{id:demoId,gamePath:gameRoot(executable,demo.engine),source2ViewerPath:settings.source2ViewerPath},1_800_000));});
  handle('reanalyse',(id:unknown)=>request('reanalyse',{id:identifier.parse(id)},1_800_000));
  handle('getDiagnostics',async()=>({...await request('diagnostics'),appVersion:app.getVersion(),libraryPath:dataDir,platform:`${process.platform}/${process.arch}`}));
  handle('exportDiagnostics',async()=>{const file=await dialog.showSaveDialog(window!,{title:'Save local diagnostics',defaultPath:'cs-demo-review-diagnostics.json',filters:[{name:'JSON',extensions:['json']}]});if(file.canceled||!file.filePath)return null;const diagnostics={appVersion:app.getVersion(),platform:`${process.platform}/${process.arch}`,electron:process.versions.electron,worker:await request('diagnostics').catch(()=>({status:'unavailable'})),errors:worker.errors().map(line=>line.replaceAll(dataDir,'[library]').replaceAll(app.getPath('home'),'[user]'))};await writeFile(file.filePath,JSON.stringify(diagnostics,null,2));return file.filePath;});
}
async function createWindow(){
  pageLoaded=false;
  window=new BrowserWindow({width:1560,height:980,minWidth:1000,minHeight:700,title:'CS Demo Review',icon:join(__dirname,'../build/icon.ico'),backgroundColor:'#0b1018',autoHideMenuBar:true,webPreferences:{preload:join(__dirname,'preload.cjs'),contextIsolation:true,nodeIntegration:false,sandbox:true,webSecurity:true}});
  window.webContents.setWindowOpenHandler(({url})=>{if(['https://s2v.app/','https://github.com/markus-wa/demoinfocs-golang'].includes(url))void shell.openExternal(url);return {action:'deny'};});
  window.webContents.on('will-navigate',event=>event.preventDefault());
  window.webContents.on('did-finish-load',()=>{pageLoaded=true;for(const item of pendingEvents.splice(0))send(item.event,item.data);});
  window.on('closed',()=>{window=null;pageLoaded=false;captureGrant.clear();capturedDemoId=null;});
  if(devOrigin)await window.loadURL(devOrigin);
  else await window.loadFile(join(__dirname,'../dist/index.html'));
}
const gotLock=app.requestSingleInstanceLock();
if(!gotLock)app.quit();
else{
  app.on('second-instance',()=>{if(window?.isMinimized())window.restore();window?.focus();});
  app.whenReady().then(async()=>{
    app.setAppUserModelId('com.csdemoreview.app');
    dataDir=resolve(process.env.CS_DEMO_REVIEW_DATA_DIR||join(app.getPath('userData'),'library'));
    await mkdir(join(dataDir,'maps'),{recursive:true});
    clips=new ClipStore(join(dataDir,'clips'));await clips.initialize();
    const extractor=app.isPackaged?join(process.resourcesPath,'tools','Source2Viewer-CLI.exe'):resolve(__dirname,'../.tools/source2viewer/Source2Viewer-CLI.exe');
    settings=await loadSettings(dataDir,extractor);
    worker=new WorkerClient(app.isPackaged?join(process.resourcesPath,'worker','demo-worker.exe'):resolve(__dirname,'../worker/bin/demo-worker.exe'),dataDir);
    worker.on('import-progress',(progress:ImportProgress)=>{activeJobs.set(progress.jobId,progress);send('import-progress',progress);});
    worker.on('worker-error',(message:string)=>{for(const [id,p]of activeJobs){if(!['complete','cancelled','error'].includes(p.stage)){const failed={...p,stage:'error' as const,message};activeJobs.set(id,failed);send('import-progress',failed);}}send('worker-error',message);});
    worker.start();registerHandlers();
    protocol.handle('demomap',async req=>{
      const url=new URL(req.url);const file=url.hostname==='asset'?assetPaths.get(url.pathname.slice(1)):undefined;
      if(!file)return new Response('Not found',{status:404});
      const response=await net.fetch(pathToFileURL(file).toString());
      const headers=new Headers(response.headers);headers.set('Access-Control-Allow-Origin','*');
      return new Response(response.body,{status:response.status,headers});
    });
    protocol.handle('democlip',async req=>{
      try{
        const url=new URL(req.url);if(url.hostname!=='video')return new Response('Not found',{status:404});
        if(req.method!=='GET'&&req.method!=='HEAD')return new Response('Method not allowed',{status:405});
        const file=await clips.mediaPath(url.pathname.slice(1)),info=await stat(file);
        let range;try{range=parseVideoRange(req.headers.get('range'),info.size);}catch{return new Response(null,{status:416,headers:{'Content-Range':`bytes */${info.size}`,'Accept-Ranges':'bytes'}});}
        const headers=new Headers({'Content-Type':extname(file)==='.webm'?'video/webm':'video/mp4','Accept-Ranges':'bytes','Content-Length':String(range?range.end-range.start+1:info.size)});
        if(range)headers.set('Content-Range',`bytes ${range.start}-${range.end}/${info.size}`);
        if(req.method==='HEAD')return new Response(null,{status:range?206:200,headers});
        // Electron's file:// loader returns the requested bytes with status 200
        // and no range headers. Our standard scheme must expose HTTP semantics
        // or Chromium treats seek responses as a new file starting at byte zero.
        const response=await net.fetch(pathToFileURL(file).toString(),range?{headers:{Range:`bytes=${range.start}-${range.end}`}}:undefined);
        return new Response(response.body,{status:range?206:200,headers});
      }
      catch{return new Response('Clip unavailable',{status:404});}
    });
    session.defaultSession.setPermissionCheckHandler((wc,permission)=>permission==='display-capture'&&wc===window?.webContents&&captureGrant.available());
    session.defaultSession.setPermissionRequestHandler((wc,permission,callback,details)=>callback(permitsGameCapture(permission,wc===window?.webContents,captureGrant.available(),details)));
    session.defaultSession.setDisplayMediaRequestHandler((capture,callback)=>{
      if(!window||capture.frame!==window.webContents.mainFrame||!capture.videoRequested||capture.audioRequested){callback({});return;}
      const grant=captureGrant.consume();if(!grant){callback({});return;}
      void gameSources(grant.demoId).then(sources=>{
        const source=sources.find(item=>item.id===grant.sourceId);
        const allowed=source&&window&&!window.isDestroyed()&&capture.frame===window.webContents.mainFrame;
        if(allowed)capturedDemoId=grant.demoId;
        callback(allowed?{video:source}:{});
      }).catch(()=>{try{callback({});}catch{/* Requesting window closed during source lookup. */}});
    });
    const developmentSocket=devOrigin?` ${devOrigin.replace('http:','ws:')}`:'';
    session.defaultSession.webRequest.onHeadersReceived((details,callback)=>callback({responseHeaders:{...details.responseHeaders,'Content-Security-Policy':[`default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: demomap:; media-src 'self' blob: democlip:; connect-src 'self'${developmentSocket}; object-src 'none'; base-uri 'none'; frame-src 'none'`]}}));
    await createWindow();
  }).catch(error=>{dialog.showErrorBox('CS Demo Review could not start',String(error));app.quit();});
  app.on('activate',()=>{if(BrowserWindow.getAllWindows().length===0)void createWindow();});
  let quitting=false;
  app.on('before-quit',event=>{if(quitting||!worker)return;event.preventDefault();quitting=true;void Promise.allSettled([worker.stop(),clips?.close()]).finally(()=>app.exit(0));});
  app.on('window-all-closed',()=>app.quit());
}
