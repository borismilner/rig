package watch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/borismilner/rig/internal/paths"
	"github.com/borismilner/rig/internal/store"
)

const stateVersion = 1

// State is the watch namespace in an estate's existing SQLite state area.
// Its owner holds the estate's single-instance lock; Close follows Run's exit.
type State struct {
	db   *sql.DB
	path string
}

// OpenState refuses unnamed estates, as the other durable namespaces do.
func OpenState(ctx context.Context, estate string) (*State, error) {
	dir, err := paths.EstateStateDir(estate)
	if err != nil {
		return nil, err
	}
	return openState(ctx, filepath.Join(dir, "watch.db"))
}

func openState(ctx context.Context, path string) (*State, error) {
	db, err := store.OpenDB(ctx, path, store.Schema{What: "watch state", Version: stateVersion, Create: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE settings (id INTEGER PRIMARY KEY CHECK(id=1), body TEXT NOT NULL);
		CREATE TABLE watches (id TEXT PRIMARY KEY, body TEXT NOT NULL);
		CREATE TABLE snapshots (watch_id TEXT NOT NULL, path TEXT NOT NULL, body TEXT NOT NULL,
		PRIMARY KEY(watch_id,path)) WITHOUT ROWID;`)
		return err
	}})
	if err != nil {
		return nil, err
	}
	return &State{db: db, path: path}, nil
}

func (s *State) Close() error { return s.db.Close() }
func (s *State) Path() string { return s.path }

type pending struct {
	FirstNS int64  `json:"first_ns"`
	LastNS  int64  `json:"last_ns"`
	DueNS   int64  `json:"due_ns"`
	Count   uint64 `json:"count"`
	Gap     bool   `json:"gap"`
}

type groupState struct {
	Group    Group            `json:"group"`
	Sequence uint64           `json:"sequence"`
	Pending  *pending         `json:"pending,omitempty"`
	Entries  map[string]stamp `json:"-"`
}

type stamp struct {
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	Mode     uint32 `json:"mode"`
	Link     string `json:"link,omitempty"`
}

type delta struct {
	path  string
	value *stamp
}
type update struct {
	state  *groupState
	deltas []delta
}

func (s *State) load(ctx context.Context) (Config, map[string]*groupState, error) {
	c := Config{}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT body FROM settings WHERE id=1`).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return c, nil, err
	}
	if err == nil {
		if err = json.Unmarshal([]byte(raw), &c); err != nil {
			return c, nil, err
		}
	}
	states := map[string]*groupState{}
	rows, err := s.db.QueryContext(ctx, `SELECT body FROM watches ORDER BY id`)
	if err != nil {
		return c, nil, err
	}
	for rows.Next() {
		var g groupState
		if err = rows.Scan(&raw); err == nil {
			err = json.Unmarshal([]byte(raw), &g)
		}
		if err != nil {
			_ = rows.Close()
			return c, nil, err
		}
		g.Entries = map[string]stamp{}
		states[g.Group.ID] = &g
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return c, nil, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT watch_id,path,body FROM snapshots`)
	if err != nil {
		return c, nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, path string
		var entry stamp
		if err = rows.Scan(&id, &path, &raw); err == nil {
			err = json.Unmarshal([]byte(raw), &entry)
		}
		if err != nil {
			return c, nil, err
		}
		g := states[id]
		if g == nil {
			return c, nil, fmt.Errorf("watch: snapshot references absent group %q", id)
		}
		g.Entries[path] = entry
	}
	return c, states, rows.Err()
}

// save writes pending work and its snapshot together. Only changed paths are
// written on ordinary events; a file write never rewrites its whole tree.
func (s *State) save(ctx context.Context, c *Config, updates []update, removed []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if c != nil {
		b, e := json.Marshal(c)
		if e != nil {
			return e
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO settings(id,body) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body`, string(b)); err != nil {
			return err
		}
	}
	for _, id := range removed {
		if _, err = tx.ExecContext(ctx, `DELETE FROM snapshots WHERE watch_id=?`, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM watches WHERE id=?`, id); err != nil {
			return err
		}
	}
	for _, u := range updates {
		b, e := json.Marshal(u.state)
		if e != nil {
			return e
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO watches(id,body) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body`, u.state.Group.ID, string(b)); err != nil {
			return err
		}
		for _, d := range u.deltas {
			if d.value == nil {
				_, err = tx.ExecContext(ctx, `DELETE FROM snapshots WHERE watch_id=? AND path=?`, u.state.Group.ID, d.path)
			} else {
				b, e = json.Marshal(d.value)
				if e != nil {
					return e
				}
				_, err = tx.ExecContext(ctx, `INSERT INTO snapshots(watch_id,path,body) VALUES(?,?,?) ON CONFLICT(watch_id,path) DO UPDATE SET body=excluded.body`, u.state.Group.ID, d.path, string(b))
			}
			if err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
