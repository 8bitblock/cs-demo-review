package maps

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReadVPKEntry supports Source VPK v1/v2 directory trees and split archives.
// It reads only an exact resource path and bounds both preload and payload.
func ReadVPKEntry(directory, path string) ([]byte, error) {
	f, e := os.Open(directory)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	header := make([]byte, 28)
	if _, e = io.ReadFull(f, header[:12]); e != nil {
		return nil, e
	}
	if binary.LittleEndian.Uint32(header) != 0x55aa1234 {
		return nil, errors.New("invalid VPK signature")
	}
	version := binary.LittleEndian.Uint32(header[4:])
	headerSize := 12
	if version == 2 {
		headerSize = 28
		if _, e = io.ReadFull(f, header[12:]); e != nil {
			return nil, e
		}
	} else if version != 1 {
		return nil, errors.New("unsupported VPK version")
	}
	length := binary.LittleEndian.Uint32(header[8:])
	if length > 64<<20 {
		return nil, errors.New("VPK directory is too large")
	}
	tree := make([]byte, int(length))
	if _, e = io.ReadFull(f, tree); e != nil {
		return nil, e
	}
	p := 0
	token := func() (string, error) {
		if p >= len(tree) {
			return "", io.ErrUnexpectedEOF
		}
		end := bytes.IndexByte(tree[p:], 0)
		if end < 0 {
			return "", io.ErrUnexpectedEOF
		}
		s := string(tree[p : p+end])
		p += end + 1
		return s, nil
	}
	for {
		ext, e := token()
		if e != nil {
			return nil, e
		}
		if ext == "" {
			break
		}
		for {
			folder, e := token()
			if e != nil {
				return nil, e
			}
			if folder == "" {
				break
			}
			for {
				name, e := token()
				if e != nil {
					return nil, e
				}
				if name == "" {
					break
				}
				if p+18 > len(tree) {
					return nil, io.ErrUnexpectedEOF
				}
				preload := int(binary.LittleEndian.Uint16(tree[p+4:]))
				archive := binary.LittleEndian.Uint16(tree[p+6:])
				offset := int64(binary.LittleEndian.Uint32(tree[p+8:]))
				size := int64(binary.LittleEndian.Uint32(tree[p+12:]))
				term := binary.LittleEndian.Uint16(tree[p+16:])
				p += 18
				if term != 0xffff || p+preload > len(tree) {
					return nil, errors.New("invalid VPK entry")
				}
				resource := name
				if ext != " " {
					resource += "." + ext
				}
				if folder != " " {
					resource = folder + "/" + resource
				}
				if resource == path {
					if size+int64(preload) > 32<<20 {
						return nil, errors.New("radar resource exceeds 32 MiB")
					}
					result := make([]byte, preload+int(size))
					copy(result, tree[p:p+preload])
					archiveFile := f
					if archive == 0x7fff {
						offset += int64(headerSize) + int64(length)
					} else {
						base := strings.TrimSuffix(directory, "_dir.vpk")
						if base == directory {
							return nil, errors.New("split VPK must use a _dir.vpk index")
						}
						archiveFile, e = os.Open(fmt.Sprintf("%s_%03d.vpk", base, archive))
						if e != nil {
							return nil, e
						}
						defer archiveFile.Close()
					}
					if _, e = archiveFile.ReadAt(result[preload:], offset); e != nil && !(e == io.EOF && size == 0) {
						return nil, e
					}
					return result, nil
				}
				p += preload
			}
		}
	}
	return nil, os.ErrNotExist
}
func color565(value uint16) color.NRGBA {
	return color.NRGBA{R: uint8((value >> 11) * 255 / 31), G: uint8(((value >> 5) & 63) * 255 / 63), B: uint8((value & 31) * 255 / 31), A: 255}
}
func blend(a, b color.NRGBA, wa, wb, div uint16) color.NRGBA {
	return color.NRGBA{R: uint8((uint16(a.R)*wa + uint16(b.R)*wb) / div), G: uint8((uint16(a.G)*wa + uint16(b.G)*wb) / div), B: uint8((uint16(a.B)*wa + uint16(b.B)*wb) / div), A: 255}
}

