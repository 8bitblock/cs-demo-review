# Worker protocol

One Go process, launched as `demo-worker --data-dir <absolute directory>`. UTF-8 JSON lines over stdin/stdout. Logs only on stderr. Requests `{id:string, method:string, params:object}`; replies `{id, result}` or `{id,error:{code,message}}`. Notifications `{event:"import-progress",data:ImportProgress}`. All request/response shapes follow shared/types.ts (camelCase, empty arrays instead of null). Worker must allow cancel during imports and read requests while background jobs run. Process one import job at a time; jobs persist only in-process. All generated data resides in data-dir, never overwrite source demos.

Methods:
- `listDemos {}` -> Demo[]
- `getMatch {id}` -> MatchDetail
- `getReplay {id,fromTick,toTick}` -> ReplayWindow (bounded at 4096 ticks per call, include a nearest prior sample per player to seed window if useful)
- `importDemo {path,jobId}` -> `{jobId}` immediately; notifications track queue/progress; duplicated SHA256 returns complete with existing demoId
- `cancelImport {jobId}` -> `{ok:true}`
- `removeDemo {id}` -> `{ok:true}` (delete derived cache only)
- `saveNote {id?,demoId,tick,playerId,text,kind}` -> ReviewNote
- `deleteNote {id}` -> `{ok:true}`
- `getMap {id}` -> MapAsset|null (id = demo id)
- `importMapPack {path}` -> MapAsset (manifest JSON path, store validated copy under data-dir/maps)
- `extractMap {id,gamePath,source2ViewerPath}` -> MapAsset
- `reanalyse {id}` -> `{ok:true}`
- `diagnostics {}` -> `{workerVersion,libraryPath}`

Map manifest format is specified in `docs/MAP-PACKS.md` and `maps.Manifest`: `schemaVersion:1`, map/engine/version/source, radar transform, relative image/floor paths, optional `geometry` (not `geometryPath`), `coordinates`, `completeGeometry`, and `dynamicGeometry`. Geometry accepts `.mesh` AWMH v1 (4 byte magic, uint32 version, uint32 vertex count, uint32 triangle count, xyz float32 vertices, uint32 index triples, little endian), TRI, GLB, or limited BSP. Manifest version must match demo mapVersion; verified is computed by app, never trusted from input. Paths must resolve inside the pack. Exact matching and complete static geometry are required for visibility findings; unknown versions are not matches. Local extraction may support visual replay while remaining ineligible for reaction assessments.

Shared Go packages: `worker/internal/model` owns structs equivalent to TS; `worker/internal/analysis` owns pure rules engine; `worker/internal/maps` owns asset loading/extraction and geometry; parser/store/main owned by backend implementer. Coordinate vectors serialized x,y,z. Times seconds; tick original demo tick; angles degrees; timingPrecision seconds.

## Telemetry and coverage additions

- `Sample.spottedKnown` distinguishes an absent spotting mask from a recorded mask with no observers; `spottedBy` contains recorded observer player IDs. This is a network-state proxy, not exact line of sight.
- `Shot.aimPunchScale` records how punch values convert to angular offsets: CS2 native packet offsets use 1, while legacy sampled entity punch uses the nominal assumption 2. Unknown scales are not inferred.
- `Capability.samples` counts detector-eligible observations; `measuredSamples` and `basis` describe useful measured telemetry separately. Statuses include `limited` for small eligible samples and `measured` for observations without a supported detector.
- `ReviewMetric` and `ReviewClip` carry optional `signal` and `provenance`; neutral observations never increase evidence counts. `Reviewed with limits` is distinct from `Low concern` and from missing data.
- Parser adapter upgrades automatically reparse available original recordings through atomic cache replacement, preserving library IDs, notes and map selections. Missing originals leave cached review usable with a recovery warning.
