// Package maps loads local geometry without assuming that a matching map name
// proves compatibility with a recording.
package maps

import (
	"csdemoreview/worker/internal/model"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Triangle [3]model.Vec3
type bounds struct{ Min, Max model.Vec3 }
type node struct {
	Box         bounds
	Left, Right *node
	Triangles   []Triangle
}
type Scene struct {
	Asset model.MapAsset
	// Complete is a pack author's declaration, separate from version matching.
	Complete        bool
	DynamicGeometry bool
	root            *node
	TriangleCount   int
}

func sub(a, b model.Vec3) model.Vec3 { return model.Vec3{X: a.X - b.X, Y: a.Y - b.Y, Z: a.Z - b.Z} }
func cross(a, b model.Vec3) model.Vec3 {
	return model.Vec3{X: a.Y*b.Z - a.Z*b.Y, Y: a.Z*b.X - a.X*b.Z, Z: a.X*b.Y - a.Y*b.X}
}
func dot(a, b model.Vec3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func coord(v model.Vec3, a int) float64 {
	if a == 0 {
		return v.X
	}
	if a == 1 {
		return v.Y
	}
	return v.Z
}
func finite(v float64) bool        { return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) < 1e9 }
func validPoint(v model.Vec3) bool { return finite(v.X) && finite(v.Y) && finite(v.Z) }
func NewScene(asset model.MapAsset, triangles []Triangle, complete, dynamic bool) (*Scene, error) {
	if len(triangles) == 0 || len(triangles) > 5000000 {
		return nil, errors.New("geometry must contain 1..5,000,000 triangles")
	}
	clean := make([]Triangle, 0, len(triangles))
	for _, t := range triangles {
		for _, v := range t {
			if !validPoint(v) {
				return nil, errors.New("geometry contains invalid coordinates")
			}
		}
		if dot(cross(sub(t[1], t[0]), sub(t[2], t[0])), cross(sub(t[1], t[0]), sub(t[2], t[0]))) > 1e-12 {
			clean = append(clean, t)
		}
	}
	if len(clean) == 0 {
		return nil, errors.New("geometry contains only degenerate triangles")
	}
	return &Scene{Asset: asset, Complete: complete, DynamicGeometry: dynamic, root: build(clean), TriangleCount: len(clean)}, nil
}
func build(ts []Triangle) *node {
	b := bounds{Min: model.Vec3{X: math.Inf(1), Y: math.Inf(1), Z: math.Inf(1)}, Max: model.Vec3{X: math.Inf(-1), Y: math.Inf(-1), Z: math.Inf(-1)}}
	for _, t := range ts {
		for _, v := range t {
			b.Min.X = math.Min(b.Min.X, v.X)
			b.Min.Y = math.Min(b.Min.Y, v.Y)
			b.Min.Z = math.Min(b.Min.Z, v.Z)
			b.Max.X = math.Max(b.Max.X, v.X)
			b.Max.Y = math.Max(b.Max.Y, v.Y)
			b.Max.Z = math.Max(b.Max.Z, v.Z)
		}
	}
	n := &node{Box: b}
	if len(ts) <= 8 {
		n.Triangles = ts
		return n
	}
	extent := sub(b.Max, b.Min)
	axis := 0
	if extent.Y > extent.X {
		axis = 1
	}
	if extent.Z > coord(extent, axis) {
		axis = 2
	}
	sort.Slice(ts, func(i, j int) bool {
		a, b := ts[i], ts[j]
		return coord(a[0], axis)+coord(a[1], axis)+coord(a[2], axis) < coord(b[0], axis)+coord(b[1], axis)+coord(b[2], axis)
	})
	n.Left = build(ts[:len(ts)/2])
	n.Right = build(ts[len(ts)/2:])
	return n
}
func boxHit(b bounds, o, d model.Vec3) bool {
	lo, hi := 0.0, 1.0
	for axis := 0; axis < 3; axis++ {
		p, v := coord(o, axis), coord(d, axis)
		min, max := coord(b.Min, axis), coord(b.Max, axis)
		if math.Abs(v) < 1e-12 {
			if p < min || p > max {
				return false
			}
			continue
		}
		a, z := (min-p)/v, (max-p)/v
		if a > z {
			a, z = z, a
		}
		lo = math.Max(lo, a)
		hi = math.Min(hi, z)
		if hi < lo {
			return false
		}
	}
	return true
}
func triangleHit(t Triangle, o, d model.Vec3) bool {
	e1, e2 := sub(t[1], t[0]), sub(t[2], t[0])
	h := cross(d, e2)
	a := dot(e1, h)
	if math.Abs(a) < 1e-9 {
		return false
	}
	f := 1 / a
	s := sub(o, t[0])
	u := f * dot(s, h)
	if u < 0 || u > 1 {
		return false
	}
	q := cross(s, e1)
	v := f * dot(d, q)
	if v < 0 || u+v > 1 {
		return false
	}
	distance := f * dot(e2, q)
	return distance > 1e-7 && distance < 1-1e-7
}
func hit(n *node, o, d model.Vec3) bool {
	if n == nil || !boxHit(n.Box, o, d) {
		return false
	}
	for _, t := range n.Triangles {
		if triangleHit(t, o, d) {
			return true
		}
	}
	return hit(n.Left, o, d) || hit(n.Right, o, d)
}
func (s *Scene) Visible(a, b model.Vec3) bool {
	return s != nil && s.root != nil && validPoint(a) && validPoint(b) && !hit(s.root, a, sub(b, a))
}
func (s *Scene) CanScore(d model.Demo) bool {
	return s != nil && s.Complete && !s.DynamicGeometry && Match(s.Asset, d).Verified
}

// Radar transforms are in native radar pixels, before scaling to the canvas.
func WorldToRadar(asset model.MapAsset, v model.Vec3) (float64, float64) {
	if asset.Scale <= 0 {
		return 0, 0
	}
	x, y := (v.X-asset.PosX)/asset.Scale, (asset.PosY-v.Y)/asset.Scale
	angle := asset.Rotate * math.Pi / 180
	return x*math.Cos(angle) - y*math.Sin(angle), x*math.Sin(angle) + y*math.Cos(angle)
}
func FloorAt(asset model.MapAsset, z float64) int {
	for i, f := range asset.Floors {
		if z >= f.MinZ && z < f.MaxZ {
			return i
		}
	}
	return -1
}

func LoadScene(asset model.MapAsset) (*Scene, error) {
	if asset.GeometryPath == "" {
		return nil, errors.New("no geometry is available")
	}
	triangles, err := ReadGeometry(asset.GeometryPath)
	if err != nil {
		return nil, err
	}
	var metadata struct {
		Complete bool `json:"completeGeometry"`
		Dynamic  bool `json:"dynamicGeometry"`
	}
	if data, e := os.ReadFile(filepath.Join(filepath.Dir(asset.GeometryPath), "geometry-metadata.json")); e == nil {
		_ = json.Unmarshal(data, &metadata)
	}
	return NewScene(asset, triangles, metadata.Complete, metadata.Dynamic)
}
func ReadGeometry(path string) ([]Triangle, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if stat.Size() > 256<<20 {
		return nil, errors.New("geometry exceeds the 256 MiB limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".bsp":
		return readBSP(data)
	case ".json":
		var ts []Triangle
		if err = json.Unmarshal(data, &ts); err != nil {
			return nil, err
		}
		return ts, nil
	case ".tri":
		return readTriangles(data)
	case ".mesh":
		return readAWMH(data)
	case ".glb", ".gltf":
		return readGLTF(path, data)
	default:
		return nil, fmt.Errorf("unsupported geometry extension %s", filepath.Ext(path))
	}
}
func readTriangles(data []byte) ([]Triangle, error) {
	if len(data) == 0 || len(data)%36 != 0 {
		return nil, errors.New("triangle data must contain complete 36-byte float32 triangles")
	}
	if len(data)/36 > 5000000 {
		return nil, errors.New("too many triangles")
	}
	ts := make([]Triangle, len(data)/36)
	for i := range ts {
		for j := 0; j < 3; j++ {
			p := i*36 + j*12
			ts[i][j] = model.Vec3{X: float64(math.Float32frombits(binary.LittleEndian.Uint32(data[p:]))), Y: float64(math.Float32frombits(binary.LittleEndian.Uint32(data[p+4:]))), Z: float64(math.Float32frombits(binary.LittleEndian.Uint32(data[p+8:])))}
		}
	}
	return ts, nil
}
