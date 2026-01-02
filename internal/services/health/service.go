package health

// Service encapsulates health-related checks.
type Service struct{}

// NewService constructs a new health service.
func NewService() *Service {
	return &Service{}
}

// Status returns a simple health payload.
func (s *Service) Status() map[string]bool {
	return map[string]bool{"ok": true}
}
