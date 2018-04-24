package main

import (
	"encoding/json"
	"errors"
	"html"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	rice "github.com/GeertJohan/go.rice"
	"github.com/gorilla/mux"
	"github.com/nicksnyder/go-i18n/i18n"
	"golang.org/x/tools/godoc/vfs/httpfs"
)

const (
	localeCookieName = "obijudge-locale"
)

type Server struct {
	Port          int
	DB            *Database
	Reference     *Reference
	Judge         *Judge
	Logger        *log.Logger
	DefaultLocale string

	templates      *template.Template
	sessionManager *SessionManager
	server         *http.Server
}

func (srv *Server) Start() error {
	// setup session storage
	if testingFlag {
		srv.sessionManager = NewSessionManager(srv.Judge.TaskVerdictChannel, srv.Judge.TestVerdictChannel, "obijudge-testing")
	} else {
		randBytes, _ := generateKey(10)
		srv.sessionManager = NewSessionManager(srv.Judge.TaskVerdictChannel, srv.Judge.TestVerdictChannel, "obijudge-"+string(randBytes))
	}

	srv.sessionManager.StartWatcher()

	// setup templates
	srv.templates = template.New("")
	templateBox := rice.MustFindBox("templates/dist")
	if err := templateBox.Walk("", func(path string, _ os.FileInfo, _ error) error {
		if path == "" {
			return nil
		}

		templateString, err := templateBox.String(path)
		if err != nil {
			return err
		}

		_, err = srv.templates.New(path).Funcs(template.FuncMap{
			"T": i18n.IdentityTfunc(),
		}).Parse(templateString)
		return err
	}); err != nil {
		return err
	}

	// setup router
	r := mux.NewRouter()

	// default 404
	r.NotFoundHandler = http.HandlerFunc(srv.notFoundHandler)

	// setup reference
	r.PathPrefix("/ref/").Handler(http.StripPrefix("/ref/",
		http.FileServer(httpfs.New(srv.Reference.FileSystem))))

	// load static files and serve
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/",
		http.FileServer(rice.MustFindBox("static/dist").HTTPBox())))

	// string translation api
	r.HandleFunc("/translate", srv.translateHandler).Methods("GET")

	r.HandleFunc("/", srv.homeHandler).Methods("GET")
	r.HandleFunc("/login", srv.loginHandler).Methods("POST")
	r.HandleFunc("/logout", srv.logoutHandler).Methods("POST")

	r.Handle("/overview", srv.authWrapper(srv.overviewHandler)).Methods("GET")
	r.Handle("/task/{name}.pdf", srv.authWrapper(srv.pdfHandler)).Methods("GET")
	r.Handle("/task/{name}", srv.authWrapper(srv.taskHandler)).Methods("GET")
	r.Handle("/submit/{name}", srv.authWrapper(srv.submitHandler)).Methods("POST")
	r.Handle("/test/{name}", srv.authWrapper(srv.testHandler)).Methods("POST")

	r.Handle("/getsubmission", srv.authWrapper(srv.getSubmissionHandler)).Methods("GET")
	r.Handle("/gettest", srv.authWrapper(srv.getTestHandler)).Methods("GET")
	r.Handle("/gettasks", srv.authWrapper(srv.getTasksHandler)).Methods("GET")
	r.Handle("/gettasktitle", srv.authWrapper(srv.getTaskTitleHandler)).Methods("GET")
	r.Handle("/getcode", srv.authWrapper(srv.getCode)).Methods("GET")
	r.Handle("/setcode", srv.authWrapper(srv.setCode)).Methods("POST")

	// setup http.Server
	srv.server = &http.Server{
		Addr:    ":" + strconv.Itoa(srv.Port),
		Handler: srv.loggingWrapper(srv.localeWrapper(r)),
	}

	// run server
	go func() {
		if err := srv.server.ListenAndServe(); err != nil {
			srv.Logger.Print(err)
		}
	}()

	return nil
}

