package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"csdemoreview/worker/internal/analysis"
	"csdemoreview/worker/internal/maps"
	"csdemoreview/worker/internal/model"
	parser "csdemoreview/worker/internal/parser"
	"csdemoreview/worker/internal/store"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const Version = "0.1.0-beta.2"

type Notify func(event string, data any)
type job struct {
	id, path   string
	expectedID string
	ctx        context.Context
	cancel     context.CancelFunc
}
type Service struct {
	Store   *store.Store
	DataDir string
	notify  Notify
	queue   chan *job
	mu      sync.Mutex
	jobs    map[string]*job
	workMu  sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func New(dir string, notify Notify) (*Service, error) {
	dir, e := filepath.Abs(dir)
	if e != nil {
		return nil, e
	}
	s, e := store.Open(filepath.Join(dir, "library.db"))
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithCancel(context.Background())
	v := &Service{Store: s, DataDir: dir, notify: notify, queue: make(chan *job, 128), jobs: map[string]*job{}, ctx: ctx, cancel: cancel}
	v.wg.Add(1)
	go func() {
		defer v.wg.Done()
		// Rules are versioned; reopening a library refreshes outdated derived
		// evidence while keeping original recordings and review notes intact.
		if demos, err := v.Store.Demos(); err == nil {
			for _, d := range demos {
				if d.AnalysisVersion != analysis.Version || needsParserRefresh(d) {
					v.workMu.Lock()
					j := &job{id: "refresh-" + d.ID, path: d.Path, ctx: v.ctx, expectedID: d.ID}
					err := v.refreshCached(j, d)
					if err != nil {
						v.progress(j, "error", 0, err.Error(), d.ID)
					} else {
						v.progress(j, "complete", 1, "Evidence updated for current analysis rules", d.ID)
					}
					v.workMu.Unlock()
				}
			}
		}
		v.runQueue()
	}()
	return v, nil
}

func needsParserRefresh(d model.Demo) bool {
	current := parser.AdapterVersion(d.Engine)
	return current != "" && d.ParserVersion != current
}

// A rules-only refresh cannot recover telemetry the old adapter never saved.
// Reparse accessible sources through the existing atomic promotion path.
func (v *Service) refreshCached(j *job, d model.Demo) error {
	if needsParserRefresh(d) {
		if info, err := os.Stat(d.Path); err == nil && info.Mode().IsRegular() {
			v.progress(j, "parsing", 0, "Updating recorded recoil, direction and spotting telemetry", d.ID)
			return v.importDemo(j)
		}
		warning := "Source demo is unavailable for the telemetry upgrade. Cached review remains usable; restore the original file or import its new location to recover recoil, shot direction and spotting data."
		if !containsWarning(d.Warnings, warning) {
			d.Warnings = append(d.Warnings, warning)
			if err := v.Store.SaveDemo(d); err != nil {
				return err
			}
		}
	}
	return v.analyse(j.ctx, d.ID, func(p float64, m string) { v.progress(j, "analysing", p, m, d.ID) })
}

func containsWarning(warnings []string, value string) bool {
	for _, warning := range warnings {
		if warning == value {
			return true
		}
	}
	return false
}
func (v *Service) Close() {
	v.cancel()
	v.mu.Lock()
	for _, j := range v.jobs {
		j.cancel()
	}
	v.mu.Unlock()
	v.wg.Wait()
}
func ID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func (v *Service) progress(j *job, stage string, value float64, message, id string) {
	v.notify("import-progress", model.ImportProgress{JobID: j.id, Path: j.path, Name: filepath.Base(j.path), Stage: stage, Progress: value, Message: message, DemoID: id})
}
func (v *Service) runQueue() {
	for {
		var j *job
		select {
		case <-v.ctx.Done():
			return
		case j = <-v.queue:
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					v.progress(j, "error", 0, fmt.Sprintf("Import failed safely: %v", r), "")
				}
				v.mu.Lock()
				delete(v.jobs, j.id)
				v.mu.Unlock()
				j.cancel()
			}()
			if j.ctx.Err() != nil {
				v.progress(j, "cancelled", 0, "Import cancelled", "")
				return
			}
			v.workMu.Lock()
			defer v.workMu.Unlock()
			e := v.importDemo(j)
			if e != nil {
				if j.ctx.Err() != nil {
					v.progress(j, "cancelled", 0, "Import cancelled", "")
				} else {
					v.progress(j, "error", 0, e.Error(), "")
				}
			}
		}()
	}
}
func (v *Service) startImport(path, id string) (any, error) {
	if path == "" || len(path) > 32768 {
		return nil, errors.New("invalid demo path")
	}
	path, e := filepath.Abs(path)
	if e != nil {
		return nil, e
	}
	st, e := os.Stat(path)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("demo must be a regular file")
	}
	if id == "" {
		id = ID()
	}
	if len(id) > 128 {
		return nil, errors.New("invalid job ID")
	}
	if err := v.ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(v.ctx)
	j := &job{id: id, path: path, ctx: ctx, cancel: cancel}
	v.mu.Lock()
	if _, exists := v.jobs[id]; exists {
		v.mu.Unlock()
		cancel()
		return nil, errors.New("job ID already exists")
	}
	v.jobs[id] = j
	v.mu.Unlock()
	v.progress(j, "queued", 0, "Waiting to import", "")
	select {
	case v.queue <- j:
		return map[string]string{"jobId": id}, nil
	default:
		cancel()
		v.mu.Lock()
		delete(v.jobs, id)
		v.mu.Unlock()
		v.progress(j, "error", 0, "Import queue is full", "")
		return nil, errors.New("import queue is full")
	}
}
func (v *Service) importDemo(j *job) error {
	engine, e := parser.Detect(j.path)
	if e != nil {
		return e
	}
	f, e := os.Open(j.path)
	if e != nil {
		return e
	}
	info, e := f.Stat()
	if e != nil {
		f.Close()
		return e
	}
	h := sha256.New()
	buf := make([]byte, 1<<20)
	var read int64
	last := time.Time{}
	for {
		if e = j.ctx.Err(); e != nil {
			f.Close()
			return e
		}
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			read += int64(n)
		}
		if time.Since(last) > 250*time.Millisecond {
			last = time.Now()
			v.progress(j, "hashing", float64(read)/float64(info.Size()), "Checking recording identity", "")
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			f.Close()
			return err
		}
	}
	f.Close()
	hash := hex.EncodeToString(h.Sum(nil))
	if j.expectedID != "" {
		cached, err := v.Store.Demo(j.expectedID)
		if err != nil {
			return err
		}
		if cached.Hash != hash {
			return errors.New("source demo contents changed; cached review was preserved. Import the changed recording separately")
		}
	}
	existingID := ""
	if d, e := v.Store.ByHash(hash); e == nil {
		// The content hash is authoritative. Reimporting a moved recording
		// repairs native playback without discarding notes or parsed data.
		d.Path = j.path
		d.Name = filepath.Base(j.path)
		if e = v.Store.SaveDemo(d); e != nil {
			return e
		}
		if d.ParserVersion == parser.AdapterVersion(engine) {
			if j.expectedID == "" {
				v.progress(j, "complete", 1, "Already in your library", d.ID)
			}
			return nil
		}
		existingID = d.ID
	} else if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	id := ID()
	success := false
	defer func() {
		if !success {
			_ = v.Store.Remove(id)
		}
	}()
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	binary := filepath.Join(filepath.Dir(exe), "demo-parser-"+engine)
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	cmd := exec.CommandContext(j.ctx, binary, "--demo", j.path, "--database", filepath.Join(v.DataDir, "library.db"), "--id", id)
	hideProcess(cmd)
	out, e := cmd.StdoutPipe()
	if e != nil {
		return e
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if e = cmd.Start(); e != nil {
		return fmt.Errorf("could not start %s parser: %w", engine, e)
	}
	v.progress(j, "parsing", 0, "Opening recording", id)
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 65536), 4<<20)
	var parsed *model.Demo
	var childError string
	for scan.Scan() {
		var message struct {
			Progress *float64    `json:"progress"`
			Message  string      `json:"message"`
			Demo     *model.Demo `json:"demo"`
			Error    string      `json:"error"`
		}
		if e = json.Unmarshal(scan.Bytes(), &message); e != nil {
			continue
		}
		if message.Progress != nil {
			v.progress(j, "parsing", *message.Progress, message.Message, id)
		}
		if message.Demo != nil {
			parsed = message.Demo
		}
		if message.Error != "" {
			childError = message.Error
		}
	}
	waitErr := cmd.Wait()
	if e = j.ctx.Err(); e != nil {
		return e
	}
	if scan.Err() != nil {
		return scan.Err()
	}
	if childError != "" {
		return errors.New(childError)
	}
	if waitErr != nil {
		return fmt.Errorf("%s parser exited: %v %s", engine, waitErr, truncate(stderr.String(), 1200))
	}
	if parsed == nil {
		return errors.New("parser did not return recording metadata")
	}
	after, e := os.Stat(j.path)
	if e != nil {
		return e
	}
	if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return errors.New("recording changed during import; import it after recording has finished")
	}
	parsed.Hash = hash
	if existingID != "" {
		parsed.Hash = "staging:" + id
	}
	finalStatus := parsed.Status
	parsed.Status = "staging"
	parsed.FileSize = info.Size()
	parsed.Date = info.ModTime().UTC().Format(time.RFC3339)
	parsed.AnalysisVersion = analysis.Version
	parsed.Path = j.path
	parsed.ID = id
	if e = v.Store.SaveDemo(*parsed); e != nil {
		return e
	}
	if existingID != "" {
		if m, e := v.Store.GetMap(existingID); e != nil {
			return e
		} else if m != nil {
			if e = v.Store.SaveMap(id, *m); e != nil {
				return e
			}
		}
	}
	v.progress(j, "analysing", 0, "Evaluating recorded evidence", id)
	if e = v.analyseWithStatus(j.ctx, id, func(p float64, m string) { v.progress(j, "analysing", p, m, id) }, finalStatus); e != nil {
		return e
	}
	final, e := v.Store.Demo(id)
	if e != nil {
		return e
	}
	final.Status = finalStatus
	final.Hash = hash
	finalID := id
	if existingID != "" {
		if e = v.Store.PromoteReimport(id, existingID, final); e != nil {
			return e
		}
		finalID = existingID
		_ = v.Store.Remove(id)
	} else if e = v.Store.SaveDemo(final); e != nil {
		return e
	}
	success = true
	if j.expectedID == "" {
		v.progress(j, "complete", 1, "Ready to review", finalID)
	}
	return nil
}
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
func (v *Service) analyse(ctx context.Context, id string, progress func(float64, string)) error {
	return v.analyseWithStatus(ctx, id, progress, "")
}

