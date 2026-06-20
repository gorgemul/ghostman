package store

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type RequestEntity struct {
	Id          int64
	Url         string
	Method      string
	Environment string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UpsertRequestParams struct {
	Id          *int64 // nil if insert
	Url         string
	Method      string
	Environment string
}

type Store struct {
	loc      *time.Location
	Requests *sql.DB
}

func (re RequestEntity) FilterValue() string { return string(re.Url) }

func New() (*Store, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", "./requests.db")
	if err != nil {
		return nil, err
	}
	createRequestsStatement := `
	CREATE TABLE IF NOT EXISTS requests (
		id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		url VARCHAR(255),
		method VARCHAR(255),
		environment VARCHAR(255),
		"createdAt" TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		"updatedAt" TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)
`
	if _, err := db.Exec(createRequestsStatement); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{Requests: db, loc: loc}, nil
}

func (s *Store) FindRequests() ([]RequestEntity, error) {
	var reqs []RequestEntity
	rows, err := s.Requests.Query(`SELECT id, url, method, environment, "createdAt", "updatedAt" FROM requests`)
	if err != nil {
		return nil, fmt.Errorf("FindRequests: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var req RequestEntity
		if err := rows.Scan(&req.Id, &req.Url, &req.Method, &req.Environment, &req.CreatedAt, &req.UpdatedAt); err != nil {
			return nil, fmt.Errorf("FindRequests: %v", err)
		}
		reqs = append(reqs, req)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindRequests: %v", err)
	}
	log.Printf("[FindRequests] successfully query %v requests\n", len(reqs))
	return reqs, nil
}

func (s *Store) UpsertRequest(req UpsertRequestParams) (int64, error) {
	requestId := "nil"
	if req.Id != nil {
		requestId = fmt.Sprintf("%d", *req.Id)
	}
	log.Printf("[UpsertRequest] request: id=%v, url=%v, method=%v, environment=%v\n", requestId, req.Url, req.Method, req.Environment)
	// insert
	if req.Id == nil {
		result, err := s.Requests.Exec(`INSERT INTO requests (url, method, environment) VALUES (?, ?, ?);`, req.Url, req.Method, req.Environment)
		if err != nil {
			return 0, fmt.Errorf("UpsertRequest: %v\n", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("UpsertRequest: %v\n", err)
		}
		log.Printf("[UpsertRequest] successfully insert new request#%v\n", id)
		return id, nil
	}
	// update
	result, err := s.Requests.Exec(`UPDATE requests SET "updatedAt" = ?, url = ?, method = ?, environment = ? WHERE id = ?;`, time.Now().UTC(), req.Url, req.Method, req.Environment, *req.Id)
	if err != nil {
		return 0, fmt.Errorf("UpsertRequest: %v\n", err)
	}
	affectedNum, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("UpsertRequest: %v\n", err)
	}
	if affectedNum == 0 {
		return 0, fmt.Errorf("UpsertRequest: request#%v not exist\n", err)
	}
	log.Printf("[UpsertRequest] successfully update request#%v\n", *req.Id)
	return *req.Id, nil
}
