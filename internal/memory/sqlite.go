package memory

import (
	"database/sql"
	"encoding/json"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

type sqliteStore struct {
	db *sql.DB
	mu sync.Mutex
}

func newSQLiteStore(dbPath string) (*sqliteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	store := &sqliteStore{db: db}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *sqliteStore) initSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			data BLOB NOT NULL
		);
		CREATE TABLE IF NOT EXISTS dead_jobs (
			id TEXT PRIMARY KEY,
			data BLOB NOT NULL
		);
		CREATE TABLE IF NOT EXISTS crons (
			name TEXT PRIMARY KEY,
			data BLOB NOT NULL
		);
		CREATE TABLE IF NOT EXISTS workflows (
			id TEXT PRIMARY KEY,
			data BLOB NOT NULL
		);
		CREATE TABLE IF NOT EXISTS unique_keys (
			fingerprint TEXT PRIMARY KEY,
			job_id TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS queue_state (
			name TEXT PRIMARY KEY,
			status TEXT NOT NULL
		);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *sqliteStore) SaveJob(job *core.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(job)
	if err != nil {
		return err
	}

	_, err = s.db.Exec("INSERT OR REPLACE INTO jobs (id, data) VALUES (?, ?)", job.ID, data)
	return err
}

func (s *sqliteStore) DeleteJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM jobs WHERE id = ?", id)
	return err
}

func (s *sqliteStore) SaveDeadJob(job *core.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(job)
	if err != nil {
		return err
	}

	_, err = s.db.Exec("INSERT OR REPLACE INTO dead_jobs (id, data) VALUES (?, ?)", job.ID, data)
	return err
}

func (s *sqliteStore) DeleteDeadJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM dead_jobs WHERE id = ?", id)
	return err
}

func (s *sqliteStore) SaveCron(cron *core.CronJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(cron)
	if err != nil {
		return err
	}

	_, err = s.db.Exec("INSERT OR REPLACE INTO crons (name, data) VALUES (?, ?)", cron.Name, data)
	return err
}

func (s *sqliteStore) DeleteCron(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM crons WHERE name = ?", name)
	return err
}

func (s *sqliteStore) SaveWorkflow(id string, state *workflowState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	_, err = s.db.Exec("INSERT OR REPLACE INTO workflows (id, data) VALUES (?, ?)", id, data)
	return err
}

func (s *sqliteStore) DeleteWorkflow(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM workflows WHERE id = ?", id)
	return err
}

func (s *sqliteStore) SaveUniqueKey(fingerprint, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("INSERT OR REPLACE INTO unique_keys (fingerprint, job_id) VALUES (?, ?)", fingerprint, jobID)
	return err
}

func (s *sqliteStore) DeleteUniqueKey(fingerprint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM unique_keys WHERE fingerprint = ?", fingerprint)
	return err
}

func (s *sqliteStore) SaveQueueState(name, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("INSERT OR REPLACE INTO queue_state (name, status) VALUES (?, ?)", name, status)
	return err
}

func (s *sqliteStore) LoadAll() (*persistedState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := &persistedState{
		Jobs:       make(map[string]*core.Job),
		DeadJobs:   make(map[string]*core.Job),
		Crons:      make(map[string]*core.CronJob),
		Workflows:  make(map[string]*workflowState),
		UniqueKeys: make(map[string]string),
		QueueState: make(map[string]string),
	}

	// Load jobs
	rows, err := s.db.Query("SELECT id, data FROM jobs")
	if err != nil {
		return state, nil
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			continue
		}
		var job core.Job
		if err := json.Unmarshal(data, &job); err != nil {
			continue
		}
		state.Jobs[id] = &job
	}

	// Load dead jobs
	rows, err = s.db.Query("SELECT id, data FROM dead_jobs")
	if err != nil {
		return state, nil
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			continue
		}
		var job core.Job
		if err := json.Unmarshal(data, &job); err != nil {
			continue
		}
		state.DeadJobs[id] = &job
	}

	// Load crons
	rows, err = s.db.Query("SELECT name, data FROM crons")
	if err != nil {
		return state, nil
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var data []byte
		if err := rows.Scan(&name, &data); err != nil {
			continue
		}
		var cron core.CronJob
		if err := json.Unmarshal(data, &cron); err != nil {
			continue
		}
		state.Crons[name] = &cron
	}

	// Load workflows
	rows, err = s.db.Query("SELECT id, data FROM workflows")
	if err != nil {
		return state, nil
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			continue
		}
		var wf workflowState
		if err := json.Unmarshal(data, &wf); err != nil {
			continue
		}
		state.Workflows[id] = &wf
	}

	// Load unique keys
	rows, err = s.db.Query("SELECT fingerprint, job_id FROM unique_keys")
	if err != nil {
		return state, nil
	}
	defer rows.Close()

	for rows.Next() {
		var fingerprint, jobID string
		if err := rows.Scan(&fingerprint, &jobID); err != nil {
			continue
		}
		state.UniqueKeys[fingerprint] = jobID
	}

	// Load queue state
	rows, err = s.db.Query("SELECT name, status FROM queue_state")
	if err != nil {
		return state, nil
	}
	defer rows.Close()

	for rows.Next() {
		var name, status string
		if err := rows.Scan(&name, &status); err != nil {
			continue
		}
		state.QueueState[name] = status
	}

	return state, nil
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}
