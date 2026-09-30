package maps

import (
	"csdemoreview/worker/internal/model"
	"encoding/binary"
	"errors"
	"math"
)

// AWMH v1 layout is documented by Awpy's geometry.rs (MIT).
func readAWMH(data []byte) ([]Triangle, error) {
	if len(data) < 16 || string(data[:4]) != "AWMH" || binary.LittleEndian.Uint32(data[4:]) != 1 {
		return nil, errors.New("invalid or unsupported AWMH header")
	}
	nv, nt := uint64(binary.LittleEndian.Uint32(data[8:])), uint64(binary.LittleEndian.Uint32(data[12:]))
	if nv > 15000000 || nt > 5000000 || 16+nv*12+nt*12 != uint64(len(data)) {
		return nil, errors.New("invalid AWMH counts or truncated payload")
	}
	vs := make([]model.Vec3, int(nv))
	for i := range vs {
		p := 16 + i*12
		vs[i] = model.Vec3{X: f32(data, p), Y: f32(data, p+4), Z: f32(data, p+8)}
	}
	ts := make([]Triangle, int(nt))
	for i := range ts {
		for j := 0; j < 3; j++ {
			index := binary.LittleEndian.Uint32(data[16+int(nv)*12+i*12+j*4:])
			if uint64(index) >= nv {
				return nil, errors.New("AWMH vertex index is out of bounds")
			}
			ts[i][j] = vs[index]
		}
	}
	return ts, nil
}
func f32(b []byte, p int) float64 {
	return float64(math.Float32frombits(binary.LittleEndian.Uint32(b[p:])))
}

// Source BSP face triangulation is a visual approximation. Static prop models,
// displacements and moving entities are absent; extraction never marks it complete.
func readBSP(b []byte) ([]Triangle, error) {
	if len(b) < 1036 || string(b[:4]) != "VBSP" {
		return nil, errors.New("invalid BSP header")
	}
	version := binary.LittleEndian.Uint32(b[4:])
	if version < 19 || version > 21 {
		return nil, errors.New("unsupported BSP version")
	}
	lump := func(n, size int) ([]byte, error) {
		p := 8 + n*16
		off, length := uint64(binary.LittleEndian.Uint32(b[p:])), uint64(binary.LittleEndian.Uint32(b[p+4:]))
		if off+length > uint64(len(b)) || length%uint64(size) != 0 {
			return nil, errors.New("invalid BSP lump")
		}
		data := b[off : off+length]
		if len(data) >= 4 && string(data[:4]) == "LZMA" {
			return nil, errors.New("compressed BSP lumps are unsupported")
		}
		return data, nil
	}
	vertices, e := lump(3, 12)
	if e != nil {
		return nil, e
	}
	edges, e := lump(12, 4)
	if e != nil {
		return nil, e
	}
	surf, e := lump(13, 4)
	if e != nil {
		return nil, e
	}
	faces, e := lump(7, 56)
	if e != nil {
		return nil, e
	}
	get := func(index uint16) (model.Vec3, error) {
		p := int(index) * 12
		if p+12 > len(vertices) {
			return model.Vec3{}, errors.New("BSP vertex index out of bounds")
		}
		return model.Vec3{X: f32(vertices, p), Y: f32(vertices, p+4), Z: f32(vertices, p+8)}, nil
	}
	triangles := []Triangle{}
	for p := 0; p < len(faces); p += 56 {
		start := int(int32(binary.LittleEndian.Uint32(faces[p+4:])))
		count := int(int16(binary.LittleEndian.Uint16(faces[p+8:])))
		if start < 0 || count < 0 || (start+count)*4 > len(surf) {
			return nil, errors.New("BSP face has invalid edges")
		}
		if count < 3 {
			continue
		}
		polygon := make([]model.Vec3, 0, count)
		for j := 0; j < count; j++ {
			edge := int(int32(binary.LittleEndian.Uint32(surf[(start+j)*4:])))
			side := 0
			if edge < 0 {
				edge = -edge
				side = 2
			}
			if edge*4+4 > len(edges) {
				return nil, errors.New("BSP edge index out of bounds")
			}
			v, err := get(binary.LittleEndian.Uint16(edges[edge*4+side:]))
			if err != nil {
				return nil, err
			}
			polygon = append(polygon, v)
		}
		for j := 1; j+1 < len(polygon); j++ {
			triangles = append(triangles, Triangle{polygon[0], polygon[j], polygon[j+1]})
		}
		if len(triangles) > 5000000 {
			return nil, errors.New("BSP triangle limit exceeded")
		}
	}
	return triangles, nil
}
