package usage

import "time"

// Usage represents a user's plan consumption snapshot.
type Usage struct {
	Plan     string    `json:"plan"`
	Limit    int       `json:"limit"`
	Used     int       `json:"used"`
	ResetsAt time.Time `json:"resetsAt"`
}
