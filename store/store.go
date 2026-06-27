package store

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	requestMethodConfig      = "all"
	requestEnvironmentConfig = "all"
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

type Config struct {
	Method      string
	Environment string
}

type Store struct {
	db *sql.DB
}

func (re RequestEntity) FilterValue() string { return string(re.Url) }

func New() (*Store, error) {
	db, err := sql.Open("sqlite3", "./ghostman.db")
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
	createConfigsStatement := `
	CREATE TABLE IF NOT EXISTS configs (
		method varchar(255),
		environment varchar(255),
		"createdAt" TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		"updatedAt" TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
`
	if _, err := db.Exec(createRequestsStatement); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(createConfigsStatement); err != nil {
		db.Close()
		return nil, err
	}
	if err := initConfig(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() {
	s.db.Close()
}

func (s *Store) FindRequests() ([]RequestEntity, error) {
	var (
		reqs []RequestEntity
		rows *sql.Rows
		err  error
	)
	log.Printf("FindRequests config:%v, env: %v\n", requestMethodConfig, requestEnvironmentConfig)
	if requestMethodConfig == "all" && requestEnvironmentConfig == "all" {
		rows, err = s.db.Query(`SELECT id, url, method, environment, "createdAt", "updatedAt" FROM requests;`)
	} else if requestMethodConfig == "all" {
		rows, err = s.db.Query(`SELECT id, url, method, environment, "createdAt", "updatedAt" FROM requests WHERE environment = ?;`, requestEnvironmentConfig)
	} else if requestEnvironmentConfig == "all" {
		rows, err = s.db.Query(`SELECT id, url, method, environment, "createdAt", "updatedAt" FROM requests WHERE method = ?;`, requestMethodConfig)
	} else { // both not "all"
		rows, err = s.db.Query(`SELECT id, url, method, environment, "createdAt", "updatedAt" FROM requests WHERE environment = ? AND method = ?;`, requestEnvironmentConfig, requestMethodConfig)
	}
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

func (s *Store) UpsertRequest(params UpsertRequestParams) (int64, error) {
	requestId := "nil"
	if params.Id != nil {
		requestId = fmt.Sprintf("%d", *params.Id)
	}
	log.Printf("[UpsertRequest] request: id=%v, url=%v, method=%v, environment=%v\n", requestId, params.Url, params.Method, params.Environment)
	// insert
	if params.Id == nil {
		result, err := s.db.Exec(`INSERT INTO requests (url, method, environment) VALUES (?, ?, ?);`, params.Url, params.Method, params.Environment)
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
	result, err := s.db.Exec(`UPDATE requests SET "updatedAt" = ?, url = ?, method = ?, environment = ? WHERE id = ?;`, time.Now().UTC(), params.Url, params.Method, params.Environment, *params.Id)
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
	log.Printf("[UpsertRequest] successfully update request#%v\n", *params.Id)
	return *params.Id, nil
}

func (s *Store) FindConfig() Config {
	return Config{Method: requestMethodConfig, Environment: requestEnvironmentConfig}
}

func (s *Store) UpdateMethodConfig(newMethod string) error {
	log.Printf("[UpdateMethodConfig] newMethod=%s\n", newMethod)
	result, err := s.db.Exec(`UPDATE configs SET "updatedAt" = ?, method = ?;`, time.Now().UTC(), newMethod)
	if err != nil {
		return fmt.Errorf("UpdateMethodConfig: %v\n", err)
	}
	affectedNum, err := result.RowsAffected()
	if err != nil || affectedNum == 0 {
		return fmt.Errorf("UpdateMethodConfig: %v\n", err)
	}
	log.Printf("[UpdateMethodConfig] successfully update configs with newMethod=%s\n", newMethod)
	requestMethodConfig = newMethod
	return nil
}

func (s *Store) UpdateEnvironmentConfig(newEnvironment string) error {
	log.Printf("[UpdateMethodConfig] newEnvironment=%s\n", newEnvironment)
	result, err := s.db.Exec(`UPDATE configs SET "updatedAt" = ?, environment = ?;`, time.Now().UTC(), newEnvironment)
	if err != nil {
		return fmt.Errorf("UpdateEnvironmentConfig: %v\n", err)
	}
	affectedNum, err := result.RowsAffected()
	if err != nil || affectedNum == 0 {
		return fmt.Errorf("UpdateEnvironmentConfig: %v\n", err)
	}
	log.Printf("[UpdateEnvironmentConfig] successfully update configs with newEnvironment=%s\n", newEnvironment)
	requestEnvironmentConfig = newEnvironment
	return nil
}

// seed config if not exist, else return existing config
func initConfig(db *sql.DB) error {
	var config Config
	row := db.QueryRow(`SELECT method, environment FROM configs;`)
	if err := row.Scan(&config.Method, &config.Environment); err != nil {
		if err != sql.ErrNoRows {
			return err
		}
		initMethod := "post"
		initEnvironment := "test"
		if _, err := db.Exec(`INSERT INTO configs (method, environment) VALUES (?, ?);`, initMethod, initEnvironment); err != nil {
			return err
		}
		requestMethodConfig = initMethod
		requestEnvironmentConfig = initEnvironment
		return nil
	}
	requestMethodConfig = config.Method
	requestEnvironmentConfig = config.Environment
	return nil
}
