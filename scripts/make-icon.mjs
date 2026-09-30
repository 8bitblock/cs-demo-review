import {mkdir,writeFile} from 'node:fs/promises';
const n=256,stride=n*4,andStride=Math.ceil(n/32)*4,dib=Buffer.alloc(40+stride*n+andStride*n);
dib.writeUInt32LE(40,0);dib.writeInt32LE(n,4);dib.writeInt32LE(n*2,8);dib.writeUInt16LE(1,12);dib.writeUInt16LE(32,14);dib.writeUInt32LE(stride*n,20);
const inside=(x,y,points)=>{let yes=false;for(let i=0,j=points.length-1;i<points.length;j=i++){const a=points[i],b=points[j];if((a[1]>y)!==(b[1]>y)&&x<(b[0]-a[0])*(y-a[1])/(b[1]-a[1])+a[0])yes=!yes;}return yes;};
const outer=[[128,38],[207,80],[207,166],[128,219],[49,166],[49,80]],inner=[[128,53],[194,87],[194,159],[128,204],[62,159],[62,87]];
for(let y=0;y<n;y++)for(let x=0;x<n;x++){
  let color=[14,21,31,255];const cx=Math.max(30,Math.min(n-31,x)),cy=Math.max(30,Math.min(n-31,y));if(Math.hypot(x-cx,y-cy)>30)color=[0,0,0,0];
  if(inside(x,y,outer)&&!inside(x,y,inner))color=[235,169,64,255];
  const dx=x-128,dy=y-127,d=Math.hypot(dx,dy);
  if((d>=28&&d<=34)||((Math.abs(dx)<=3&&Math.abs(dy)>=40&&Math.abs(dy)<=57)||(Math.abs(dy)<=3&&Math.abs(dx)>=40&&Math.abs(dx)<=57))||d<5)color=[236,185,95,255];
  const offset=40+((n-1-y)*n+x)*4;dib[offset]=color[2];dib[offset+1]=color[1];dib[offset+2]=color[0];dib[offset+3]=color[3];
  if(color[3]===0)dib[40+stride*n+(n-1-y)*andStride+(x>>3)]|=1<<(7-x%8);
}
const header=Buffer.alloc(22);header.writeUInt16LE(1,2);header.writeUInt16LE(1,4);header.writeUInt16LE(1,10);header.writeUInt16LE(32,12);header.writeUInt32LE(dib.length,14);header.writeUInt32LE(22,18);
await mkdir('build',{recursive:true});await writeFile('build/icon.ico',Buffer.concat([header,dib]));