func (srv *Server) Stop() {
	if err := srv.server.Shutdown(nil); err != nil {
		panic(err)
	}

	srv.sessionManager.StopWatcher()
}

func (srv *Server) getLang(r *http.Request) (i18n.TranslateFunc, error) {
	newLocale := r.FormValue("locale")

	var pastLocale string
	localeCookie, err := r.Cookie(localeCookieName)
	if err == nil {
		pastLocale = localeCookie.Value
	}

	acceptLocale := r.Header.Get("Accept-Language")

	return i18n.Tfunc(newLocale, pastLocale, acceptLocale, srv.DefaultLocale)
}

// template renderer
func (srv *Server) render(w http.ResponseWriter, r *http.Request,
	template string, data map[string]interface{}, status int) {
	T, err := srv.getLang(r)
	if err != nil {
		srv.Logger.Print(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	w.WriteHeader(status)

	data["T"] = T
	if err := srv.templates.Funcs(map[string]interface{}{"T": T}).ExecuteTemplate(w, template, data); err != nil {
		srv.Logger.Print(err)
	}
}

// 404 page handler
func (srv *Server) notFoundHandler(w http.ResponseWriter, r *http.Request) {
	srv.render(w, r, "404.html", map[string]interface{}{}, http.StatusNotFound)
}

// 500 page handler
func (srv *Server) errorHandler(err error, w http.ResponseWriter, r *http.Request) {
	srv.Logger.Print(err)

	srv.render(w, r, "500.html", map[string]interface{}{
		"Error": err.Error(),
	}, http.StatusInternalServerError)
}

// login handler
func (srv *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	s, err := srv.sessionManager.OpenSession(w, r)
	if err != nil {
		srv.errorHandler(err, w, r)
		return
	}

	err = r.ParseForm()
	if err != nil {
		srv.errorHandler(err, w, r)
		return
	}

	password := r.Form.Get("password")
	contest := r.Form.Get("contest")

	if testingFlag {
		pass, err := ioutil.ReadFile("pass")
		if err == nil && len(password) > 0 {
			password = string(pass)
		}
	}

	if srv.DB.Authenticate([]byte(password)) {
		s.SetPassword([]byte(password))
		s.SetContest(contest)
		http.Redirect(w, r, "/", http.StatusFound)
	} else {
		http.Redirect(w, r, "/?wrong=true", http.StatusFound)
	}
}

// logout handler
func (srv *Server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	srv.sessionManager.DeleteSession(w, r)
	http.Redirect(w, r, "/", http.StatusFound)
}

// translateHandler: translates a string
func (srv *Server) translateHandler(w http.ResponseWriter, r *http.Request) {
	T, err := srv.getLang(r)
	if err != nil {
		srv.Logger.Print(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	w.Write([]byte(html.EscapeString(T(r.FormValue("str")))))
}

func (srv *Server) homeHandler(w http.ResponseWriter, r *http.Request) {
	s, err := srv.sessionManager.OpenSession(w, r)
	if err != nil {
		srv.errorHandler(err, w, r)
		return
	}

	if len(s.GetPassword()) == 0 {
		contests, err := srv.DB.Contests()
		if err != nil {
			srv.errorHandler(err, w, r)
			return
		}

		wrongpassword := r.FormValue("wrong")

		srv.render(w, r, "home.html", map[string]interface{}{
			"PageID":        "_home",
			"Title":         "OBIJudge",
			"Contests":      contests,
			"WrongPassword": wrongpassword,
		}, http.StatusOK)
	} else {
		http.Redirect(w, r, "/overview", http.StatusFound)
	}
}

func (srv *Server) overviewHandler(s *Session, w http.ResponseWriter, r *http.Request) {
	contest, err := srv.DB.Contest(s.GetContest())
	if err != nil {
		srv.errorHandler(err, w, r)
		return
	}

	tasks, err := srv.DB.ContestTasks(s.GetContest())
	if err != nil {
		srv.errorHandler(err, w, r)
		return
	}

	srv.render(w, r, "overview.html", map[string]interface{}{
		"PageID": "_overview",
		"Title":  contest.Title,
		"Tasks":  tasks,
		"Refs":   srv.Reference.Data,
	}, http.StatusOK)
}

func (srv *Server) taskHandler(s *Session, w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	task, err := srv.DB.Task(name)
	if err != nil {
		srv.errorHandler(err, w, r)
		return
	}

	tasks, err := srv.DB.ContestTasks(s.GetContest())
	if err != nil {
		srv.errorHandler(err, w, r)
		return
	}

	statement, err := srv.DB.Statement(name, s.GetPassword())
	if err != nil {
		srv.errorHandler(err, w, r)
		return
	}

	srv.render(w, r, "task.html", map[string]interface{}{
		"PageID":        task.Name,
		"Title":         task.Title,
		"Tasks":         tasks,
		"Refs":          srv.Reference.Data,
		"Task":          task,
		"HasPDF":        len(statement.PDF) > 0,
		"HasHTML":       len(statement.HTML) > 0,
		"HTMLStatement": template.HTML(string(statement.HTML)),
		"Langs":         Languages,
	}, http.StatusOK)
}

func (srv *Server) submitHandler(s *Session, w http.ResponseWriter, r *http.Request) {
	type result struct {
		Error string
		ID    uint32
	}

	vars := mux.Vars(r)
	name := vars["name"]

	encoder := json.NewEncoder(w)

	task, err := srv.DB.Task(name)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		encoder.Encode(result{err.Error(), 0})
		return
	}

	err = r.ParseMultipartForm(32 << 20)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		encoder.Encode(result{err.Error(), 0})
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		encoder.Encode(result{err.Error(), 0})
		return
	}
	defer file.Close()

	code, err := ioutil.ReadAll(file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		encoder.Encode(result{err.Error(), 0})
		return
	}

	if len(code) == 0 {
		code = []byte(r.Form.Get("code"))
	}

	if len(code) > (1 << 20) { // 1MB
		w.WriteHeader(http.StatusBadRequest)
		encoder.Encode(result{"Code length (" + strconv.Itoa(len(code)) + ") exceeds 1MB!", 0})
		return
	}

	langIndex, err := strconv.Atoi(r.Form.Get("lang"))
	if err != nil || langIndex > len(Languages) {
		w.WriteHeader(http.StatusInternalServerError)
		encoder.Encode(result{"Language " + r.Form.Get("lang") + " doesn't have a runner!", 0})
		return
	}

	lang := Languages[langIndex]

	subID := srv.Judge.SendSubmission(Submission{
		SID:  s.GetID(),
		When: time.Now(),
		Task: &task,
		Code: code,
		Lang: lang,
		Key:  s.GetPassword(),
	})
	encoder.Encode(result{"", subID})
}

func (srv *Server) testHandler(s *Session, w http.ResponseWriter, r *http.Request) {
	type result struct {
		Error string
		ID    uint32
	}

	vars := mux.Vars(r)
	name := vars["name"]

	encoder := json.NewEncoder(w)

	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		encoder.Encode(result{err.Error(), 0})
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		encoder.Encode(result{err.Error(), 0})
		return
	}
	defer file.Close()

	code, err := ioutil.ReadAll(file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		encoder.Encode(result{err.Error(), 0})
		return
	}

	if len(code) == 0 {
		code = []byte(r.Form.Get("code"))
	}

	if len(code) > (1 << 20) { // 1MB
		w.WriteHeader(http.StatusBadRequest)
		encoder.Encode(result{"Code length (" + strconv.Itoa(len(code)) + ") exceeds 1MB!", 0})
		return
	}

	langIndex, err := strconv.Atoi(r.Form.Get("lang"))
	if err != nil || langIndex > len(Languages) {
		w.WriteHeader(http.StatusInternalServerError)
		encoder.Encode(result{"Language " + r.Form.Get("lang") + " doesn't have a runner!", 0})
		return
	}

	lang := Languages[langIndex]

	input := []byte(r.Form.Get("input"))

	testID := srv.Judge.SendCustomTest(CustomTest{
		SID:      s.GetID(),
		When:     time.Now(),
		TaskName: name,
		Input:    input,
		Code:     code,
		Lang:     lang,
	})
	encoder.Encode(result{"", testID})
}

func (srv *Server) pdfHandler(s *Session, w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	statement, err := srv.DB.Statement(name, s.GetPassword())
	if err != nil {
		srv.errorHandler(err, w, r)
		return
	}

	if len(statement.PDF) > 0 {
		w.Write(statement.PDF)
	} else {
		err = errors.New("No PDF problem statement for " + name + ".")
		srv.errorHandler(err, w, r)
	}
}

func (srv *Server) getSubmissionHandler(s *Session, w http.ResponseWriter, r *http.Request) {
	encoder := json.NewEncoder(w)

	var subs []TaskVerdict

	task := r.FormValue("task")
	if len(task) > 0 {
		subs = s.GetTaskSubmissions(task)
	} else {
		id, err := strconv.Atoi(r.FormValue("id"))
		if err != nil {
			subs = s.GetSubmissions()
		} else {
			subs = s.GetSubmission(id)
		}
	}

	encoder.Encode(subs)
}

func (srv *Server) getTestHandler(s *Session, w http.ResponseWriter, r *http.Request) {
	encoder := json.NewEncoder(w)

	id, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		encoder.Encode(s.GetTests())
	} else {
		encoder.Encode(s.GetTest(id))
	}
}

