package maps

import (
	"context"
	"csdemoreview/worker/internal/model"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func wall() []Triangle {
	return []Triangle{{{X: 0, Y: -10, Z: -10}, {X: 0, Y: 10, Z: -10}, {X: 0, Y: 10, Z: 10}}, {{X: 0, Y: -10, Z: -10}, {X: 0, Y: 10, Z: 10}, {X: 0, Y: -10, Z: 10}}}
}
func TestVisibilitySegmentAndBoundaries(t *testing.T) {
	scene, e := NewScene(model.MapAsset{}, wall(), true, false)
	if e != nil {
		t.Fatal(e)
	}
	tests := []struct {
		a, b    model.Vec3
		visible bool
	}{{model.Vec3{X: -5}, model.Vec3{X: 5}, false}, {model.Vec3{X: -5, Y: 15}, model.Vec3{X: 5, Y: 15}, true}, {model.Vec3{X: 1}, model.Vec3{X: 5}, true}, {model.Vec3{}, model.Vec3{X: 5}, true}, {model.Vec3{X: 5}, model.Vec3{X: -5}, false}}
	for _, c := range tests {
		if v := scene.Visible(c.a, c.b); v != c.visible {
			t.Fatalf("%+v -> %+v: got %t", c.a, c.b, v)
		}
	}
	if scene.Visible(model.Vec3{X: math.NaN()}, model.Vec3{}) {
		t.Fatal("NaN accepted")
	}
}
func TestBVHTraversesBothBranches(t *testing.T) {
	ts := []Triangle{}
	for y := 0; y < 20; y++ {
		for _, tr := range wall() {
			for i := range tr {
				tr[i].Y += float64(y * 20)
			}
			ts = append(ts, tr)
		}
	}
	s, e := NewScene(model.MapAsset{}, ts, true, false)
	if e != nil {
		t.Fatal(e)
	}
	for y := 0; y < 20; y++ {
		if s.Visible(model.Vec3{X: -2, Y: float64(y * 20)}, model.Vec3{X: 2, Y: float64(y * 20)}) {
			t.Fatal("missed BVH branch")
		}
	}
}
func TestRadarFloors(t *testing.T) {
	a := model.MapAsset{PosX: -100, PosY: 100, Scale: 2, Floors: []model.MapFloor{{Name: "lower", MinZ: -100, MaxZ: 0}, {Name: "upper", MinZ: 0, MaxZ: 100}}}
	x, y := WorldToRadar(a, model.Vec3{X: 0, Y: 0})
	if x != 50 || y != 50 {
		t.Fatal(x, y)
	}
	if FloorAt(a, -1) != 0 || FloorAt(a, 0) != 1 || FloorAt(a, 100) != -1 {
		t.Fatal("floor interval boundary")
	}
	a.Rotate = 90
	x, y = WorldToRadar(a, model.Vec3{})
	if math.Abs(x+50) > 1e-8 || math.Abs(y-50) > 1e-8 {
		t.Fatal("rotation")
	}
}
func TestUnknownVersionNeverVerified(t *testing.T) {
	a := model.MapAsset{Engine: "cs2", Map: "de_test", Version: "v1", Verified: true}
	d := model.Demo{Engine: "cs2", Map: "de_test"}
	if Match(a, d).Verified {
		t.Fatal("trusted incoming verified flag")
	}
	d.MapVersion = "v1"
	if !Match(a, d).Verified {
		t.Fatal("exact version mismatch")
	}
	d.Engine = "csgo"
	if Match(a, d).Verified {
		t.Fatal("wrong engine")
	}
}
func TestAWMHValidation(t *testing.T) {
	b := make([]byte, 16+36+12)
	copy(b, "AWMH")
	binary.LittleEndian.PutUint32(b[4:], 1)
	binary.LittleEndian.PutUint32(b[8:], 3)
	binary.LittleEndian.PutUint32(b[12:], 1)
	binary.LittleEndian.PutUint32(b[16+12+4:], math.Float32bits(1))
	binary.LittleEndian.PutUint32(b[16+24+8:], math.Float32bits(1))
	binary.LittleEndian.PutUint32(b[16+36+4:], 1)
	binary.LittleEndian.PutUint32(b[16+36+8:], 2)
	ts, e := readAWMH(b)
	if e != nil || len(ts) != 1 {
		t.Fatal(e)
	}
	for i := 0; i < len(b); i++ {
		if _, e = readAWMH(b[:i]); e == nil {
			t.Fatal("truncated AWMH accepted", i)
		}
	}
	binary.LittleEndian.PutUint32(b[len(b)-4:], 3)
	if _, e = readAWMH(b); e == nil {
		t.Fatal("invalid index")
	}
}
func TestTriangleFormatRejectsTrailingBytes(t *testing.T) {
	for _, b := range [][]byte{{}, make([]byte, 35), make([]byte, 37)} {
		if _, e := readTriangles(b); e == nil {
			t.Fatal("malformed triangle data accepted")
		}
	}
}
func TestPackImportAndTraversal(t *testing.T) {
	dir := t.TempDir()
	cache := t.TempDir()
	if e := writeTriangles(filepath.Join(dir, "geometry.tri"), wall()); e != nil {
		t.Fatal(e)
	}
	m := Manifest{SchemaVersion: 1, Map: "test", Engine: "cs2", Version: "build:1", Scale: 1, Geometry: "geometry.tri", CompleteGeometry: true}
	save := func() {
		b, _ := json.Marshal(m)
		if e := os.WriteFile(filepath.Join(dir, "map-pack.json"), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	save()
	a, e := ImportPack(filepath.Join(dir, "map-pack.json"), cache)
	if e != nil {
		t.Fatal(e)
	}
	if a.Verified {
		t.Fatal("pack alone cannot prove demo compatibility")
	}
	s, e := LoadScene(a)
	if e != nil || !s.Complete {
		t.Fatal(e)
	}
	m.Geometry = "../outside.tri"
	save()
	if _, e = ImportPack(filepath.Join(dir, "map-pack.json"), cache); e == nil {
		t.Fatal("traversal accepted")
	}
	m.Geometry = "geometry.tri"
	m.Floors = []model.MapFloor{{Name: "a", MinZ: 0, MaxZ: 10}, {Name: "b", MinZ: 5, MaxZ: 15}}
	save()
	if _, e = ImportPack(filepath.Join(dir, "map-pack.json"), cache); e == nil {
		t.Fatal("overlapping floor bounds")
	}
}
func TestGLTFNodeTransform(t *testing.T) {
	dir := t.TempDir()
	bin := make([]byte, 36)
	binary.LittleEndian.PutUint32(bin[12+4:], math.Float32bits(1))
	binary.LittleEndian.PutUint32(bin[24+8:], math.Float32bits(1))
	if e := os.WriteFile(filepath.Join(dir, "mesh.bin"), bin, 0600); e != nil {
		t.Fatal(e)
	}
	data := []byte(`{"asset":{"version":"2.0"},"buffers":[{"uri":"mesh.bin","byteLength":36}],"bufferViews":[{"buffer":0,"byteLength":36}],"accessors":[{"bufferView":0,"componentType":5126,"count":3,"type":"VEC3"}],"meshes":[{"primitives":[{"attributes":{"POSITION":0}}]}],"nodes":[{"mesh":0,"translation":[5,0,0]}],"scenes":[{"nodes":[0]}],"scene":0}`)
	ts, e := readGLTF(filepath.Join(dir, "mesh.gltf"), data)
	if e != nil {
		t.Fatal(e)
	}
	if len(ts) != 1 || ts[0][0].X != 5 || ts[0][1].Y != 1 {
		t.Fatal(ts)
	}
}
func TestExtractLocalOptIn(t *testing.T) {
	dir := os.Getenv("CS_DEMO_TEST_GAME")
	cli := os.Getenv("CS_DEMO_TEST_S2V")
	if dir == "" || cli == "" {
		t.Skip("set CS_DEMO_TEST_GAME and CS_DEMO_TEST_S2V for local installation extraction")
	}
	cache := os.Getenv("CS_DEMO_TEST_CACHE")
	if cache == "" {
		cache = t.TempDir()
	}
	a, e := Extract(context.Background(), model.AppSettings{CS2Path: dir, Source2ViewerPath: cli}, model.Demo{Map: "de_dust2", Engine: "cs2"}, cache)
	if e != nil {
		t.Fatal(e)
	}
	if a.Verified {
		t.Fatal("unknown version incorrectly verified")
	}
	if a.Image == "" && a.GeometryPath == "" {
		t.Fatal("no extracted data")
	}
	t.Logf("extracted %+v", a)
	if a.GeometryPath != "" {
		s, e := LoadScene(a)
		if e != nil {
			t.Fatal(e)
		}
		t.Logf("%d geometry triangles", s.TriangleCount)
	}
}

func TestOverviewFloorMetadata(t *testing.T) {
	text := `"de_nuke" { "verticalsections" { "default" // comment
 { "AltitudeMax" "10000" "AltitudeMin" "-495" } "lower" { "AltitudeMax" "-495" "AltitudeMin" "-10000" } } }`
	floors := overviewFloors(text, t.TempDir(), "de_nuke", "radar.png")
	if len(floors) != 2 || floors[0].Image != "radar.png" || floors[1].MaxZ != -495 {
		t.Fatalf("floor extraction: %+v", floors)
	}
}
func TestDecodeLegacyDDS(t *testing.T) {
	for _, format := range []string{"DXT1", "DXT3", "DXT5"} {
		size := 8
		if format != "DXT1" {
			size = 16
		}
		b := make([]byte, 128+size)
		copy(b, "DDS ")
		binary.LittleEndian.PutUint32(b[4:], 124)
		binary.LittleEndian.PutUint32(b[12:], 4)
		binary.LittleEndian.PutUint32(b[16:], 4)
		copy(b[84:], format)
		offset := 128
		if size == 16 {
			offset += 8
			for i := 128; i < 136; i++ {
				b[i] = 255
			}
		}
		binary.LittleEndian.PutUint16(b[offset:], 0xf800)
		img, e := DecodeDDS(b)
		if e != nil {
			t.Fatal(e)
		}
		c := img.NRGBAAt(0, 0)
		if c.R != 255 || c.G != 0 || c.B != 0 || c.A != 255 {
			t.Fatalf("%s: %+v", format, c)
		}
		if _, e = DecodeDDS(b[:len(b)-1]); e == nil {
			t.Fatal("truncated DDS accepted")
		}
	}
}
func TestVPKInlineAndPreload(t *testing.T) {
	dir := t.TempDir()
	tree := []byte("txt\x00resource/overviews\x00test\x00")
	entry := make([]byte, 18)
	binary.LittleEndian.PutUint16(entry[4:], 3)
	binary.LittleEndian.PutUint16(entry[6:], 0x7fff)
	binary.LittleEndian.PutUint32(entry[12:], 4)
	binary.LittleEndian.PutUint16(entry[16:], 0xffff)
	tree = append(tree, entry...)
	tree = append(tree, []byte("pre\x00\x00\x00")...)
	header := make([]byte, 12)
	binary.LittleEndian.PutUint32(header, 0x55aa1234)
	binary.LittleEndian.PutUint32(header[4:], 1)
	binary.LittleEndian.PutUint32(header[8:], uint32(len(tree)))
	data := append(append(header, tree...), []byte("load")...)
	path := filepath.Join(dir, "pak01_dir.vpk")
	if e := os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	b, e := ReadVPKEntry(path, "resource/overviews/test.txt")
	if e != nil || string(b) != "preload" {
		t.Fatal(string(b), e)
	}
	if _, e = ReadVPKEntry(path, "resource/overviews/no.txt"); !os.IsNotExist(e) {
		t.Fatal("missing resource", e)
	}
}
