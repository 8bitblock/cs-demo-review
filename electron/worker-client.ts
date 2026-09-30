import { spawn, execFile, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { EventEmitter } from 'node:events';
import { createInterface } from 'node:readline';
import { randomUUID } from 'node:crypto';

export class WorkerClient extends EventEmitter {
  private child?: ChildProcessWithoutNullStreams;
  private pending = new Map<string, {resolve: (value: any) => void; reject: (error: Error) => void; timer: NodeJS.Timeout}>();
  private stopping = false;
  private recentErrors: string[] = [];
  constructor(private executable: string, private dataDir: string) { super(); }
  start() {
    if (this.child) return;
    this.stopping = false;
    const child = spawn(this.executable, ['--data-dir', this.dataDir], { stdio: 'pipe', windowsHide: true });
    this.child = child;
    const lines = createInterface({ input: child.stdout });
    lines.on('line', line => {
      try {
        const message = JSON.parse(line);
        if (message.event) { this.emit(message.event, message.data); return; }
        const request = this.pending.get(message.id);
        if (!request) return;
        clearTimeout(request.timer);
        this.pending.delete(message.id);
        if (message.error) request.reject(new Error(message.error.message || 'The worker could not complete this request.'));
        else request.resolve(message.result);
      } catch { this.recentErrors.push('Worker returned an invalid response.'); }
    });
    child.stderr.on('data', data => { this.recentErrors.push(String(data).slice(0, 2000)); this.recentErrors = this.recentErrors.slice(-20); });
    const stop = (message: string) => {
      if (this.child !== child) return;
      this.child = undefined;
      for (const request of this.pending.values()) { clearTimeout(request.timer); request.reject(new Error(message)); }
      this.pending.clear();
      if (!this.stopping) this.emit('worker-error', message);
    };
    child.on('error', error => stop(`Analysis service unavailable: ${error.message}. Restart the app; if this persists, reinstall the application.`));
    child.stdin.on('error', error => stop(`Analysis worker connection closed: ${error.message}`));
    child.stdout.on('error', error => stop(`Analysis worker output closed: ${error.message}`));
    child.stderr.on('error', error => stop(`Analysis worker diagnostics closed: ${error.message}`));
    lines.on('error', error => stop(`Analysis worker response stream failed: ${error.message}`));
    child.on('exit', code => stop(`Analysis worker stopped (${code ?? 'terminated'}). Reopen the library to restart it. Your original demos are unchanged.`));
  }
  request<T = any>(method: string, params: unknown = {}, timeout = 120_000): Promise<T> {
    if (!this.child) this.start();
    return new Promise((resolve, reject) => {
      const id = randomUUID();
      const timer = setTimeout(() => { this.pending.delete(id); reject(new Error(`The ${method} request timed out.`)); }, timeout);
      this.pending.set(id, { resolve, reject, timer });
      this.child!.stdin.write(JSON.stringify({ id, method, params }) + '\n', error => {
        if (error) { clearTimeout(timer); this.pending.delete(id); reject(new Error(error.message)); }
      });
    });
  }
  errors() { return [...this.recentErrors]; }
  stop():Promise<void> {
    this.stopping=true;
    const child=this.child;
    if(!child || child.exitCode!==null)return Promise.resolve();
    return new Promise(resolve=>{
      const timer=setTimeout(()=>{
        if(child.exitCode!==null){resolve();return;}
        if(process.platform==='win32'&&child.pid)execFile('taskkill.exe',['/PID',String(child.pid),'/T','/F'],{windowsHide:true},()=>resolve());
        else {child.kill();resolve();}
      },4000);
      child.once('exit',()=>{clearTimeout(timer);resolve();});
      child.stdin.end();
    });
  }
}
