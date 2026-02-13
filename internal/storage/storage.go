package storage

import "errors"

var (
	ErrTaskNotFound = errors.New("task not found")
	ErrTaskExists   = errors.New("task already exists")
)