// DecodeDDS decodes the BC1/BC2/BC3 formats used by legacy radar images.
// Unsupported DX10/compressed formats fail explicitly instead of drawing noise.
func DecodeDDS(data []byte) (*image.NRGBA, error) {
	if len(data) < 128 || string(data[:4]) != "DDS " || binary.LittleEndian.Uint32(data[4:]) != 124 {
		return nil, errors.New("invalid DDS header")
	}
	height, width := int(binary.LittleEndian.Uint32(data[12:])), int(binary.LittleEndian.Uint32(data[16:]))
	if width < 1 || height < 1 || width > 4096 || height > 4096 {
		return nil, errors.New("DDS dimensions exceed limit")
	}
	fourCC := string(data[84:88])
	blockSize := 8
	switch fourCC {
	case "DXT1":
	case "DXT3", "DXT5":
		blockSize = 16
	default:
		return nil, fmt.Errorf("DDS compression %q is unsupported", fourCC)
	}
	blocksX, blocksY := (width+3)/4, (height+3)/4
	if len(data) < 128+blocksX*blocksY*blockSize {
		return nil, errors.New("truncated DDS blocks")
	}
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for by := 0; by < blocksY; by++ {
		for bx := 0; bx < blocksX; bx++ {
			block := data[128+(by*blocksX+bx)*blockSize:]
			rgb := block
			if blockSize == 16 {
				rgb = block[8:]
			}
			c0, c1 := binary.LittleEndian.Uint16(rgb), binary.LittleEndian.Uint16(rgb[2:])
			palette := [4]color.NRGBA{color565(c0), color565(c1)}
			if c0 > c1 || fourCC != "DXT1" {
				palette[2] = blend(palette[0], palette[1], 2, 1, 3)
				palette[3] = blend(palette[0], palette[1], 1, 2, 3)
			} else {
				palette[2] = blend(palette[0], palette[1], 1, 1, 2)
				palette[3] = color.NRGBA{}
			}
			indices := binary.LittleEndian.Uint32(rgb[4:])
			alpha := [8]uint8{}
			var alphaBits uint64
			if fourCC == "DXT5" {
				alpha[0], alpha[1] = block[0], block[1]
				if alpha[0] > alpha[1] {
					for i := 2; i < 8; i++ {
						alpha[i] = uint8((int(8-i)*int(alpha[0]) + int(i-1)*int(alpha[1])) / 7)
					}
				} else {
					for i := 2; i < 6; i++ {
						alpha[i] = uint8((int(6-i)*int(alpha[0]) + int(i-1)*int(alpha[1])) / 5)
					}
					alpha[6] = 0
					alpha[7] = 255
				}
				for i := 0; i < 6; i++ {
					alphaBits |= uint64(block[i+2]) << uint(i*8)
				}
			}
			for i := 0; i < 16; i++ {
				c := palette[(indices>>uint(i*2))&3]
				if fourCC == "DXT3" {
					n := (block[i/2] >> uint((i%2)*4)) & 15
					c.A = n * 17
				} else if fourCC == "DXT5" {
					c.A = alpha[(alphaBits>>uint(i*3))&7]
				}
				x, y := bx*4+i%4, by*4+i/4
				if x < width && y < height {
					img.SetNRGBA(x, y, c)
				}
			}
		}
	}
	return img, nil
}
func legacyRadar(dir, staging, mapName string) (overview, radar string) {
	read := func(resource string) ([]byte, error) {
		b, e := os.ReadFile(filepath.Join(dir, filepath.FromSlash(resource)))
		if e == nil {
			return b, nil
		}
		return ReadVPKEntry(filepath.Join(dir, "pak01_dir.vpk"), resource)
	}
	if b, e := read("resource/overviews/" + mapName + ".txt"); e == nil {
		overview = filepath.Join(staging, "overview.txt")
		if e = os.WriteFile(overview, b, 0600); e != nil {
			overview = ""
		}
	}
	for _, suffix := range []string{"", "_lower"} {
		name := mapName + suffix + "_radar"
		if b, e := read("resource/overviews/" + name + ".dds"); e == nil {
			if img, e := DecodeDDS(b); e == nil {
				path := filepath.Join(staging, name+".png")
				if f, e := os.Create(path); e == nil {
					e = png.Encode(f, img)
					_ = f.Close()
					if e == nil && suffix == "" {
						radar = name + ".png"
					}
				}
			}
		}
	}
	return overview, radar
}
