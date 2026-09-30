import { contextBridge, ipcRenderer, webUtils } from 'electron';
import type { DesktopAPI, ImportProgress } from '../shared/types';
const invoke=(method:string,...args:unknown[])=>ipcRenderer.invoke(`demo:${method}`,...args);
const api:DesktopAPI={
  listDemos:()=>invoke('listDemos'), getMatch:id=>invoke('getMatch',id), getReplay:(id,fromTick,toTick)=>invoke('getReplay',{id,fromTick,toTick}),
  importDemos:()=>invoke('importDemos'), importDropped:files=>invoke('importPaths',Array.from(files).map(file=>webUtils.getPathForFile(file)).filter(Boolean)), cancelImport:jobId=>invoke('cancelImport',jobId), removeDemo:id=>invoke('removeDemo',id),
  saveNote:note=>invoke('saveNote',note), deleteNote:id=>invoke('deleteNote',id), exportReport:(id,format)=>invoke('exportReport',{id,format}),
  getSettings:()=>invoke('getSettings'), updateSettings:settings=>invoke('updateSettings',settings), choosePath:kind=>invoke('choosePath',kind),
  getPlaybackCommands:(id,tick)=>invoke('getPlaybackCommands',{id,tick}),openInGame:(id,tick)=>invoke('openInGame',{id,tick}),copyText:text=>invoke('copyText',text),
  getGameWindows:id=>invoke('getGameWindows',id),prepareGameCapture:(id,sourceId)=>invoke('prepareGameCapture',{id,sourceId}),cancelGameCapture:()=>invoke('cancelGameCapture'),
  listClips:id=>invoke('listClips',id),importClip:reference=>invoke('importClip',reference),beginClipRecording:reference=>invoke('beginClipRecording',reference),appendClipRecording:(id,data)=>invoke('appendClipRecording',{id,data}),finishClipRecording:(id,duration)=>invoke('finishClipRecording',{id,duration}),discardClipRecording:id=>invoke('discardClipRecording',id),removeClip:id=>invoke('removeClip',id),
  getMap:id=>invoke('getMap',id), importMapPack:()=>invoke('importMapPack'), extractMap:id=>invoke('extractMap',id), reanalyse:id=>invoke('reanalyse',id),
  getDiagnostics:()=>invoke('getDiagnostics'), exportDiagnostics:()=>invoke('exportDiagnostics'),
  onImportProgress:callback=>{const handler=(_:unknown,data:ImportProgress)=>callback(data);ipcRenderer.on('demo:import-progress',handler);return()=>ipcRenderer.removeListener('demo:import-progress',handler);},
  onWorkerError:callback=>{const handler=(_:unknown,message:string)=>callback(message);ipcRenderer.on('demo:worker-error',handler);return()=>ipcRenderer.removeListener('demo:worker-error',handler);}
};
contextBridge.exposeInMainWorld('csDemo',api);
