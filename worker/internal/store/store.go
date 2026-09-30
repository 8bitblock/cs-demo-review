package store

import (
	"csdemoreview/worker/internal/model"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	DB *sql.DB
	mu sync.Mutex
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var schemaVersion int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&schemaVersion); err != nil {
		db.Close()
		return nil, err
	}
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=10000;
 CREATE TABLE IF NOT EXISTS demos(id TEXT PRIMARY KEY, hash TEXT UNIQUE NOT NULL, data TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS players(demo_id TEXT NOT NULL,id TEXT NOT NULL,data TEXT NOT NULL,PRIMARY KEY(demo_id,id));
 CREATE TABLE IF NOT EXISTS rounds(demo_id TEXT NOT NULL,number INTEGER NOT NULL,data TEXT NOT NULL,PRIMARY KEY(demo_id,number));
 CREATE TABLE IF NOT EXISTS samples(demo_id TEXT NOT NULL,player_id TEXT NOT NULL,tick INTEGER NOT NULL,data TEXT NOT NULL,PRIMARY KEY(demo_id,player_id,tick));
 CREATE INDEX IF NOT EXISTS samples_window ON samples(demo_id,tick);
 CREATE TABLE IF NOT EXISTS events(demo_id TEXT NOT NULL,id TEXT NOT NULL,tick INTEGER NOT NULL,data TEXT NOT NULL,PRIMARY KEY(demo_id,id));
 CREATE INDEX IF NOT EXISTS events_window ON events(demo_id,tick);
 CREATE TABLE IF NOT EXISTS shots(demo_id TEXT NOT NULL,id TEXT NOT NULL,player_id TEXT NOT NULL,tick INTEGER NOT NULL,data TEXT NOT NULL,PRIMARY KEY(demo_id,id));
 CREATE INDEX IF NOT EXISTS shots_window ON shots(demo_id,tick);
 CREATE TABLE IF NOT EXISTS findings(demo_id TEXT NOT NULL,id TEXT NOT NULL,tick INTEGER NOT NULL,data TEXT NOT NULL,PRIMARY KEY(demo_id,id));
 CREATE TABLE IF NOT EXISTS notes(id TEXT PRIMARY KEY,demo_id TEXT NOT NULL,tick INTEGER NOT NULL,data TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY,data TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS maps(demo_id TEXT PRIMARY KEY,data TEXT NOT NULL);
 `)
	if err != nil {
		db.Close()
		return nil, err
	}
	// Migrate the pre-release adapter's millisecond uncertainty into the shared
	// protocol's seconds. Invalidate derived assessments for automatic refresh.
	if schemaVersion < 2 {
		_, err = db.Exec(`UPDATE shots SET data=json_set(data,'$.timingPrecision',json_extract(data,'$.timingPrecision')/1000.0) WHERE json_extract(data,'$.timingPrecision')>1;
		UPDATE demos SET data=json_set(data,'$.analysisVersion',''); PRAGMA user_version=2;`)
		if err != nil {
			db.Close()
			return nil, err
		}
	}
	if schemaVersion < 3 {
		_, err = db.Exec(`UPDATE shots SET data=json_set(data,'$.ambiguous',json('true'),'$.provenance',coalesce(json_extract(data,'$.provenance'),'')||'; stale adapter lacks competitive-phase validation; excluded from assessments') WHERE demo_id IN (SELECT id FROM demos WHERE CAST(substr(coalesce(json_extract(data,'$.parserVersion'),''),instr(coalesce(json_extract(data,'$.parserVersion'),''),' / adapter:')+11) AS INTEGER)<3);
		UPDATE demos SET data=json_insert(json_set(data,'$.analysisVersion',''),'$.warnings[#]','Cached parser predates competitive-phase validation. Reimport the recording to restore evidence.') WHERE CAST(substr(coalesce(json_extract(data,'$.parserVersion'),''),instr(coalesce(json_extract(data,'$.parserVersion'),''),' / adapter:')+11) AS INTEGER)<3;
		PRAGMA user_version=3;`)
		if err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{DB: db}, nil
}
func (s *Store) Close() error      { return s.DB.Close() }
func encode(v any) (string, error) { b, e := json.Marshal(v); return string(b), e }
func (s *Store) SaveDemo(d model.Demo) error {
	v, e := encode(d)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(`INSERT INTO demos(id,hash,data) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET hash=excluded.hash,data=excluded.data`, d.ID, d.Hash, v)
	return e
}
func readOne[T any](db *sql.DB, q string, args ...any) (T, error) {
	var t T
	var data string
	e := db.QueryRow(q, args...).Scan(&data)
	if e != nil {
		return t, e
	}
	e = json.Unmarshal([]byte(data), &t)
	return t, e
}
func readMany[T any](db *sql.DB, q string, args ...any) ([]T, error) {
	out := make([]T, 0)
	rows, e := db.Query(q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		var t T
		if e = rows.Scan(&d); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(d), &t); e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s *Store) Demo(id string) (model.Demo, error) {
	return readOne[model.Demo](s.DB, `SELECT data FROM demos WHERE id=?`, id)
}
func (s *Store) ByHash(hash string) (model.Demo, error) {
	return readOne[model.Demo](s.DB, `SELECT data FROM demos WHERE hash=?`, hash)
}
func (s *Store) Demos() ([]model.Demo, error) {
	return readMany[model.Demo](s.DB, `SELECT data FROM demos WHERE coalesce(json_extract(data,'$.status'),'')!='staging' ORDER BY rowid DESC`)
}

// PromoteReimport atomically replaces derived parse data while retaining the
// stable library ID, bookmarks, notes and per-demo map selection.
func (s *Store) PromoteReimport(stagingID, existingID string, final model.Demo) error {
	if stagingID == existingID {
		return errors.New("staging and existing demo IDs must differ")
	}
	final.ID = existingID
	data, e := encode(final)
	if e != nil {
		return e
	}
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, table := range []string{"players", "rounds", "samples", "events", "shots", "findings"} {
		if _, e = tx.Exec(`DELETE FROM `+table+` WHERE demo_id=?`, existingID); e != nil {
			return e
		}
		if _, e = tx.Exec(`UPDATE `+table+` SET demo_id=? WHERE demo_id=?`, existingID, stagingID); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(`UPDATE findings SET data=json_set(data,'$.demoId',?) WHERE demo_id=?`, existingID, existingID); e != nil {
		return e
	}
	if _, e = tx.Exec(`DELETE FROM demos WHERE id=?`, stagingID); e != nil {
		return e
	}
	result, e := tx.Exec(`UPDATE demos SET hash=?,data=? WHERE id=?`, final.Hash, data, existingID)
	if e != nil {
		return e
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("existing demo was not found for atomic reimport")
	}
	return tx.Commit()
}
func (s *Store) Match(id string) (model.MatchDetail, error) {
	d, e := s.Demo(id)
	if e != nil {
		return model.MatchDetail{}, e
	}
	out := model.MatchDetail{Demo: d}
	if out.Players, e = s.Players(id); e != nil {
		return out, e
	}
	if out.Rounds, e = s.Rounds(id); e != nil {
		return out, e
	}
	if out.Events, e = s.Events(id); e != nil {
		return out, e
	}
	if out.Findings, e = readMany[model.Finding](s.DB, `SELECT data FROM findings WHERE demo_id=? ORDER BY tick`, id); e != nil {
		return out, e
	}
	out.Notes, e = readMany[model.ReviewNote](s.DB, `SELECT data FROM notes WHERE demo_id=? ORDER BY tick`, id)
	return out, e
}
func (s *Store) Players(id string) ([]model.Player, error) {
	return readMany[model.Player](s.DB, `SELECT data FROM players WHERE demo_id=? ORDER BY id`, id)
}
func (s *Store) Rounds(id string) ([]model.Round, error) {
	return readMany[model.Round](s.DB, `SELECT data FROM rounds WHERE demo_id=? ORDER BY number`, id)
}
func (s *Store) Events(id string) ([]model.GameEvent, error) {
	return readMany[model.GameEvent](s.DB, `SELECT data FROM events WHERE demo_id=? ORDER BY tick`, id)
}
func (s *Store) PlayerSamples(id, p string) ([]model.Sample, error) {
	return readMany[model.Sample](s.DB, `SELECT data FROM samples WHERE demo_id=? AND player_id=? ORDER BY tick`, id, p)
}
func (s *Store) PlayerShots(id, p string) ([]model.Shot, error) {
	return readMany[model.Shot](s.DB, `SELECT data FROM shots WHERE demo_id=? AND player_id=? ORDER BY tick`, id, p)
}
func (s *Store) Samples(id string, from, to int) ([]model.Sample, error) {
	return readMany[model.Sample](s.DB, `SELECT data FROM samples WHERE demo_id=? AND tick BETWEEN ? AND ? ORDER BY tick,player_id`, id, from, to)
}
func (s *Store) Replay(id string, from, to int) (model.ReplayWindow, error) {
	out := model.ReplayWindow{FromTick: from, ToTick: to}
	var e error
	if out.Samples, e = s.Samples(id, from, to); e != nil {
		return out, e
	}
	if out.Events, e = readMany[model.GameEvent](s.DB, `SELECT data FROM events WHERE demo_id=? AND tick BETWEEN ? AND ? ORDER BY tick`, id, from, to); e != nil {
		return out, e
	}
	out.Shots, e = readMany[model.Shot](s.DB, `SELECT data FROM shots WHERE demo_id=? AND tick BETWEEN ? AND ? ORDER BY tick`, id, from, to)
	return out, e
}
func (s *Store) SavePlayer(id string, p model.Player) error {
	v, e := encode(p)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(`INSERT OR REPLACE INTO players(demo_id,id,data) VALUES(?,?,?)`, id, p.ID, v)
	return e
}
func (s *Store) SaveRound(id string, r model.Round) error {
	v, e := encode(r)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(`INSERT OR REPLACE INTO rounds(demo_id,number,data) VALUES(?,?,?)`, id, r.Number, v)
	return e
}
func (s *Store) SaveFindings(id string, findings []model.Finding) error {
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`DELETE FROM findings WHERE demo_id=?`, id); e != nil {
		return e
	}
	for _, f := range findings {
		v, e := encode(f)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(`INSERT INTO findings(demo_id,id,tick,data) VALUES(?,?,?,?)`, id, f.ID, f.Tick, v); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *Store) SaveNote(n model.ReviewNote) error {
	if _, e := s.Demo(n.DemoID); e != nil {
		return e
	}
	v, e := encode(n)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(`INSERT INTO notes(id,demo_id,tick,data) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET tick=excluded.tick,data=excluded.data WHERE notes.demo_id=excluded.demo_id`, n.ID, n.DemoID, n.Tick, v)
	return e
}
func (s *Store) DeleteNote(id string) error {
	_, e := s.DB.Exec(`DELETE FROM notes WHERE id=?`, id)
	return e
}
func (s *Store) Remove(id string) error {
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, t := range []string{"players", "rounds", "samples", "events", "shots", "findings", "notes", "maps"} {
		if _, e = tx.Exec(`DELETE FROM `+t+` WHERE demo_id=?`, id); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(`DELETE FROM demos WHERE id=?`, id); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) GetSetting(key string, v any) error {
	var data string
	if e := s.DB.QueryRow(`SELECT data FROM settings WHERE key=?`, key).Scan(&data); e != nil {
		return e
	}
	return json.Unmarshal([]byte(data), v)
}
func (s *Store) SetSetting(key string, v any) error {
	data, e := encode(v)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(`INSERT OR REPLACE INTO settings(key,data) VALUES(?,?)`, key, data)
	return e
}
func (s *Store) GetMap(id string) (*model.MapAsset, error) {
	m, e := readOne[model.MapAsset](s.DB, `SELECT data FROM maps WHERE demo_id=?`, id)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	return &m, e
}
func (s *Store) SaveMap(id string, m model.MapAsset) error {
	data, e := encode(m)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(`INSERT OR REPLACE INTO maps(demo_id,data) VALUES(?,?)`, id, data)
	return e
}

// Batch owns one bounded transaction. It commits periodically, keeping imports
// off the renderer and avoiding retaining the full match in memory.
type Batch struct {
	s       *Store
	id      string
	samples []model.Sample
	events  []model.GameEvent
	shots   []model.Shot
}

func (s *Store) Batch(id string) *Batch { return &Batch{s: s, id: id} }
func (b *Batch) AddSample(v model.Sample) error {
	b.samples = append(b.samples, v)
	if len(b.samples) >= 6000 {
		return b.Flush()
	}
	return nil
}
func (b *Batch) AddEvent(v model.GameEvent) error {
	b.events = append(b.events, v)
	if len(b.events) >= 6000 {
		return b.Flush()
	}
	return nil
}
func (b *Batch) AddShot(v model.Shot) error {
	b.shots = append(b.shots, v)
	if len(b.shots) >= 6000 {
		return b.Flush()
	}
	return nil
}
func (b *Batch) Flush() error {
	if len(b.samples)+len(b.events)+len(b.shots) == 0 {
		return nil
	}
	tx, e := b.s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	put := func(q string, rows [][]any) error {
		st, e := tx.Prepare(q)
		if e != nil {
			return e
		}
		defer st.Close()
		for _, r := range rows {
			if _, e = st.Exec(r...); e != nil {
				return e
			}
		}
		return nil
	}
	rows := make([][]any, 0, len(b.samples))
	for _, s := range b.samples {
		v, e := encode(s)
		if e != nil {
			return fmt.Errorf("sample: %w", e)
		}
		rows = append(rows, []any{b.id, s.PlayerID, s.Tick, v})
	}
	if e = put(`INSERT OR REPLACE INTO samples(demo_id,player_id,tick,data) VALUES(?,?,?,?)`, rows); e != nil {
		return e
	}
	rows = nil
	for _, s := range b.events {
		v, e := encode(s)
		if e != nil {
			return e
		}
		rows = append(rows, []any{b.id, s.ID, s.Tick, v})
	}
	if e = put(`INSERT OR REPLACE INTO events(demo_id,id,tick,data) VALUES(?,?,?,?)`, rows); e != nil {
		return e
	}
	rows = nil
	for _, s := range b.shots {
		v, e := encode(s)
		if e != nil {
			return e
		}
		rows = append(rows, []any{b.id, s.ID, s.PlayerID, s.Tick, v})
	}
	if e = put(`INSERT OR REPLACE INTO shots(demo_id,id,player_id,tick,data) VALUES(?,?,?,?,?)`, rows); e != nil {
		return e
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	b.samples = nil
	b.events = nil
	b.shots = nil
	return nil
}
