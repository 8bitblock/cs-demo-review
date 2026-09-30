package store

import (
	"csdemoreview/worker/internal/model"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPreReleaseTimingMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	s.SaveDemo(model.Demo{ID: "d", Hash: "h", AnalysisVersion: "old-rules"})
	b := s.Batch("d")
	b.AddShot(model.Shot{ID: "s", PlayerID: "p", TimingPrecision: 15.625, Impacts: []model.Vec3{}})
	if e = b.Flush(); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(`PRAGMA user_version=1`); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	shots, e := s.PlayerShots("d", "p")
	if e != nil || len(shots) != 1 || shots[0].TimingPrecision != .015625 {
		t.Fatalf("migration result %+v %v", shots, e)
	}
	d, e := s.Demo("d")
	if e != nil || d.AnalysisVersion != "" {
		t.Fatal("old assessments were not invalidated")
	}
	if !shots[0].Ambiguous {
		t.Fatal("old adapter without match eligibility remains usable as evidence")
	}
}

func TestAtomicReimportKeepsIdentityAndReviews(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "library.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, d := range []model.Demo{{ID: "stable", Hash: "hash", Status: "ready", Name: "old"}, {ID: "staging", Hash: "temporary", Status: "staging", Name: "new"}} {
		if e = s.SaveDemo(d); e != nil {
			t.Fatal(e)
		}
	}
	if visible, e := s.Demos(); e != nil || len(visible) != 1 || visible[0].ID != "stable" {
		t.Fatal("unfinished import is visible")
	}
	if e = s.SaveNote(model.ReviewNote{ID: "review", DemoID: "stable", Text: "keep me", Kind: "bookmark"}); e != nil {
		t.Fatal(e)
	}
	s.SaveMap("stable", model.MapAsset{ID: "saved-map"})
	for _, id := range []string{"stable", "staging"} {
		b := s.Batch(id)
		tick := 1
		if id == "staging" {
			tick = 2
		}
		b.AddSample(model.Sample{PlayerID: "p", Tick: tick})
		if e = b.Flush(); e != nil {
			t.Fatal(e)
		}
	}
	s.SaveFindings("staging", []model.Finding{{ID: "finding", DemoID: "staging", PlayerID: "p"}})
	if e = s.PromoteReimport("staging", "stable", model.Demo{ID: "staging", Hash: "hash", Status: "ready", Name: "new"}); e != nil {
		t.Fatal(e)
	}
	m, e := s.Match("stable")
	if e != nil || m.Demo.ID != "stable" || m.Demo.Name != "new" || len(m.Notes) != 1 || len(m.Findings) != 1 || m.Findings[0].DemoID != "stable" {
		t.Fatalf("bad promoted match %+v %v", m, e)
	}
	rows, e := s.PlayerSamples("stable", "p")
	if e != nil || len(rows) != 1 || rows[0].Tick != 2 {
		t.Fatal("old or staging samples survived incorrectly")
	}
	asset, e := s.GetMap("stable")
	if e != nil || asset == nil || asset.ID != "saved-map" {
		t.Fatal("map selection lost")
	}
	if _, e = s.Demo("staging"); e == nil {
		t.Fatal("staging metadata remains")
	}
}

func TestReplayLibraryAndRemoval(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "recording.dem")
	os.WriteFile(source, []byte("untouched"), 0600)
	s, e := Open(filepath.Join(dir, "library.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	d := model.Demo{ID: "d", Hash: "unique", Path: source, Warnings: []string{}}
	if e = s.SaveDemo(d); e != nil {
		t.Fatal(e)
	}
	b := s.Batch("d")
	for _, tick := range []int{1, 5, 10} {
		if e = b.AddSample(model.Sample{Tick: tick, PlayerID: "p"}); e != nil {
			t.Fatal(e)
		}
	}
	b.AddEvent(model.GameEvent{ID: "e", Tick: 5})
	b.AddShot(model.Shot{ID: "s", PlayerID: "p", Tick: 5, Impacts: []model.Vec3{}})
	if e = b.Flush(); e != nil {
		t.Fatal(e)
	}
	w, e := s.Replay("d", 4, 9)
	if e != nil || len(w.Samples) != 1 || w.Samples[0].Tick != 5 || len(w.Events) != 1 || len(w.Shots) != 1 {
		t.Fatalf("bad bounded replay %+v %v", w, e)
	}
	empty, e := s.Replay("d", 20, 30)
	encoded, _ := json.Marshal(empty)
	if e != nil || string(encoded) != "{\"samples\":[],\"events\":[],\"shots\":[],\"fromTick\":20,\"toTick\":30}" {
		t.Fatalf("empty arrays: %s %v", encoded, e)
	}
	if _, e = s.ByHash("unique"); e != nil {
		t.Fatal(e)
	}
	if e = s.SaveNote(model.ReviewNote{ID: "n", DemoID: "d", Text: "<script> remains text", Kind: "note"}); e != nil {
		t.Fatal(e)
	}
	if e = s.Remove("d"); e != nil {
		t.Fatal(e)
	}
	items, e := s.Demos()
	if e != nil || len(items) != 0 {
		t.Fatal("demo not removed")
	}
	bts, e := os.ReadFile(source)
	if e != nil || string(bts) != "untouched" {
		t.Fatal("source changed")
	}
	w, e = s.Replay("d", 0, 20)
	if e != nil || len(w.Samples)+len(w.Events)+len(w.Shots) != 0 {
		t.Fatal("derived rows remain")
	}
}
func TestDuplicateHashAndMissingNoteDemo(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "library.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.SaveDemo(model.Demo{ID: "a", Hash: "h"}); e != nil {
		t.Fatal(e)
	}
	if s.SaveDemo(model.Demo{ID: "b", Hash: "h"}) == nil {
		t.Fatal("duplicate hash accepted")
	}
	if s.SaveNote(model.ReviewNote{ID: "n", DemoID: "missing"}) == nil {
		t.Fatal("orphan note accepted")
	}
}
