package workspace

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("workspace item not found")

type Labels struct {
	CompanyName string `json:"companyName"`
	TargetRole  string `json:"targetRole"`
}

// Application contains owner-facing context, never report bodies or private answers.
type Application struct {
	AnalysisID     string    `json:"analysisId"`
	DocumentID     string    `json:"documentId"`
	ResumeFileName string    `json:"resumeFileName"`
	JobExcerpt     string    `json:"jobExcerpt"`
	Mode           string    `json:"mode"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
	Labels
}

type Store interface {
	Primary(context.Context, string) (string, error)
	SetPrimary(context.Context, string, string) error
	GetLabels(context.Context, string) (Labels, error)
	SaveLabels(context.Context, string, Labels) error
	List(context.Context, string, string, int, int) ([]Application, error)
}

type MemoryStore struct {
	mu      sync.RWMutex
	primary map[string]string
	labels  map[string]Labels
	// Sources is supplied by bootstrap from the authoritative owner-scoped repositories.
	Sources func(context.Context, string) ([]Application, error)
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{primary: map[string]string{}, labels: map[string]Labels{}}
}
func (s *MemoryStore) Primary(ctx context.Context, user string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.primary[user], nil
}
func (s *MemoryStore) SetPrimary(ctx context.Context, user, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.primary[user] = id
	return nil
}
func (s *MemoryStore) GetLabels(ctx context.Context, id string) (Labels, error) {
	if err := ctx.Err(); err != nil {
		return Labels{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.labels[id], nil
}
func (s *MemoryStore) SaveLabels(ctx context.Context, id string, l Labels) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.labels[id] = l
	return nil
}
func (s *MemoryStore) List(ctx context.Context, user, q string, limit, offset int) ([]Application, error) {
	apps, err := s.Sources(ctx, user)
	if err != nil {
		return nil, err
	}
	out := []Application{}
	q = strings.ToLower(q)
	s.mu.RLock()
	for _, a := range apps {
		a.Labels = s.labels[a.AnalysisID]
		if q == "" || strings.Contains(strings.ToLower(a.CompanyName+" "+a.TargetRole+" "+a.ResumeFileName+" "+a.JobExcerpt), q) {
			out = append(out, a)
		}
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].AnalysisID > out[j].AnalysisID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if offset >= len(out) {
		return []Application{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}
