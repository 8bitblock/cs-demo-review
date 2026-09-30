import {describe,it,expect} from 'vitest';
import {consolePath,replayRequest,noteRequest,settingsUpdate} from '../electron/validation';
describe('privileged request validation',()=>{
  it('bounds queries and rejects invalid ticks',()=>{expect(()=>replayRequest.parse({id:'abc',fromTick:0,toTick:4097})).toThrow();expect(()=>replayRequest.parse({id:'abc',fromTick:100,toTick:1})).toThrow();expect(()=>replayRequest.parse({id:'abc',fromTick:0,toTick:4096})).not.toThrow();});
  it('prevents arbitrary library-path changes',()=>expect(()=>settingsUpdate.parse({libraryPath:'C:\\Windows'})).toThrow());
  it('rejects oversized notes',()=>expect(()=>noteRequest.parse({demoId:'abc',tick:1,playerId:'',kind:'note',text:'x'.repeat(10001)})).toThrow());
  it('quotes demo paths without allowing console command injection',()=>{expect(consolePath('B:\\Matches\\my demo.dem')).toBe('B:/Matches/my demo.dem');expect(()=>consolePath('B:\\x";quit.dem')).toThrow();});
});
