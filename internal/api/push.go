package api

import (
	"net/http"

	"github.com/kzark/gwen/internal/remote"
)

// Phone notifications (Web Push): a paired phone turns them on in the phone
// app, which subscribes in its service worker and hands the subscription to
// the laptop. Only on the remote socket: these are a phone's own.
func (s *Server) pushRoutes(route router) {
	route("GET /v1/code/push", s.withCode(s.pushState))
	route("PUT /v1/code/push", s.withCode(s.pushSubscribe))
	route("DELETE /v1/code/push", s.withCode(s.pushUnsubscribe))
	route("POST /v1/code/push/test", s.withCode(s.pushTest))
}

func (s *Server) pushState(w http.ResponseWriter, r *http.Request) error {
	st, err := s.Code.PushState(deviceOf(r))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, st)
	return nil
}

func (s *Server) pushSubscribe(w http.ResponseWriter, r *http.Request) error {
	var sub remote.PushSubscription
	if err := decode(r, &sub); err != nil {
		return err
	}
	if err := s.Code.Subscribe(deviceOf(r), sub); err != nil {
		return err
	}
	return s.pushState(w, r)
}

func (s *Server) pushUnsubscribe(w http.ResponseWriter, r *http.Request) error {
	if err := s.Code.Unsubscribe(deviceOf(r)); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) pushTest(w http.ResponseWriter, r *http.Request) error {
	if err := s.Code.TestPush(r.Context(), deviceOf(r)); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
