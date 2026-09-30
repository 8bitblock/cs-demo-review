package parser

import (
	"context"
	"csdemoreview/worker/internal/model"
	"csdemoreview/worker/internal/store"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func Detect(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	b := make([]byte, 8)
	if _, e = io.ReadFull(f, b); e != nil {
		return "", errors.New("demo is truncated: missing 8-byte signature")
	}
	switch string(b) {
	case "PBDEMS2\x00":
		return "cs2", nil
	case "HL2DEMO\x00":
		return "csgo", nil
	default:
		return "", errors.New("unsupported file signature; expected a CS2 or CS:GO demo")
	}
}
func Team(t int) string {
	switch t {
	case 2:
		return "T"
	case 3:
		return "CT"
	default:
		return "Spectator"
	}
}
func PlayerID(steam uint64, user int) string {
	if steam > 0 {
		return fmt.Sprint(steam)
	}
	return fmt.Sprintf("bot-%d", user)
}
func Finite(v float64) bool  { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func Ptr(v float64) *float64 { return &v }

func FiniteVec(v *model.Vec3) bool {
	return v != nil && Finite(v.X) && Finite(v.Y) && Finite(v.Z)
}

// Preserve warmup and unstarted-match shots for replay but prevent every
// detector family from treating noncompetitive activity as evidence.
func MatchEligibility(s *model.Shot, warmup, matchStarted bool) {
	if warmup {
		s.Ambiguous = true
		s.Provenance += "; noncompetitive warmup; excluded from assessments"
	} else if !matchStarted {
		s.Ambiguous = true
		s.Provenance += "; match has not started or state is unavailable; excluded from assessments"
	}
}
func MapCRCVersion(crc uint32) string {
	if crc == 0 {
		return ""
	}
	return fmt.Sprintf("crc32:%08x", crc)
}
func AdapterVersion(engine string) string {
	return map[string]string{"cs2": "demoinfocs v5.2.0 / adapter:4", "csgo": "demoinfocs v3.3.0 / adapter:4"}[engine]
}

type Collector struct {
	Demo         model.Demo
	Store        *store.Store
	Batch        *store.Batch
	Players      map[string]*model.Player
	Rounds       []model.Round
	Pending      []*model.Shot
	Tick         int
	Time         float64
	Round        int
	SampleCount  int
	Err          error
	seq          int
	lastProgress time.Time
	progress     func(float64, string)
	warns        map[string]bool
	last         map[string]model.Sample
}

func NewCollector(d model.Demo, s *store.Store, p func(float64, string)) *Collector {
	return &Collector{Demo: d, Store: s, Batch: s.Batch(d.ID), Players: map[string]*model.Player{}, Rounds: []model.Round{}, Pending: []*model.Shot{}, progress: p, warns: map[string]bool{}, last: map[string]model.Sample{}}
}
func (c *Collector) Warn(w string) {
	if c.warns[w] {
		return
	}
	c.warns[w] = true
	if len(c.Demo.Warnings) < 30 {
		c.Demo.Warnings = append(c.Demo.Warnings, w)
	}
}
func (c *Collector) Error(e error) {
	if e != nil && c.Err == nil {
		c.Err = e
	}
}
func (c *Collector) ID(prefix string) string { c.seq++; return fmt.Sprintf("%s-%d", prefix, c.seq) }
func (c *Collector) Player(id, name, team string) *model.Player {
	p := c.Players[id]
	if p == nil {
		p = &model.Player{ID: id, Name: name, Team: team, Verdict: "Insufficient data", Capabilities: []model.Capability{}}
		c.Players[id] = p
	}
	p.Name = name
	p.Team = team
	return p
}
func (c *Collector) Sample(s model.Sample) {
	if !Finite(s.X) || !Finite(s.Y) || !Finite(s.Z) || !Finite(s.Yaw) || !Finite(s.Pitch) {
		c.Warn("Non-finite player samples were omitted.")
		return
	}
	if prev, ok := c.last[s.PlayerID]; ok {
		if s.Time > prev.Time && s.Time-prev.Time < 0.2 {
			s.Velocity = math.Hypot(s.X-prev.X, s.Y-prev.Y) / (s.Time - prev.Time)
		} else if s.Tick == prev.Tick && s.Time == prev.Time {
			// A demo can contain several frames for one game tick. The last frame
			// replaces that snapshot in SQLite; do not replace its speed with zero.
			s.Velocity = prev.Velocity
		}
	}
	if !FiniteVec(s.Eye) {
		s.Eye = nil
	}
	if !FiniteVec(s.AimPunch) {
		s.AimPunch = nil
	}
	if s.RecoilIndex != nil && !Finite(*s.RecoilIndex) {
		s.RecoilIndex = nil
	}
	if !Finite(s.Velocity) {
		s.Velocity = 0
	}
	c.last[s.PlayerID] = s
	c.SampleCount++
	c.Error(c.Batch.AddSample(s))
}
func (c *Collector) Event(e model.GameEvent) {
	e.ID = c.ID("event")
	e.Tick = c.Tick
	e.Time = c.Time
	e.Round = c.Round
	c.Error(c.Batch.AddEvent(e))
}
func (c *Collector) StartRound() {
	if len(c.Rounds) > 0 && c.Rounds[len(c.Rounds)-1].EndTick == 0 {
		c.Rounds[len(c.Rounds)-1].EndTick = c.Tick
	}
	c.Round++
	c.Rounds = append(c.Rounds, model.Round{Number: c.Round, StartTick: c.Tick})
	c.Event(model.GameEvent{Kind: "round", Text: fmt.Sprintf("Round %d", c.Round)})
}
func (c *Collector) EndRound(team, reason string) {
	if len(c.Rounds) == 0 {
		c.StartRound()
	}
	r := &c.Rounds[len(c.Rounds)-1]
	r.EndTick = c.Tick
	r.Winner = team
	r.Reason = reason
}
func (c *Collector) Shot(s model.Shot) {
	s.ID = c.ID("shot")
	s.Tick = c.Tick
	s.Time = c.Time
	s.Round = c.Round
	s.Impacts = []model.Vec3{}
	// Shared protocol times, including sampling uncertainty, are in seconds.
	s.TimingPrecision = 1 / math.Max(1, c.Demo.TickRate)
	s.Provenance = "weapon_fire; sampled view angles; impacts are endpoints only" + s.Provenance
	for _, old := range c.Pending {
		if old.PlayerID == s.PlayerID && old.Tick == s.Tick {
			old.Ambiguous = true
			s.Ambiguous = true
		}
	}
	c.Pending = append(c.Pending, &s)
	c.Event(model.GameEvent{Kind: "shot", PlayerID: s.PlayerID, Weapon: s.Weapon, Text: s.Weapon + " fired"})
}
func (c *Collector) Impact(player string, v model.Vec3) {
	if !Finite(v.X) || !Finite(v.Y) || !Finite(v.Z) {
		return
	}
	matches := []*model.Shot{}
	for _, s := range c.Pending {
		if s.PlayerID == player && c.Tick-s.Tick >= 0 && c.Tick-s.Tick <= 2 {
			matches = append(matches, s)
		}
	}
	if len(matches) == 1 {
		matches[0].Impacts = append(matches[0].Impacts, v)
	} else {
		for _, s := range matches {
			s.Ambiguous = true
		}
	}
}
func (c *Collector) FlushShots(all bool) {
	keep := c.Pending[:0]
	for _, s := range c.Pending {
		if all || c.Tick-s.Tick > 8 {
			c.Error(c.Batch.AddShot(*s))
		} else {
			keep = append(keep, s)
		}
	}
	c.Pending = keep
}
func (c *Collector) Frame(tick int, seconds, rate, progress float64) {
	c.Tick = tick
	c.Time = seconds
	if rate > 0 && Finite(rate) {
		c.Demo.TickRate = rate
	}
	if tick > c.Demo.TotalTicks {
		c.Demo.TotalTicks = tick
	}
	if seconds > c.Demo.Duration && Finite(seconds) {
		c.Demo.Duration = seconds
	}
	c.FlushShots(false)
	if time.Since(c.lastProgress) > 300*time.Millisecond {
		c.lastProgress = time.Now()
		if Finite(progress) {
			c.progress(math.Min(.99, math.Max(0, progress)), fmt.Sprintf("Reading tick %d · %d player samples", tick, c.SampleCount))
		}
	}
}
func (c *Collector) Finish(parseErr error) error {
	if parseErr != nil {
		if c.SampleCount < 100 {
			return parseErr
		}
		c.Demo.Status = "partial"
		c.Warn("Parsing stopped early: " + parseErr.Error())
	}
	if c.Err != nil {
		return c.Err
	}
	if c.SampleCount == 0 {
		return errors.New("demo contains no supported player snapshots")
	}
	if c.Demo.Map == "" {
		return errors.New("demo does not contain valid Counter-Strike map metadata")
	}
	c.FlushShots(true)
	if c.Err != nil {
		return c.Err
	}
	if e := c.Batch.Flush(); e != nil {
		return e
	}
	if len(c.Rounds) == 0 {
		c.Rounds = append(c.Rounds, model.Round{Number: 1, StartTick: 0, EndTick: c.Demo.TotalTicks})
	}
	r := &c.Rounds[len(c.Rounds)-1]
	if r.EndTick == 0 {
		r.EndTick = c.Demo.TotalTicks
	}
	for _, r := range c.Rounds {
		if e := c.Store.SaveRound(c.Demo.ID, r); e != nil {
			return e
		}
	}
	ids := make([]string, 0, len(c.Players))
	for id := range c.Players {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if e := c.Store.SavePlayer(c.Demo.ID, *c.Players[id]); e != nil {
			return e
		}
	}
	c.Demo.PlayerCount = len(ids)
	c.Demo.RoundCount = len(c.Rounds)
	return nil
}
func ValidateGame(dir string) error {
	d := strings.ToLower(strings.TrimRight(strings.ReplaceAll(dir, "\\", "/"), "/"))
	if d != "csgo" && !strings.HasSuffix(d, "/csgo") {
		return fmt.Errorf("not a Counter-Strike demo (game directory %q)", dir)
	}
	return nil
}

type EngineFunc func(context.Context, string, *Collector) error

func ChildMain(engine string, parse EngineFunc) {
	path := flag.String("demo", "", "demo path")
	db := flag.String("database", "", "library database")
	id := flag.String("id", "", "demo id")
	flag.Parse()
	enc := json.NewEncoder(os.Stdout)
	fail := func(e error) { enc.Encode(map[string]any{"error": e.Error()}); os.Exit(1) }
	defer func() {
		if r := recover(); r != nil {
			fail(fmt.Errorf("parser panicked on unsupported or corrupt recording: %v", r))
		}
	}()
	got, e := Detect(*path)
	if e != nil {
		fail(e)
	}
	if got != engine {
		fail(errors.New("demo signature does not match parser"))
	}
	s, e := store.Open(*db)
	if e != nil {
		fail(e)
	}
	defer s.Close()
	d := model.Demo{ID: *id, Name: filepath.Base(*path), Path: *path, Engine: engine, Status: "ready", RecordingType: "server / unknown", Warnings: []string{}, ParserVersion: AdapterVersion(engine)}
	c := NewCollector(d, s, func(v float64, m string) { enc.Encode(map[string]any{"progress": v, "message": m}) })
	e = parse(context.Background(), *path, c)
	if e = c.Finish(e); e != nil {
		fail(e)
	}
	enc.Encode(map[string]any{"demo": c.Demo})
}
