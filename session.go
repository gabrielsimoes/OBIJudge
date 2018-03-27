package main

import (
	"net/http"
	"sync"
	"time"
)

type SessionManager struct {
	lock           sync.Mutex
	cookieName     string
	sessions       map[string]*Session
	verdictChannel <-chan TaskVerdict
}

type Session struct {
	id       string
	password []byte
	contest  string

	lock sync.Mutex
}

func NewSessionManager(verdictChannel <-chan TaskVerdict, cookieName string) *SessionManager {
	m := &SessionManager{
		cookieName:     cookieName,
		sessions:       make(map[string]*Session),
		verdictChannel: verdictChannel,
	}

	// TODO run watcher

	return m
}

func (m *SessionManager) OpenSession(w http.ResponseWriter, r *http.Request) (*Session, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	cookie, err := r.Cookie(m.cookieName)
	if err == nil && cookie.Value != "" {
		if session, ok := m.sessions[cookie.Value]; ok {
			return session, nil
		}
	}

	sid, err := generateKey(32)
	if err != nil {
		return nil, err
	}

	session := &Session{
		id: string(sid),
	}

	m.sessions[string(sid)] = session

	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    string(sid),
		Path:     "/",
		HttpOnly: true,
		MaxAge:   0,
	})

	return session, nil
}

func (m *SessionManager) DeleteSession(w http.ResponseWriter, r *http.Request) {
	m.lock.Lock()
	defer m.lock.Unlock()

	cookie, err := r.Cookie(m.cookieName)
	if err == nil && cookie.Value != "" {
		delete(m.sessions, cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:    m.cookieName,
		Value:   "",
		Path:    "/",
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
	})
}

func (s *Session) GetID() string {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.id
}

func (s *Session) GetPassword() []byte {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.password
}

func (s *Session) SetPassword(password []byte) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.password = password
}

func (s *Session) GetContest() string {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.contest
}

func (s *Session) SetContest(contest string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.contest = contest
}
