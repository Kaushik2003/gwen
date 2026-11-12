package remote

import "context"

// A phone's open connections end the moment it may no longer use Gwen
// (plan.md 3.14.4): when it is unpaired, and when the remote is turned off.
// A terminal view is a WebSocket, which neither a refused request nor
// closing the HTTP server ever reaches, so each view and event stream runs
// under a context that ends then.

// PhoneContext is ctx, ended too when device is unpaired or EndPhones runs.
// The caller calls cancel when its connection is done. Device "" is the
// laptop, which is never cut off.
func (s *Service) PhoneContext(ctx context.Context, device string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	if device == "" {
		return ctx, cancel
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended[device] {
		// Unpaired between the request's check and now.
		cancel()
		return ctx, cancel
	}
	n := s.phoneN
	s.phoneN++
	if s.phones[device] == nil {
		s.phones[device] = map[int]context.CancelFunc{}
	}
	s.phones[device][n] = cancel
	return ctx, func() {
		cancel()
		s.mu.Lock()
		delete(s.phones[device], n)
		if len(s.phones[device]) == 0 {
			delete(s.phones, device)
		}
		s.mu.Unlock()
	}
}

// unpaired ends device's open connections and refuses it any more.
func (s *Service) unpaired(device string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended[device] = true
	for _, cancel := range s.phones[device] {
		cancel()
	}
	delete(s.phones, device)
	delete(s.views, device)
}

// EndPhones ends every phone's open connections, as when the remote is
// turned off. A phone still paired may connect again once it is back on.
func (s *Service) EndPhones() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for device, conns := range s.phones {
		for _, cancel := range conns {
			cancel()
		}
		delete(s.phones, device)
	}
}
