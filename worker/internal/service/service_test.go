package service

import (
	"context"
	"crypto/sha256"
	"csdemoreview/worker/internal/analysis"
	"csdemoreview/worker/internal/model"
	"csdemoreview/worker/internal/parser"
	"csdemoreview/worker/internal/store"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartupRefreshAddsReviewWithoutReimportAndKeepsNotes(t *testing.T) {
	dir := t.TempDir()
	seed, err := store.Open(filepath.Join(dir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = seed.SaveDemo(model.Demo{ID: "old", Hash: "hash", AnalysisVersion: "beta-rules-1.0.1", ParserVersion: parser.AdapterVersion("cs2")}); err != nil {
		t.Fatal(err)
	}
	if err = seed.SavePlayer("old", model.Player{ID: "p", Name: "Recorded player"}); err != nil {
		t.Fatal(err)
	}
	if err = seed.SaveNote(model.ReviewNote{ID: "note", DemoID: "old", Text: "Keep this review"}); err != nil {
		t.Fatal(err)
	}
	if err = seed.Close(); err != nil {
		t.Fatal(err)
	}
	completed := make(chan model.ImportProgress, 1)
	s, err := New(dir, func(event string, data any) {
		if event == "import-progress" {
			p := data.(model.ImportProgress)
			if p.Stage == "complete" || p.Stage == "error" {
				completed <- p
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	defer s.Close()
	select {
	case p := <-completed:
		if p.Stage != "complete" {
			t.Fatal(p.Message)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stale cache was not refreshed")
	}
	match, err := s.Store.Match("old")
	if err != nil {
		t.Fatal(err)
	}
	if match.Demo.AnalysisVersion != analysis.Version || len(match.Players) != 1 || match.Players[0].Review == nil || len(match.Notes) != 1 || match.Notes[0].Text != "Keep this review" {
		t.Fatalf("refresh lost data: %+v", match)
	}
}

func TestLatestExactHistoricalPackOverridesExtractedMap(t *testing.T) {
	s, e := New(t.TempDir(), func(string, any) {})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Store.Close()
	defer s.Close()
	if e = s.Store.SaveDemo(model.Demo{ID: "d", Hash: "h", Engine: "csgo", Map: "de_cache", MapVersion: "crc32:71743acf", AnalysisVersion: analysis.Version}); e != nil {
		t.Fatal(e)
	}
	extracted := model.MapAsset{ID: "local", Engine: "csgo", Map: "de_cache", Version: "sha256:local"}
	if e = s.Store.SaveMap("d", extracted); e != nil {
		t.Fatal(e)
	}
	pack := model.MapAsset{ID: "historic", Engine: "csgo", Map: "de_cache", Version: "crc32:71743acf"}
	if e = s.addMap(pack); e != nil {
		t.Fatal(e)
	}
	pack.ID = "newest"
	if e = s.addMap(pack); e != nil {
		t.Fatal(e)
	}
	got, e := s.getMap("d")
	if e != nil || got == nil || got.ID != "newest" || !got.Verified {
		t.Fatalf("exact newest pack did not supersede extraction: %+v %v", got, e)
	}
	if e = s.Store.SetSetting("mapAssets", []model.MapAsset{{ID: "unrelated", Engine: "csgo", Map: "de_cache", Version: "crc32:11111111"}}); e != nil {
		t.Fatal(e)
	}
	got, e = s.getMap("d")
	if e != nil || got == nil || got.ID != "local" || got.Verified {
		t.Fatalf("local selection fallback lost: %+v %v", got, e)
	}
}

func TestLifecycleCancellationStopsAnalysisAndLaterOperations(t *testing.T) {
	s, e := New(t.TempDir(), func(string, any) {})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Store.Close()
	defer s.Close()
	s.Store.SaveDemo(model.Demo{ID: "d", Hash: "h", AnalysisVersion: analysis.Version})
	s.Store.SavePlayer("d", model.Player{ID: "p"})
	e = s.analyse(s.ctx, "d", func(float64, string) { s.cancel() })
	if !errors.Is(e, context.Canceled) {
		t.Fatalf("analysis ignored lifecycle cancellation: %v", e)
	}
	s.Close()
	for _, method := range []string{"extractMap", "reanalyse"} {
		if _, e = s.Call(method, json.RawMessage(`{"id":"d"}`)); !errors.Is(e, context.Canceled) {
			t.Fatalf("%s accepted work after shutdown: %v", method, e)
		}
	}
}

func TestDuplicateRepairsMovedPathAndPreservesReview(t *testing.T) {
	dir := t.TempDir()
	s, e := New(dir, func(string, any) {})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Store.Close()
	defer s.Close()
	contents := []byte("PBDEMS2\x00identical file contents")
	sum := sha256.Sum256(contents)
	path := filepath.Join(dir, "renamed ü recording.dem")
	if e = os.WriteFile(path, contents, 0600); e != nil {
		t.Fatal(e)
	}
	d := model.Demo{ID: "kept-id", Path: "C:/missing/old.dem", Name: "old.dem", Hash: hex.EncodeToString(sum[:]), AnalysisVersion: analysis.Version, ParserVersion: parser.AdapterVersion("cs2")}
	if e = s.Store.SaveDemo(d); e != nil {
		t.Fatal(e)
	}
	if e = s.Store.SaveNote(model.ReviewNote{ID: "kept-note", DemoID: d.ID, Kind: "note", Text: "Keep my review"}); e != nil {
		t.Fatal(e)
	}
	if e = s.importDemo(&job{id: "test", path: path, ctx: context.Background()}); e != nil {
		t.Fatal(e)
	}
	match, e := s.Store.Match(d.ID)
	if e != nil {
		t.Fatal(e)
	}
	if match.Demo.ID != d.ID || match.Demo.Path != path || match.Demo.Name != filepath.Base(path) || len(match.Notes) != 1 || match.Notes[0].ID != "kept-note" {
		t.Fatalf("bad restored demo %+v", match)
	}
}

func TestRPCValidationAndNoteRoundTrip(t *testing.T) {
	s, e := New(t.TempDir(), func(string, any) {})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Store.Close()
	defer s.Close()
	s.Store.SaveDemo(model.Demo{ID: "d", Hash: "h", TotalTicks: 100})
	for _, raw := range []string{`{"id":"d","fromTick":0,"toTick":4097}`, `{"id":"d","fromTick":-1,"toTick":4}`, `{"id":"d","fromTick":5,"toTick":4}`} {
		if _, e = s.Call("getReplay", json.RawMessage(raw)); e == nil {
			t.Fatal("invalid window accepted")
		}
	}
	value, e := s.Call("saveNote", json.RawMessage(`{"demoId":"d","tick":50,"playerId":"p","kind":"bookmark","text":"Review this angle"}`))
	if e != nil {
		t.Fatal(e)
	}
	note := value.(model.ReviewNote)
	if note.ID == "" || note.CreatedAt == "" {
		t.Fatal("note identity missing")
	}
	m, e := s.Store.Match("d")
	if e != nil || len(m.Notes) != 1 || m.Notes[0].Text != note.Text {
		t.Fatalf("%+v %v", m, e)
	}
	if _, e = s.Call("saveNote", json.RawMessage(`{"demoId":"d","tick":101,"kind":"note"}`)); e == nil {
		t.Fatal("outside timestamp accepted")
	}
	if _, e = s.Call("unknown", nil); e == nil {
		t.Fatal("unknown method accepted")
	}
}
