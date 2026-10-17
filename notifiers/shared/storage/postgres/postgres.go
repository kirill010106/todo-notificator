package postgres

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

type Storage struct {
	DB *sql.DB
}

func New(DBUrl string) (*Storage, error) {
	const op = "storage.postgres.New"

	db, err := sql.Open("postgres", DBUrl)
	if err != nil {
		return nil, fmt.Errorf("%s:%w", op, err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("%s ping: %w", op, err)
	}

	return &Storage{
		DB: db,
	}, nil
}

func (s *Storage) Close() error {
	return s.DB.Close()
}
