package main

import (
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/securecookie"
)

// SessionManager stores information related to a single instance of the
// session cookies manager used by the whole program.
type SessionManager struct {
	cookieName         string
	sessions           map[string]*Session
	secureCookie       *securecookie.SecureCookie
	taskVerdictChannel <-chan TaskVerdict
	testVerdictChannel <-chan CustomTestVerdict
	watcherStopChannel chan bool
	lock               sync.Mutex
}

// Session stores information related to a single user session.
type Session struct {
	sid          string
	password     []byte
	database     *Database
	taskVerdicts []TaskVerdict
	testVerdicts []CustomTestVerdict
	codes        map[string]CodeInfo
	lock         sync.Mutex
}

// CodeInfo is used to share information between Go and Javascript.
type CodeInfo struct {
	Code string
	Lang int
}

// NewSessionManager acts as a constructor and initializes a new session manager.
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

type taskVerdictsByID []TaskVerdict
type testVerdictsByID []CustomTestVerdict

func (v taskVerdictsByID) Len() int           { return len(v) }
func (v testVerdictsByID) Len() int           { return len(v) }
func (v taskVerdictsByID) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v testVerdictsByID) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v taskVerdictsByID) Less(i, j int) bool { return v[i].ID < v[j].ID }
func (v testVerdictsByID) Less(i, j int) bool { return v[i].ID < v[j].ID }

func (m *SessionManager) StartWatcher() {
	if m.watcherStopChannel != nil {
		return
	}

	m.watcherStopChannel = make(chan bool)
	go func() {
		for {
			select {
			case <-m.watcherStopChannel:
				return
			case v := <-m.taskVerdictChannel:
				m.lock.Lock()
				if session, ok := m.sessions[v.SID]; ok {
					session.lock.Lock()
					session.taskVerdicts = append(session.taskVerdicts, v)
					session.lock.Unlock()
					sort.Sort(taskVerdictsByID(session.taskVerdicts))
				}
				m.lock.Unlock()
			case v := <-m.testVerdictChannel:
				m.lock.Lock()
				if session, ok := m.sessions[v.SID]; ok {
					session.lock.Lock()
					session.testVerdicts = append(session.testVerdicts, v)
					session.lock.Unlock()
					sort.Sort(testVerdictsByID(session.testVerdicts))
				}
				m.lock.Unlock()
			}
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
		sid:   string(sid),
		codes: make(map[string]CodeInfo),
	}

	m.sessions[sid] = session

	return session, nil
}

func (m *SessionManager) DeleteSession(w http.ResponseWriter, r *http.Request) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if sid := m.getSessionID(r); len(sid) > 0 {
		if session, ok := m.sessions[sid]; ok {
			session.GetDatabase().Clear()
		}

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

func (s *Session) GetDatabase() *Database {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.database
}

func (s *Session) SetDatabase(database *Database) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.database = database
}

func (s *Session) GetTaskSubmissions(taskName string) []TaskVerdict {
	s.lock.Lock()
	defer s.lock.Unlock()

	ret := make([]TaskVerdict, 0)
	for _, v := range s.taskVerdicts {
		if v.TaskName == taskName {
			ret = append(ret, v)
		}
	}
	return ret
}

func (s *Session) GetSubmissions() []TaskVerdict {
	s.lock.Lock()
	defer s.lock.Unlock()

	ret := make([]TaskVerdict, len(s.taskVerdicts))
	copy(ret, s.taskVerdicts)
	return ret
}

func (s *Session) GetSubmission(id int) []TaskVerdict {
	s.lock.Lock()
	defer s.lock.Unlock()

	ret := make([]TaskVerdict, 0)
	for _, v := range s.taskVerdicts {
		if int(v.ID) == id {
			ret = append(ret, v)
		}
	}
	return ret
}

func (s *Session) GetTests() []CustomTestVerdict {
	s.lock.Lock()
	defer s.lock.Unlock()

	ret := make([]CustomTestVerdict, len(s.testVerdicts))
	copy(ret, s.testVerdicts)
	return ret
}

func (s *Session) GetTest(id int) []CustomTestVerdict {
	s.lock.Lock()
	defer s.lock.Unlock()

	ret := make([]CustomTestVerdict, 0)
	for _, v := range s.testVerdicts {
		if int(v.ID) == id {
			ret = append(ret, v)
		}
	}
	return ret
}

func (s *Session) SetCode(task string, code CodeInfo) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.codes[task] = code
}

func (s *Session) GetCode(task string) CodeInfo {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.codes[task]
}
