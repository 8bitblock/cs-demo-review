package maps

import (
	"csdemoreview/worker/internal/model"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type matrix [16]float64

func identity() matrix { return matrix{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1} }
func multiply(a, b matrix) matrix {
	var c matrix
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			for k := 0; k < 4; k++ {
				c[x*4+y] += a[k*4+y] * b[x*4+k]
			}
		}
	}
	return c
}
func transform(m matrix, v model.Vec3) model.Vec3 {
	return model.Vec3{X: m[0]*v.X + m[4]*v.Y + m[8]*v.Z + m[12], Y: m[1]*v.X + m[5]*v.Y + m[9]*v.Z + m[13], Z: m[2]*v.X + m[6]*v.Y + m[10]*v.Z + m[14]}
}

type gltfNode struct {
	Mesh        *int
	Children    []int
	Matrix      []float64
	Translation []float64
	Rotation    []float64
	Scale       []float64
}

func localMatrix(n gltfNode) (matrix, error) {
	m := identity()
	if len(n.Matrix) > 0 {
		if len(n.Matrix) != 16 {
			return m, errors.New("invalid node matrix")
		}
		copy(m[:], n.Matrix)
		return m, nil
	}
	if len(n.Rotation) > 0 {
		if len(n.Rotation) != 4 {
			return m, errors.New("invalid node quaternion")
		}
		x, y, z, w := n.Rotation[0], n.Rotation[1], n.Rotation[2], n.Rotation[3]
		m = matrix{1 - 2*(y*y+z*z), 2 * (x*y + z*w), 2 * (x*z - y*w), 0, 2 * (x*y - z*w), 1 - 2*(x*x+z*z), 2 * (y*z + x*w), 0, 2 * (x*z + y*w), 2 * (y*z - x*w), 1 - 2*(x*x+y*y), 0, 0, 0, 0, 1}
	}
	if len(n.Scale) > 0 {
		if len(n.Scale) != 3 {
			return m, errors.New("invalid node scale")
		}
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				m[i*4+j] *= n.Scale[i]
			}
		}
	}
	if len(n.Translation) > 0 {
		if len(n.Translation) != 3 {
			return m, errors.New("invalid node translation")
		}
		copy(m[12:15], n.Translation)
	}
	return m, nil
}
func readGLTF(path string, data []byte) ([]Triangle, error) {
	var bin []byte
	if filepath.Ext(path) == ".glb" {
		if len(data) < 20 || binary.LittleEndian.Uint32(data) != 0x46546c67 || binary.LittleEndian.Uint32(data[4:]) != 2 || int(binary.LittleEndian.Uint32(data[8:])) != len(data) {
			return nil, errors.New("invalid GLB header")
		}
		var js []byte
		for p := 12; p < len(data); {
			if p+8 > len(data) {
				return nil, errors.New("truncated GLB chunk")
			}
			n := int(binary.LittleEndian.Uint32(data[p:]))
			kind := binary.LittleEndian.Uint32(data[p+4:])
			p += 8
			if n < 0 || p+n > len(data) {
				return nil, errors.New("invalid GLB chunk size")
			}
			if kind == 0x4e4f534a {
				js = data[p : p+n]
			}
			if kind == 0x004e4942 {
				bin = data[p : p+n]
			}
			p += n
		}
		data = js
	}
	var g struct {
		Asset   struct{ Version string }
		Buffers []struct {
			URI        string
			ByteLength int
		}
		BufferViews []struct{ Buffer, ByteOffset, ByteLength, ByteStride int }
		Accessors   []struct {
			BufferView                       *int
			ByteOffset, ComponentType, Count int
			Type                             string
			Sparse                           json.RawMessage
		}
		Nodes  []gltfNode
		Meshes []struct {
			Primitives []struct {
				Attributes map[string]int
				Indices    *int
				Mode       *int
			}
		}
		Scenes []struct{ Nodes []int }
		Scene  *int
	}
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, err
	}
	if g.Asset.Version != "2.0" {
		return nil, errors.New("only glTF 2.0 is supported")
	}
	buffers := make([][]byte, len(g.Buffers))
	total := 0
	for i, b := range g.Buffers {
		var err error
		if b.URI == "" {
			buffers[i] = bin
		} else if strings.HasPrefix(b.URI, "data:application/octet-stream;base64,") {
			buffers[i], err = base64.StdEncoding.DecodeString(strings.TrimPrefix(b.URI, "data:application/octet-stream;base64,"))
		} else {
			var p string
			p, err = safeLocal(filepath.Dir(path), b.URI)
			if err == nil {
				var st os.FileInfo
				st, err = os.Stat(p)
				if err == nil && st.Size() > 256<<20 {
					err = errors.New("glTF buffer too large")
				}
				if err == nil {
					buffers[i], err = os.ReadFile(p)
				}
			}
		}
		if err != nil {
			return nil, err
		}
		total += len(buffers[i])
		if total > 256<<20 || b.ByteLength > len(buffers[i]) {
			return nil, errors.New("invalid or oversized glTF buffers")
		}
	}
	readAccessor := func(id int, positions bool) ([][]float64, error) {
		if id < 0 || id >= len(g.Accessors) {
			return nil, errors.New("invalid glTF accessor")
		}
		a := g.Accessors[id]
		if a.BufferView == nil || len(a.Sparse) > 0 || a.Count < 0 || a.Count > 15000000 {
			return nil, errors.New("sparse or excessive glTF accessor unsupported")
		}
		vID := *a.BufferView
		if vID < 0 || vID >= len(g.BufferViews) {
			return nil, errors.New("invalid glTF buffer view")
		}
		v := g.BufferViews[vID]
		if v.Buffer < 0 || v.Buffer >= len(buffers) {
			return nil, errors.New("invalid glTF buffer")
		}
		components, size := 1, 0
		if positions {
			if a.Type != "VEC3" || a.ComponentType != 5126 {
				return nil, errors.New("positions must be float VEC3")
			}
			components = 3
		}
		switch a.ComponentType {
		case 5121:
			size = 1
		case 5123:
			size = 2
		case 5125, 5126:
			size = 4
		default:
			return nil, errors.New("unsupported glTF component")
		}
		stride := v.ByteStride
		if stride == 0 {
			stride = components * size
		}
		if stride < components*size || v.ByteOffset < 0 || a.ByteOffset < 0 || v.ByteLength < 0 {
			return nil, errors.New("invalid glTF stride")
		}
		b := buffers[v.Buffer]
		if v.ByteOffset > len(b) || a.ByteOffset > len(b)-v.ByteOffset || a.ByteOffset > v.ByteLength {
			return nil, errors.New("glTF accessor starts outside its buffer")
		}
		available := len(b) - v.ByteOffset - a.ByteOffset
		if v.ByteLength-a.ByteOffset < available {
			available = v.ByteLength - a.ByteOffset
		}
		if a.Count > 0 {
			if available < components*size || (a.Count > 1 && stride > (available-components*size)/(a.Count-1)) {
				return nil, errors.New("glTF accessor count exceeds its buffer")
			}
		}
		result := make([][]float64, a.Count)
		for i := range result {
			p := v.ByteOffset + a.ByteOffset + i*stride
			if p < 0 || p+components*size > len(b) || p+components*size > v.ByteOffset+v.ByteLength {
				return nil, errors.New("glTF accessor out of bounds")
			}
			result[i] = make([]float64, components)
			for j := 0; j < components; j++ {
				q := p + j*size
				switch a.ComponentType {
				case 5121:
					result[i][j] = float64(b[q])
				case 5123:
					result[i][j] = float64(binary.LittleEndian.Uint16(b[q:]))
				case 5125:
					result[i][j] = float64(binary.LittleEndian.Uint32(b[q:]))
				case 5126:
					result[i][j] = float64(math.Float32frombits(binary.LittleEndian.Uint32(b[q:])))
				}
			}
		}
		return result, nil
	}
	scene := 0
	if g.Scene != nil {
		scene = *g.Scene
	}
	if scene < 0 || scene >= len(g.Scenes) {
		return nil, errors.New("glTF missing scene")
	}
	triangles := []Triangle{}
	active := map[int]bool{}
	visits := 0
	var visit func(int, matrix) error
	visit = func(id int, parent matrix) error {
		visits++
		if visits > 100000 || id < 0 || id >= len(g.Nodes) || active[id] {
			return errors.New("invalid or cyclic glTF node tree")
		}
		active[id] = true
		defer delete(active, id)
		n := g.Nodes[id]
		local, err := localMatrix(n)
		if err != nil {
			return err
		}
		m := multiply(parent, local)
		if n.Mesh != nil {
			if *n.Mesh < 0 || *n.Mesh >= len(g.Meshes) {
				return errors.New("invalid glTF mesh")
			}
			for _, p := range g.Meshes[*n.Mesh].Primitives {
				if p.Mode != nil && *p.Mode != 4 {
					continue
				}
				aid, ok := p.Attributes["POSITION"]
				if !ok {
					continue
				}
				vs, err := readAccessor(aid, true)
				if err != nil {
					return err
				}
				indices := make([]int, 0)
				if p.Indices != nil {
					is, e := readAccessor(*p.Indices, false)
					if e != nil {
						return e
					}
					for _, i := range is {
						indices = append(indices, int(i[0]))
					}
				} else {
					for i := range vs {
						indices = append(indices, i)
					}
				}
				if len(indices)%3 != 0 {
					return errors.New("glTF incomplete triangle")
				}
				for i := 0; i < len(indices); i += 3 {
					var t Triangle
					for j := 0; j < 3; j++ {
						index := indices[i+j]
						if index < 0 || index >= len(vs) {
							return errors.New("glTF vertex index out of bounds")
						}
						v := vs[index]
						t[j] = transform(m, model.Vec3{X: v[0], Y: v[1], Z: v[2]})
					}
					triangles = append(triangles, t)
					if len(triangles) > 5000000 {
						return errors.New("glTF triangle limit exceeded")
					}
				}
			}
		}
		for _, child := range n.Children {
			if err = visit(child, m); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range g.Scenes[scene].Nodes {
		if err := visit(root, identity()); err != nil {
			return nil, fmt.Errorf("glTF: %w", err)
		}
	}
	return triangles, nil
}
