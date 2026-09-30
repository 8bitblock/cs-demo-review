import {describe,it,expect,vi} from 'vitest';
import {EventEmitter} from 'node:events';
import {PassThrough} from 'node:stream';
const spawnMock=vi.hoisted(()=>vi.fn());
vi.mock('node:child_process',()=>({spawn:spawnMock,execFile:vi.fn()}));
import {WorkerClient} from '../electron/worker-client';
function child(){const value=Object.assign(new EventEmitter(),{stdin:new PassThrough(),stdout:new PassThrough(),stderr:new PassThrough(),exitCode:null as number|null,pid:123456,kill:vi.fn()});spawnMock.mockReturnValue(value);return value;}
describe('worker connection failure containment',()=>{
 it('handles a broken stdin stream and rejects requests without an uncaught error',async()=>{const process=child(),worker=new WorkerClient('worker.exe','data');const events:string[]=[];worker.on('worker-error',e=>events.push(e));const pending=worker.request('listDemos');const rejected=expect(pending).rejects.toThrow('connection closed');expect(()=>process.stdin.emit('error',new Error('EPIPE'))).not.toThrow();await rejected;expect(events).toHaveLength(1);process.emit('exit',1);expect(events).toHaveLength(1);});
 it('closes stdin and waits for graceful worker exit before force termination',async()=>{const process=child(),worker=new WorkerClient('worker.exe','data');worker.start();const stopped=worker.stop();expect(process.stdin.writableEnded).toBe(true);process.exitCode=0;process.emit('exit',0);await stopped;expect(process.kill).not.toHaveBeenCalled();});
});