func (srv *Server) getTasksHandler(s *Session, w http.ResponseWriter, r *http.Request) {
	encoder := json.NewEncoder(w)

	tasks, err := srv.DB.ContestTasks(s.GetContest())
	if err != nil {
		srv.errorHandler(err, w, r)
		return
	}

	encoder.Encode(tasks)
}

func (srv *Server) getTaskTitleHandler(s *Session, w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")

	task, err := srv.DB.Task(name)
	if err == nil {
		w.Write([]byte(task.Title))
	}
}

func (srv *Server) getCode(s *Session, w http.ResponseWriter, r *http.Request) {
	encoder := json.NewEncoder(w)
	task := r.FormValue("task")
	encoder.Encode(s.GetCode(task))
}

func (srv *Server) setCode(s *Session, w http.ResponseWriter, r *http.Request) {
	task := r.FormValue("task")
	code := r.FormValue("code")
	lang, _ := strconv.Atoi(r.FormValue("lang"))
	s.SetCode(task, CodeInfo{Code: code, Lang: lang})
}

func (srv *Server) localeWrapper(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		newLocale := r.FormValue("locale")
		if newLocale != "" {
			http.SetCookie(w, &http.Cookie{
				Name:  localeCookieName,
				Value: newLocale,
				Path:  "/",
			})
		}

		next.ServeHTTP(w, r)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.status = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	size, err := lrw.ResponseWriter.Write(b)
	lrw.size += size
	return size, err
}

func (srv *Server) loggingWrapper(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lrw := &loggingResponseWriter{w, http.StatusOK, 0}

		defer func() {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}

			uri := r.RequestURI
			if uri == "" {
				uri = r.URL.RequestURI()
			}

			srv.Logger.Println(host, "--", r.Method, uri, r.Proto, "--",
				lrw.status, http.StatusText(lrw.status), lrw.size, "bytes")
		}()

		next.ServeHTTP(lrw, r)
	})
}

func (srv *Server) authWrapper(f func(*Session, http.ResponseWriter, *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, err := srv.sessionManager.OpenSession(w, r)
		if err != nil {
			srv.errorHandler(err, w, r)
			return
		}

		if len(s.GetPassword()) == 0 {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		f(s, w, r)
	})
}
