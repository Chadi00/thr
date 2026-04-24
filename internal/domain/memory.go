package domain

import "time"

type Memory struct {
	ID        int64
	Text      string
	CreatedAt time.Time
	UpdatedAt time.Time
}
