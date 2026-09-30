package store

import (
	"csdemoreview/worker/internal/model"
	"path/filepath"
	"testing"
)

func TestPhaseMigrationPreservesNewerAdapters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	d := model.Demo{ID: "new", Hash: "hash", ParserVersion: "demoinfocs v5.2.0 / adapter:4"}
	s.SaveDemo(d)
	b := s.Batch(d.ID)
	b.AddShot(model.Shot{ID: "shot", PlayerID: "p", TimingPrecision: 1.0 / 64})
	if err = b.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`PRAGMA user_version=2`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	shots, err := s.PlayerShots(d.ID, "p")
	if err != nil || len(shots) != 1 || shots[0].Ambiguous {
		t.Fatal("new adapter invalidated by historical migration", shots, err)
	}
}
