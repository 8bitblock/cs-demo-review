# Local maps and historical asset packs

Map names alone do not establish geometry compatibility. `maps.Match` recomputes the `verified` field by comparing engine, map name and a **nonempty exact version identifier** with the recording. A pack's incoming `verified` value is ignored. Pack authors supply version/completeness metadata; this is not third-party certification. Use provenance and independently checked assets before relying on visibility estimates.

## Pack format

Choose `map-pack.json` in the app's import dialog. Images and geometry paths are relative to that file, use forward slashes, and must remain within its folder after resolving symbolic links. Imports copy assets into the local cache and do not modify originals. Geometry is validated and normalized to a triangle file. Network URLs, absolute paths, traversal, unknown engines, malformed geometry, invalid transforms and overlapping floors are rejected.

```json
{
  "schemaVersion": 1,
  "map": "de_nuke",
  "engine": "cs2",
  "version": "build:EXACT_RECORDED_BUILD",
  "source": "Description of the original matching map and extraction",
  "posX": -3453,
  "posY": 2887,
  "scale": 7,
  "rotate": 0,
  "image": "radar.png",
  "floors": [
    {"name": "upper", "minZ": -495, "maxZ": 10000, "image": "radar.png"},
    {"name": "lower", "minZ": -10000, "maxZ": -495, "image": "radar_lower.png"}
  ],
  "geometry": "map.mesh",
  "coordinates": "source",
  "completeGeometry": false,
  "dynamicGeometry": true
}
```

The example intentionally disables scoring: fill in the exact version and only declare complete geometry after validating collision coverage. CS:GO recordings with a nonzero ServerInfo map CRC use `crc32:1234abcd` (eight lowercase hexadecimal digits), which is the engine's map CRC rather than a checksum of the entire BSP file. CS2 header versions use `build:<recorded BuildNum>`. Do not substitute a Steam client version unless its correspondence to that header field is established. `dynamicGeometry: true` means unresolved moving geometry remains and disables reaction assessment. Map version correspondence alone is insufficient: scoring requires matching version, complete geometry and no unresolved dynamic geometry.

Supported geometry:

- `.mesh`: Awpy AWMH v1, little-endian `AWMH`, version `uint32`, vertex count `uint32`, triangle count `uint32`, float32 XYZ vertices, then uint32 triangle indices. [Awpy format source](https://github.com/pnxenopoulos/awpy/blob/v3/crates/awpy/src/geometry.rs).
- `.tri`: a sequence of little-endian float32 triangles, nine floats / 36 bytes per triangle.
- `.json`: an array of triangles, each containing three `{"x": ..., "y": ..., "z": ...}` vertices.
- `.glb` / `.gltf`: triangle primitives with float32 positions and optional unsigned indices, hierarchy matrices or translation/rotation/scale. Sparse accessors and unsupported primitive modes are excluded/rejected. External buffers must remain in the pack folder. The default `source` coordinates are native world units; `source2viewer-gltf` explicitly reverses Source2Viewer's documented axis/unit conversion.
- `.bsp`: Source versions 19–21, uncompressed face/edge/vertex lumps. This fallback omits prop models, displacements and moving objects and **always remains incomplete**, regardless of the manifest flag. For complete legacy collision geometry, import a separately validated triangulated pack.

Geometry is limited to 256 MiB and five million triangles; images to 32 MiB and the manifest to 1 MiB. Degenerate-only geometry and nonfinite coordinates are rejected. BVH raycasts use line segments with a small endpoint tolerance. Floors use `[minZ, maxZ)` intervals. Radar coordinates are `(x - posX) / scale`, `(posY - y) / scale`, followed by an explicitly requested rotation in degrees. Valve's overview `rotate` HUD flag is not interpreted as a static image rotation.

## Installed game extraction

CS2 extraction uses the configured local [Source 2 Viewer](https://s2v.app/) CLI. It exports the exact named overview/radar resources from `pak01_dir.vpk` and the map's glTF export from `maps/<map>.vpk`. The smaller companion `_physics.glb` is preferred when produced. The importer reverses the glTF metre/axis conversion, caches the collision triangles, and retains radar PNGs and vertical-section metadata. [Official CLI documentation](https://s2v.app/ValveResourceFormat/guides/command-line.html).

Extraction runs without a shell, in a hidden subprocess on Windows, with cancellation and bounded run times. Its output stays in a temporary cache folder. It never overwrites installed game assets. The original VPK content hash is recorded as the asset version, which generally cannot match a recording's build identifier automatically; reaction analysis therefore remains unavailable until compatible historical metadata and complete geometry are supplied.

CS:GO extraction supports loose BSP files and PNG/JPEG radars, plus legacy radar/overview resources in split or inline Source VPK v1/v2 archives. DXT1, DXT3 and DXT5 DDS radars are decoded to PNG, including the lower floor when present. Other DDS formats produce no guessed image. BSP faces provide only partial static geometry. Even the referenced [bsp-tracer project](https://github.com/saiko-tech/bsp-tracer) explicitly lists displacements and dynamic entities as unfinished, so it cannot by itself establish complete visibility coverage.

On this machine, the bundled Source2Viewer 20.0 CLI successfully extracted the installed `de_dust2` radar and 1,128 companion physics triangles in approximately ten seconds. The map hash is preserved; scoring remains disabled because coverage and recording-version correspondence have not been established. This real extraction test can be repeated with `CS_DEMO_TEST_GAME`, `CS_DEMO_TEST_S2V`, and optional `CS_DEMO_TEST_CACHE` environment variables via `go test ./internal/maps -run TestExtractLocalOptIn -v`.

Powered by [Source 2 Viewer](https://s2v.app/) ([ValveResourceFormat](https://github.com/ValveResourceFormat/ValveResourceFormat)). AWMH compatibility follows Awpy's published format; no Awpy geometry assets are redistributed.

## Session additions and fixes

- Added safe map-pack import, exact version matching, cached normalized geometry, floor transforms and BVH raycasts.
- Added AWMH, TRI, JSON, GLB/glTF and partial BSP readers with malformed-input tests.
- Added Source2Viewer extraction, historical metadata gates and local Source VPK / DDS radar support.
- Fixed Source2Viewer radar suffix detection, physics-export selection and Valve HUD rotation interpretation after testing real installed assets.
- Added bounds checks before glTF allocations and based cached asset identities on the imported content.
