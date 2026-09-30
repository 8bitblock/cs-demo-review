package maps

import (
	"csdemoreview/worker/internal/model"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Only leaf sections with both altitude bounds are interpreted as floors.
// Comments are stripped before matching so ordinary overview metadata is not
// confused with a nested floor declaration.
func overviewFloors(text, root, mapName, primary string) []model.MapFloor {
	text = regexp.MustCompile(`(?m)//[^\r\n]*`).ReplaceAllString(text, "")
	blocks := regexp.MustCompile(`"([^"\r\n]+)"\s*\{([^{}]*)\}`).FindAllStringSubmatch(text, -1)
	images := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".png") {
			rel, e := filepath.Rel(root, path)
			if e == nil {
				images[strings.ToLower(entry.Name())] = filepath.ToSlash(rel)
			}
		}
		return nil
	})
	floors := []model.MapFloor{}
	for _, block := range blocks {
		var min, max *float64
		for _, v := range regexp.MustCompile(`(?i)"(AltitudeMin|AltitudeMax)"\s*"(-?[0-9.]+)"`).FindAllStringSubmatch(block[2], -1) {
			n, e := strconv.ParseFloat(v[2], 64)
			if e != nil {
				continue
			}
			if strings.EqualFold(v[1], "AltitudeMin") {
				min = &n
			} else {
				max = &n
			}
		}
		if min == nil || max == nil || *min >= *max {
			continue
		}
		image := ""
		if block[1] == "default" {
			image = primary
		} else {
			prefix := strings.ToLower(mapName + "_" + block[1] + "_radar")
			for name, path := range images {
				if strings.HasPrefix(name, prefix) {
					image = path
					break
				}
			}
		}
		floors = append(floors, model.MapFloor{Name: block[1], MinZ: *min, MaxZ: *max, Image: image})
	}
	return floors
}
