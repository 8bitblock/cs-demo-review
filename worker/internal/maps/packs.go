package maps

import (
	"context"
	"crypto/sha256"
	"csdemoreview/worker/internal/model"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Manifest struct {
	SchemaVersion    int              `json:"schemaVersion"`
	Map              string           `json:"map"`
	Engine           string           `json:"engine"`
	Version          string           `json:"version"`
	Source           string           `json:"source"`
	PosX             float64          `json:"posX"`
	PosY             float64          `json:"posY"`
	Scale            float64          `json:"scale"`
	Rotate           float64          `json:"rotate"`
	Image            string           `json:"image"`
	Floors           []model.MapFloor `json:"floors"`
	Geometry         string           `json:"geometry"`
	Coordinates      string           `json:"coordinates"`
	CompleteGeometry bool             `json:"completeGeometry"`
	DynamicGeometry  bool             `json:"dynamicGeometry"`
}

var mapNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func safeLocal(root, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || strings.ContainsAny(rel, ":\x00") || strings.Contains(rel, "\\") {
		return "", errors.New("pack paths must be relative slash-separated local paths")
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	p, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	p, err = filepath.Abs(p)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, p)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("pack path escapes its folder")
	}
	return p, nil
}
func Match(asset model.MapAsset, demo model.Demo) model.MapAsset {
	asset.Verified = asset.Engine == demo.Engine && asset.Map == demo.Map && asset.Version != "" && demo.MapVersion != "" && asset.Version == demo.MapVersion
	if !asset.Verified {
		asset.Warnings = append(append([]string{}, asset.Warnings...), "Map geometry version is not verified against this recording; visibility-based evidence is disabled.")
	}
	return asset
}
func ImportPack(manifestPath, cacheDir string) (model.MapAsset, error) {
	var zero model.MapAsset
	file, err := os.Open(manifestPath)
	if err != nil {
		return zero, err
	}
	defer file.Close()
	bytes, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return zero, err
	}
	if len(bytes) > 1<<20 {
		return zero, errors.New("map manifest exceeds 1 MiB")
	}
	var m Manifest
	if err = json.Unmarshal(bytes, &m); err != nil {
		return zero, err
	}
	if m.SchemaVersion != 1 || !mapNamePattern.MatchString(m.Map) || (m.Engine != "cs2" && m.Engine != "csgo") || m.Version == "" || len(m.Version) > 256 || !finite(m.Scale) || m.Scale <= 0 || !finite(m.PosX) || !finite(m.PosY) || !finite(m.Rotate) {
		return zero, errors.New("invalid map pack schema, identity, version or transform")
	}
	if m.Coordinates != "" && m.Coordinates != "source" && m.Coordinates != "source2viewer-gltf" {
		return zero, errors.New("unknown map coordinate system")
	}
	for _, floor := range m.Floors {
		if floor.Name == "" || !finite(floor.MinZ) || !finite(floor.MaxZ) || floor.MinZ >= floor.MaxZ {
			return zero, errors.New("invalid map floor bounds")
		}
	}
	for i, f := range m.Floors {
		for j, g := range m.Floors {
			if i < j && f.MinZ < g.MaxZ && g.MinZ < f.MaxZ {
				return zero, errors.New("map floors overlap")
			}
		}
	}
	sum := sha256.Sum256(bytes)
	id := hex.EncodeToString(sum[:12])
	if err = os.MkdirAll(cacheDir, 0700); err != nil {
		return zero, err
	}
	dir, err := os.MkdirTemp(cacheDir, "pack-"+id+"-")
	if err != nil {
		return zero, err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(dir)
		}
	}()
	asset := model.MapAsset{ID: id, Map: m.Map, Engine: m.Engine, Version: m.Version, Source: m.Source, PosX: m.PosX, PosY: m.PosY, Scale: m.Scale, Rotate: m.Rotate, Floors: m.Floors, Warnings: []string{}, Verified: false}
	copyImage := func(rel, name string) (string, error) {
		if rel == "" {
			return "", nil
		}
		source, e := safeLocal(filepath.Dir(manifestPath), rel)
		if e != nil {
			return "", e
		}
		extension := strings.ToLower(filepath.Ext(source))
		if extension != ".png" && extension != ".jpg" && extension != ".jpeg" && extension != ".webp" {
			return "", errors.New("radar images must be PNG, JPEG or WebP")
		}
		destination := filepath.Join(dir, name+extension)
		if e = copyLimited(source, destination, 32<<20); e != nil {
			return "", e
		}
		return destination, nil
	}
	asset.Image, err = copyImage(m.Image, "radar")
	if err != nil {
		return zero, err
	}
	for i := range asset.Floors {
		asset.Floors[i].Image, err = copyImage(asset.Floors[i].Image, fmt.Sprintf("floor-%d", i))
		if err != nil {
			return zero, err
		}
	}
	if m.Geometry != "" {
		source, e := safeLocal(filepath.Dir(manifestPath), m.Geometry)
		if e != nil {
			return zero, e
		}
		triangles, e := ReadGeometry(source)
		if e != nil {
			return zero, e
		}
		if strings.EqualFold(filepath.Ext(source), ".bsp") {
			m.CompleteGeometry = false
			asset.Warnings = append(asset.Warnings, "BSP loading includes faces only; a manifest cannot upgrade this reader to complete collision geometry.")
		}
		if m.Coordinates == "source2viewer-gltf" {
			for i := range triangles {
				for j, v := range triangles[i] {
					triangles[i][j] = model.Vec3{X: v.Z / 0.0254, Y: v.X / 0.0254, Z: v.Y / 0.0254}
				}
			}
		}
		if _, e = NewScene(asset, triangles, m.CompleteGeometry, m.DynamicGeometry); e != nil {
			return zero, e
		}
		asset.GeometryPath = filepath.Join(dir, "geometry.tri")
		if e = writeTriangles(asset.GeometryPath, triangles); e != nil {
			return zero, e
		}
		metadata, _ := json.Marshal(map[string]bool{"completeGeometry": m.CompleteGeometry, "dynamicGeometry": m.DynamicGeometry})
		if e = os.WriteFile(filepath.Join(dir, "geometry-metadata.json"), metadata, 0600); e != nil {
			return zero, e
		}
	}
	if !m.CompleteGeometry {
		asset.Warnings = append(asset.Warnings, "Pack geometry is incomplete; reaction estimates cannot contribute to assessments.")
	}
	if m.DynamicGeometry {
		asset.Warnings = append(asset.Warnings, "Map contains unresolved dynamic geometry; reaction estimates are disabled.")
	}
	// Include content in identity so reimporting a modified pack cannot reuse
	// the identity of previously reviewed geometry with the same manifest.
	identity := sha256.New()
	_, _ = identity.Write(bytes)
	paths := []string{asset.GeometryPath, asset.Image}
	for _, floor := range asset.Floors {
		paths = append(paths, floor.Image)
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		file, e := os.Open(path)
		if e != nil {
			return zero, e
		}
		_, e = io.Copy(identity, file)
		_ = file.Close()
		if e != nil {
			return zero, e
		}
	}
	asset.ID = hex.EncodeToString(identity.Sum(nil)[:12])
	asset.Warnings = append(asset.Warnings, "Historical version identifiers and geometry completeness are supplied by the pack author, not independently certified.")
	stored, _ := json.MarshalIndent(asset, "", "  ")
	if err = os.WriteFile(filepath.Join(dir, "asset.json"), stored, 0600); err != nil {
		return zero, err
	}
	success = true
	return asset, nil
}
func copyLimited(source, destination string, limit int64) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Size() > limit {
		return errors.New("asset is not a regular file or exceeds size limit")
	}
	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(in, limit+1))
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func writeTriangles(path string, ts []Triangle) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	b := make([]byte, 36)
	for _, t := range ts {
		for i, v := range t {
			binary.LittleEndian.PutUint32(b[i*12:], math.Float32bits(float32(v.X)))
			binary.LittleEndian.PutUint32(b[i*12+4:], math.Float32bits(float32(v.Y)))
			binary.LittleEndian.PutUint32(b[i*12+8:], math.Float32bits(float32(v.Z)))
		}
		if _, err = f.Write(b); err != nil {
			return err
		}
	}
	return f.Close()
}
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
func gameDir(path string, cs2 bool) string {
	candidates := []string{path, filepath.Join(path, "csgo")}
	if cs2 {
		candidates = append(candidates, filepath.Join(path, "game", "csgo"))
	}
	for _, p := range candidates {
		if st, e := os.Stat(filepath.Join(p, "maps")); e == nil && st.IsDir() {
			return p
		}
	}
	return path
}
func Extract(ctx context.Context, settings model.AppSettings, demo model.Demo, cacheDir string) (model.MapAsset, error) {
	var zero model.MapAsset
	if !mapNamePattern.MatchString(demo.Map) {
		return zero, errors.New("map name cannot be resolved to a local asset")
	}
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return zero, err
	}
	staging, err := os.MkdirTemp(cacheDir, "extract-")
	if err != nil {
		return zero, err
	}
	defer os.RemoveAll(staging)
	m := Manifest{SchemaVersion: 1, Map: demo.Map, Engine: demo.Engine, Source: "Local game installation", Scale: 1, Coordinates: "source", Floors: []model.MapFloor{}}
	var overview string
	warnings := []string{}
	if demo.Engine == "cs2" {
		if settings.Source2ViewerPath == "" {
			return zero, errors.New("Set the Source2Viewer-CLI executable in Settings to extract local CS2 maps")
		}
		dir := gameDir(settings.CS2Path, true)
		vpk := filepath.Join(dir, "maps", demo.Map+".vpk")
		m.Version, err = hashFile(vpk)
		if err != nil {
			return zero, fmt.Errorf("read local map: %w", err)
		}
		run := func(args ...string) error {
			commandCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(commandCtx, settings.Source2ViewerPath, args...)
			hideWindow(cmd)
			log, err := os.CreateTemp(staging, "extract-log-")
			if err != nil {
				return err
			}
			defer log.Close()
			cmd.Stdout = log
			cmd.Stderr = log
			if err = cmd.Run(); err != nil {
				return fmt.Errorf("Source2Viewer extraction failed: %w", err)
			}
			return nil
		}
		// Export only named resources. No shell interpretation and no writes to the game.
		radarPrefix := "panorama/images/overheadmaps/" + demo.Map
		if e := run("-i", filepath.Join(dir, "pak01_dir.vpk"), "-o", staging+string(filepath.Separator), "--vpk_filepath", radarPrefix+",resource/overviews/"+demo.Map+".txt", "-d"); e != nil {
			warnings = append(warnings, e.Error())
		}
		geometryOut := filepath.Join(staging, "world.glb")
		if e := run("-i", vpk, "-o", geometryOut, "--vpk_filepath", "maps/"+demo.Map+".vmap_c", "-d", "--gltf_export_format", "glb"); e != nil {
			warnings = append(warnings, e.Error())
		} else if _, e = os.Stat(geometryOut); e == nil {
			m.Geometry = "world.glb"
			m.Coordinates = "source2viewer-gltf"
			// The renderer mesh can be hundreds of MiB. Source2Viewer also
			// exports collision geometry, which is the useful raycast input.
			if _, physicsErr := os.Stat(filepath.Join(staging, "world_physics.glb")); physicsErr == nil {
				m.Geometry = "world_physics.glb"
			}
		}
		overview = filepath.Join(staging, "resource", "overviews", demo.Map+".txt")
		_ = filepath.WalkDir(staging, func(path string, entry os.DirEntry, e error) error {
			if e != nil || entry.IsDir() {
				return nil
			}
			name := strings.ToLower(entry.Name())
			if (strings.HasPrefix(name, strings.ToLower(demo.Map+"_radar")) && strings.HasSuffix(name, ".png")) || name == strings.ToLower(demo.Map+".png") {
				m.Image, _ = filepath.Rel(staging, path)
				m.Image = filepath.ToSlash(m.Image)
			}
			return nil
		})
	} else if demo.Engine == "csgo" {
		dir := gameDir(settings.CSGOPath, false)
		bsp := filepath.Join(dir, "maps", demo.Map+".bsp")
		m.Version, err = hashFile(bsp)
		if err != nil {
			return zero, fmt.Errorf("read legacy map: %w", err)
		}
		if err = copyLimited(bsp, filepath.Join(staging, "world.bsp"), 256<<20); err != nil {
			return zero, err
		}
		m.Geometry = "world.bsp"
		overview = filepath.Join(dir, "resource", "overviews", demo.Map+".txt")
		if extractedOverview, radar := legacyRadar(dir, staging, demo.Map); extractedOverview != "" {
			overview = extractedOverview
			m.Image = radar
		}
		for _, ext := range []string{".png", ".jpg"} {
			candidate := filepath.Join(dir, "resource", "overviews", demo.Map+"_radar"+ext)
			if _, e := os.Stat(candidate); e == nil {
				m.Image = "radar" + ext
				if e = copyLimited(candidate, filepath.Join(staging, m.Image), 32<<20); e != nil {
					return zero, e
				}
				break
			}
		}
		warnings = append(warnings, "BSP face geometry omits props, displacement surfaces and dynamic objects; imported complete historical meshes are needed for visibility analysis.")
	} else {
		return zero, errors.New("unsupported game engine")
	}
	if data, e := os.ReadFile(overview); e == nil {
		values := regexp.MustCompile(`"(pos_x|pos_y|scale|rotate)"\s+"?(-?[0-9.]+)"?`).FindAllStringSubmatch(string(data), -1)
		for _, v := range values {
			n, _ := strconv.ParseFloat(v[2], 64)
			switch v[1] {
			case "pos_x":
				m.PosX = n
			case "pos_y":
				m.PosY = n
			case "scale":
				m.Scale = n
			case "rotate":
				// Valve's flag controls rotation of the player's HUD radar;
				// it is not a rotation of the static overview coordinate system.
			}
		}
		m.Floors = overviewFloors(string(data), staging, demo.Map, m.Image)
	} else {
		warnings = append(warnings, "Overview transform was not found; coordinate-grid replay is safer than this radar.")
		m.Image = ""
	}
	if m.Geometry == "" && m.Image == "" {
		return zero, errors.New("no supported local map geometry or radar could be extracted")
	}
	data, _ := json.Marshal(m)
	manifestPath := filepath.Join(staging, "map-pack.json")
	if err = os.WriteFile(manifestPath, data, 0600); err != nil {
		return zero, err
	}
	asset, err := ImportPack(manifestPath, cacheDir)
	if err != nil {
		return zero, err
	}
	asset.Warnings = append(asset.Warnings, warnings...)
	return Match(asset, demo), nil
}
