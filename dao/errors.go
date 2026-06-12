package dao

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrOwnProduct   = errors.New("cannot buy own product")
	ErrNotPurchased = errors.New("not purchased by you")
)
