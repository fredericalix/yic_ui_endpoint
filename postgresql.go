package main

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/gofrs/uuid"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgreSQL handle the storage of sensor-storage service
type PostgreSQL struct {
	db *sql.DB
}

// Store interface to store LayoutDB type
type Store interface {
	NewLayout(*LayoutDB) error
	DeleteLayout(aid uuid.UUID, lid uuid.UUID) error
	FindLastByAID(aid uuid.UUID) ([]LayoutDB, error)
}

// LayoutDB of a city layout stored in DB
type LayoutDB struct {
	AccountID  uuid.UUID `json:"aid"`
	LayoutID   uuid.UUID `json:"lid"`
	ReceivedAt time.Time `json:"received_at"`

	Data json.RawMessage `json:"data"`
}

// NewPostgreSQL create a new PostgreSQL aut.Store
func NewPostgreSQL(uri string) Store {
	db, err := sql.Open("postgres", uri)
	if err != nil {
		return nil
	}
	// test the connection
	err = db.Ping()
	if err != nil {
		panic(err)
	}

	pg := &PostgreSQL{db: db}
	if err := pg.createSchema(); err != nil {
		panic(err)
	}
	return pg
}

func (pg *PostgreSQL) createSchema() (err error) {
	query := `CREATE TABLE IF NOT EXISTS layout (
		aid UUID NOT NULL,
		lid UUID NOT NULL,
		received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		data JSONB NOT NULL
	);
	CREATE INDEX IF NOT EXISTS layout_aid_lid_rec ON layout (aid,lid,received_at);`
	_, err = pg.db.Query(query)
	if err != nil {
		return
	}

	return
}

// NewLayout store a LayoutDB into DB
func (pg *PostgreSQL) NewLayout(l *LayoutDB) error {
	query := `INSERT INTO layout(aid, lid, received_at, data) VALUES($1,$2,$3,$4)`
	_, err := pg.db.Exec(query, l.AccountID, l.LayoutID, l.ReceivedAt, l.Data)
	return err
}

// DeleteLayout delete an ui layout and its complete history
func (pg *PostgreSQL) DeleteLayout(aid uuid.UUID, lid uuid.UUID) error {
	query := `DELETE FROM layout WHERE aid = $1 and lid = $2;`
	_, err := pg.db.Exec(query, aid, lid)
	return err
}

// FindLastByAID find the latest sensors data for each sensor id from a given account id
func (pg *PostgreSQL) FindLastByAID(aid uuid.UUID) (layouts []LayoutDB, err error) {
	query := `SELECT l.aid, l.lid, l.received_at, l.data FROM layout as l
	JOIN (
		SELECT lid, MAX(received_at) AS maxt
		FROM layout
		WHERE aid = $1
		GROUP BY lid
	) m 
	ON m.maxt = l.received_at AND l.lid = m.lid AND l.aid = $1;`
	rows, err := pg.db.Query(query, aid)
	if err == sql.ErrNoRows {
		return layouts, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var l LayoutDB
		err = rows.Scan(&l.AccountID, &l.LayoutID, &l.ReceivedAt, &l.Data)
		if err != nil {
			return nil, err
		}
		layouts = append(layouts, l)
	}
	return layouts, nil
}
