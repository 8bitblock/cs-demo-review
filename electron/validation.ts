import { z } from 'zod';
export const identifier = z.string().min(1).max(160).regex(/^[a-zA-Z0-9_.:-]+$/);
export const tickValue = z.number().int().min(0).max(2_147_483_647);
export const replayRequest = z.object({id:identifier, fromTick:tickValue, toTick:tickValue}).refine(x=>x.toTick>=x.fromTick && x.toTick-x.fromTick<=4096, 'Replay windows must span at most 4096 ticks.');
export const noteRequest = z.object({id:identifier.optional(),demoId:identifier,tick:tickValue,playerId:z.string().max(160),text:z.string().max(10_000),kind:z.enum(['note','bookmark'])});
export const settingsUpdate = z.object({cs2Path:z.string().max(4096).optional(),csgoPath:z.string().max(4096).optional(),steamPath:z.string().max(4096).optional(),source2ViewerPath:z.string().max(4096).optional()}).strict();
export function consolePath(file: string) {
  if (/[";\r\n\0]/.test(file)) throw new Error('This filename contains characters the game console cannot safely use. Rename a copy of the demo and import that copy.');
  return file.replaceAll('\\', '/');
}
