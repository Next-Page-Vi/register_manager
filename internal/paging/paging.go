// Package paging defines the shared cursor-page contract. Cursor contents
// remain private to each service.
package paging

import "register_manager/internal/apperr"

type Input struct {
	Cursor *string
	Limit  *int
}

type Page[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

func Limit(value *int, fallback int) (int, error) {
	if value == nil {
		return fallback, nil
	}
	if *value < 1 || *value > 200 {
		return 0, apperr.Invalid("validation_failed").With("field", "limit")
	}
	return *value, nil
}

// New takes at most limit+1 rows. Only the extra row signals continuation.
func New[T any](items []T, limit int, cursor func(T) string) Page[T] {
	p := Page[T]{Items: items}
	if len(items) > limit {
		p.Items = items[:limit]
		value := cursor(items[limit-1])
		p.NextCursor = &value
	}
	if p.Items == nil {
		p.Items = []T{}
	}
	return p
}
