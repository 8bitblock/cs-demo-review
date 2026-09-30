package service

import (
	"context"
	"csdemoreview/worker/internal/analysis"
	"csdemoreview/worker/internal/model"
	"csdemoreview/worker/internal/parser"
	"csdemoreview/worker/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTelemetryUpgradeWithUnavailableSourceKeepsReview(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d := model.Demo{ID: "d", Hash: "hash", Engine: "cs2", Path: filepath.Join(t.TempDir(), "missing.dem"), ParserVersion: "demoinfocs v5.2.0 / adapter:3"}
	if !needsParserRefresh(d) {
		t.Fatal("old parser does not request telemetry refresh")
	}
	s.SaveDemo(d)
	s.SavePlayer("d", model.Player{ID: "p"})
	s.SaveNote(model.ReviewNote{ID: "n", DemoID: "d", Text: "Keep me"})
	v := &Service{Store: s, notify: func(string, any) {}}
	j := &job{id: "refresh-d", path: d.Path, ctx: context.Background(), expectedID: d.ID}
	if err = v.refreshCached(j, d); err != nil {
		t.Fatal(err)
	}
	match, err := s.Match("d")
	if err != nil {
		t.Fatal(err)
	}
	if len(match.Notes) != 1 || len(match.Players) != 1 || match.Players[0].Review == nil || match.Demo.AnalysisVersion != analysis.Version || len(match.Demo.Warnings) != 1 {
		t.Fatalf("cached review lost: %+v", match)
	}
	if err = v.refreshCached(j, match.Demo); err != nil {
		t.Fatal(err)
	}
	d, _ = s.Demo("d")
	if len(d.Warnings) != 1 {
		t.Fatal("duplicate upgrade warnings")
	}
	d.ParserVersion = parser.AdapterVersion("cs2")
	if needsParserRefresh(d) {
		t.Fatal("current parser requested refresh")
	}
}

func TestTelemetryRefreshRejectsChangedOriginalBeforeReplacingCache(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	path := filepath.Join(dir, "changed.dem")
	if err = os.WriteFile(path, []byte("PBDEMS2\x00changed"), 0600); err != nil {
		t.Fatal(err)
	}
	d := model.Demo{ID: "d", Hash: "original-content-hash", Engine: "cs2", Path: path, ParserVersion: "old"}
	s.SaveDemo(d)
	s.SaveNote(model.ReviewNote{ID: "n", DemoID: "d", Text: "Keep me"})
	v := &Service{Store: s, DataDir: dir, notify: func(string, any) {}}
	err = v.refreshCached(&job{id: "refresh-d", path: path, ctx: context.Background(), expectedID: "d"}, d)
	if err == nil || !strings.Contains(err.Error(), "contents changed") {
		t.Fatal("changed source not rejected", err)
	}
	match, err := s.Match("d")
	if err != nil || match.Demo.Hash != d.Hash || len(match.Notes) != 1 {
		t.Fatal("cache changed", err)
	}
}

func TestStagedPartialImportKeepsIncompleteCoverage(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.SaveDemo(model.Demo{ID: "d", Hash: "hash", Engine: "cs2", Status: "staging"})
	s.SavePlayer("d", model.Player{ID: "p"})
	v := &Service{Store: s}
	if err = v.analyseWithStatus(context.Background(), "d", func(float64, string) {}, "partial"); err != nil {
		t.Fatal(err)
	}
	match, err := s.Match("d")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(match.Players[0].Coverage, "Recording is partial") || match.Players[0].Verdict == "Low concern" {
		t.Fatal("partial import treated as complete", match.Players)
	}
	if match.Demo.Status != "staging" {
		t.Fatal("staged recording became visible before promotion")
	}
}
