package main

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/securecookie"
)

type SessionManager struct {
	cookieName         string
	sessions           map[string]*Session
	secureCookie       *securecookie.SecureCookie
	taskVerdictChannel <-chan TaskVerdict
	testVerdictChannel <-chan CustomTestVerdict
	watcherStopChannel chan bool
	lock               sync.Mutex
}

type Session struct {
	sid          string
	password     []byte
	contest      string
	taskVerdicts []TaskVerdict
	testVerdicts []CustomTestVerdict
	lock         sync.Mutex
}

func NewSessionManager(taskVerdictChannel <-chan TaskVerdict, testVerdictChannel <-chan CustomTestVerdict, cookieName string) *SessionManager {
	var hashKey, blockKey []byte
	if testingFlag {
		hashKey = []byte("testing-key")
		blockKey = []byte("testing-blockkey")
	} else {
		hashKey = securecookie.GenerateRandomKey(32)
		hashKey = securecookie.GenerateRandomKey(32)
	}

	m := &SessionManager{
		cookieName:         cookieName,
		sessions:           make(map[string]*Session),
		taskVerdictChannel: taskVerdictChannel,
		testVerdictChannel: testVerdictChannel,
		secureCookie:       securecookie.New(hashKey, blockKey),
	}

	return m
}

func (m *SessionManager) StartWatcher() {
	if m.watcherStopChannel != nil {
		return
	}

	m.watcherStopChannel = make(chan bool)
	go func() {
		select {
		case <-m.watcherStopChannel:
			return
		case v := <-m.taskVerdictChannel:
			m.lock.Lock()
			if session, ok := m.sessions[v.SID]; ok {
				session.lock.Lock()
				session.taskVerdicts = append(session.taskVerdicts, v)
				session.lock.Unlock()
			}
			m.lock.Unlock()
		case v := <-m.testVerdictChannel:
			m.lock.Lock()
			if session, ok := m.sessions[v.SID]; ok {
				session.lock.Lock()
				session.testVerdicts = append(session.testVerdicts, v)
				session.lock.Unlock()
			}
			m.lock.Unlock()
		}
	}()
}

func (m *SessionManager) StopWatcher() {
	if m.watcherStopChannel == nil {
		return
	}

	m.watcherStopChannel <- true
}

func (m *SessionManager) getSessionID(r *http.Request) string {
	if cookie, err := r.Cookie(m.cookieName); err == nil {
		var sid string
		if err = m.secureCookie.Decode(m.cookieName, cookie.Value, &sid); err == nil {
			return sid
		}
	}

	return ""
}

func (m *SessionManager) setSessionID(w http.ResponseWriter, sid string) error {
	encoded, err := m.secureCookie.Encode(m.cookieName, sid)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
	})

	return nil
}

func (m *SessionManager) OpenSession(w http.ResponseWriter, r *http.Request) (*Session, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	sid := m.getSessionID(r)
	if len(sid) > 0 {
		if session, ok := m.sessions[sid]; ok {
			return session, nil
		}
	}

	if testingFlag {
		sid = "testing-" + strconv.Itoa(len(m.sessions))
	} else {
		sidBytes, err := generateKey(32)
		if err != nil {
			return nil, err
		}
		sid = string(sidBytes)
	}

	if err := m.setSessionID(w, sid); err != nil {
		return nil, err
	}

	session := &Session{
		sid: string(sid),
	}

	if testingFlag {
		password, err := ioutil.ReadFile("pass")
		if err == nil && len(password) > 0 {
			session.password = password
		}
		session.contest = "judge_test"
	}

	m.sessions[sid] = session

	return session, nil
}

func (m *SessionManager) DeleteSession(w http.ResponseWriter, r *http.Request) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if sid := m.getSessionID(r); len(sid) > 0 {
		delete(m.sessions, sid)
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

	return s.sid
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