func (v *Service) analyseWithStatus(ctx context.Context, id string, progress func(float64, string), recordingStatus string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d, e := v.Store.Demo(id)
	if e != nil {
		return e
	}
	analysisDemo := d
	if recordingStatus != "" {
		analysisDemo.Status = recordingStatus
	}
	players, e := v.Store.Players(id)
	if e != nil {
		return e
	}
	rounds, e := v.Store.Rounds(id)
	if e != nil {
		return e
	}
	events, e := v.Store.Events(id)
	if e != nil {
		return e
	}
	asset, e := v.getMap(id)
	if e != nil {
		return e
	}
	var scene *maps.Scene
	if asset != nil && asset.GeometryPath != "" {
		scene, e = maps.LoadScene(*asset)
		if e != nil {
			d.Warnings = append(d.Warnings, "Map geometry could not be loaded: "+e.Error())
		}
	}
	findings := []model.Finding{}
	for i, p := range players {
		if e = ctx.Err(); e != nil {
			return e
		}
		progress(float64(i)/float64(len(players)), "Analysing "+p.Name)
		if err := ctx.Err(); err != nil {
			return err
		}
		samples, e := v.Store.PlayerSamples(id, p.ID)
		if e != nil {
			return e
		}
		shots, e := v.Store.PlayerShots(id, p.ID)
		if e != nil {
			return e
		}
		var contextErr error
		result := analysis.Analyze(analysis.Input{Demo: analysisDemo, Player: p, Samples: samples, Shots: shots, Events: events, Rounds: rounds, Visibility: scene, Context: func(from, to int) []model.Sample {
			if ctx.Err() != nil {
				return nil
			}
			s, e := v.Store.Samples(id, from, to)
			if e != nil {
				contextErr = e
			}
			return s
		}})
		if contextErr != nil {
			return contextErr
		}
		p.Verdict = result.Verdict
		p.Coverage = result.Coverage
		p.Review = result.Review
		p.Capabilities = result.Capabilities
		p.EvidenceCount = len(result.Findings)
		if e = v.Store.SavePlayer(id, p); e != nil {
			return e
		}
		findings = append(findings, result.Findings...)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if e = v.Store.SaveFindings(id, findings); e != nil {
		return e
	}
	d.AnalysisVersion = analysis.Version
	return v.Store.SaveDemo(d)
}
func (v *Service) getMap(id string) (*model.MapAsset, error) {
	d, e := v.Store.Demo(id)
	if e != nil {
		return nil, e
	}
	var fallback *model.MapAsset
	if m, e := v.Store.GetMap(id); e != nil {
		return nil, e
	} else if m != nil {
		matched := maps.Match(*m, d)
		fallback = &matched
	}
	var assets []model.MapAsset
	_ = v.Store.GetSetting("mapAssets", &assets)
	for i := len(assets) - 1; i >= 0; i-- {
		m := assets[i]
		if m.Engine == d.Engine && m.Map == d.Map {
			matched := maps.Match(m, d)
			if matched.Verified {
				return &matched, nil
			}
			if fallback == nil {
				fallback = &matched
			}
		}
	}
	return fallback, nil
}
func (v *Service) addMap(m model.MapAsset) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	var assets []model.MapAsset
	_ = v.Store.GetSetting("mapAssets", &assets)
	for i, a := range assets {
		if a.ID == m.ID {
			assets[i] = m
			return v.Store.SetSetting("mapAssets", assets)
		}
	}
	return v.Store.SetSetting("mapAssets", append(assets, m))
}
func (v *Service) Call(method string, raw json.RawMessage) (any, error) {
	if err := v.ctx.Err(); err != nil {
		return nil, err
	}
	var p struct {
		ID                string `json:"id"`
		Path              string `json:"path"`
		JobID             string `json:"jobId"`
		FromTick          int    `json:"fromTick"`
		ToTick            int    `json:"toTick"`
		GamePath          string `json:"gamePath"`
		Source2ViewerPath string `json:"source2ViewerPath"`
	}
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if e := json.Unmarshal(raw, &p); e != nil {
		return nil, errors.New("invalid request parameters")
	}
	if len(p.ID) > 128 {
		return nil, errors.New("invalid ID")
	}
	switch method {
	case "listDemos":
		return v.Store.Demos()
	case "getMatch":
		return v.Store.Match(p.ID)
	case "getReplay":
		if p.FromTick < 0 || p.ToTick < p.FromTick || p.ToTick-p.FromTick > 4096 {
			return nil, errors.New("replay window must span at most 4096 nonnegative ticks")
		}
		return v.Store.Replay(p.ID, p.FromTick, p.ToTick)
	case "importDemo":
		return v.startImport(p.Path, p.JobID)
	case "cancelImport":
		v.mu.Lock()
		if j := v.jobs[p.JobID]; j != nil {
			j.cancel()
		}
		v.mu.Unlock()
		return map[string]bool{"ok": true}, nil
	case "removeDemo":
		v.workMu.Lock()
		defer v.workMu.Unlock()
		return map[string]bool{"ok": true}, v.Store.Remove(p.ID)
	case "saveNote":
		var n model.ReviewNote
		if e := json.Unmarshal(raw, &n); e != nil {
			return nil, e
		}
		if n.DemoID == "" || n.Tick < 0 || len(n.Text) > 20000 || (n.Kind != "note" && n.Kind != "bookmark") {
			return nil, errors.New("invalid note")
		}
		d, e := v.Store.Demo(n.DemoID)
		if e != nil {
			return nil, e
		}
		if n.Tick > d.TotalTicks {
			return nil, errors.New("note timestamp is outside the demo")
		}
		if n.ID == "" {
			n.ID = ID()
		}
		n.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		return n, v.Store.SaveNote(n)
	case "deleteNote":
		return map[string]bool{"ok": true}, v.Store.DeleteNote(p.ID)
	case "getMap":
		return v.getMap(p.ID)
	case "importMapPack":
		m, e := maps.ImportPack(p.Path, filepath.Join(v.DataDir, "maps"))
		if e != nil {
			return nil, e
		}
		return m, v.addMap(m)
	case "extractMap":
		d, e := v.Store.Demo(p.ID)
		if e != nil {
			return nil, e
		}
		ctx, cancel := context.WithTimeout(v.ctx, 5*time.Minute)
		defer cancel()
		settings := model.AppSettings{CS2Path: p.GamePath, CSGOPath: p.GamePath, Source2ViewerPath: p.Source2ViewerPath}
		m, e := maps.Extract(ctx, settings, d, filepath.Join(v.DataDir, "maps"))
		if e != nil {
			return nil, e
		}
		if e = v.addMap(m); e != nil {
			return nil, e
		}
		if e = v.Store.SaveMap(p.ID, m); e != nil {
			return nil, e
		}
		return maps.Match(m, d), nil
	case "reanalyse":
		v.workMu.Lock()
		defer v.workMu.Unlock()
		return map[string]bool{"ok": true}, v.analyse(v.ctx, p.ID, func(float64, string) {})
	case "diagnostics":
		return map[string]any{"workerVersion": Version, "libraryPath": v.DataDir, "platform": runtime.GOOS + "/" + runtime.GOARCH, "parserVersions": map[string]string{"cs2": "5.2.0", "csgo": "3.3.0"}}, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", strings.TrimSpace(method))
	}
}
