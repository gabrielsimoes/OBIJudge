package main

import (
	"encoding/gob"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	rice "github.com/GeertJohan/go.rice"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"golang.org/x/tools/godoc/vfs"
	"golang.org/x/tools/godoc/vfs/httpfs"
)

// keeps everything handlers need to access in a session
type context struct {
	db          *database
	templates   *template.Template
	reference   []ReferenceData
	referenceFS *vfs.FileSystem
	store       *sessions.CookieStore
	sessionid   string
	log         *log.Logger
}

// keeps last submissions' scores
type scoremap map[string]Verdict

// add scoremap to be used in gorilla/sessions
func init() {
	gob.Register(scoremap{})
}

// template renderer
func (c *context) render(w http.ResponseWriter, tmpl string, data interface{}) {
	err := c.templates.ExecuteTemplate(w, tmpl, data)
	if err != nil {
		c.log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// 404 page handler
func (c *context) notFoundHandler(w http.ResponseWriter, r *http.Request) {
	c.render(w, "404.html", nil)
}

// login handler
func (c *context) loginHandler(w http.ResponseWriter, r *http.Request) {
	session, err := c.store.Get(r, c.sessionid)
	if err != nil {
		c.log.Printf(err.Error())
	}

	err = r.ParseForm()
	if err != nil {
		c.log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	password := r.Form.Get("password")
	contest := r.Form.Get("contest")

	if c.db.authenticate([]byte(password)) {
		session.Values["password"] = password
		session.Values["authenticated"] = true
		session.Values["contest"] = contest
		if err := session.Save(r, w); err != nil {
			c.log.Printf(err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
	} else {
		http.Redirect(w, r, "/?wrong=true", http.StatusFound)
	}
}

// logout handler
func (c *context) logoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:    c.sessionid,
		Value:   "",
		Path:    "/",
		Expires: time.Unix(0, 0),
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (c *context) homeHandler(w http.ResponseWriter, r *http.Request) {
	session, err := c.store.Get(r, c.sessionid)
	if err != nil {
		c.log.Printf(err.Error())
	}

	if auth, ok := session.Values["authenticated"]; !ok || !auth.(bool) {
		contests, err := c.db.getContests()
		if err != nil {
			c.log.Printf(err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		wrongpassword := r.FormValue("wrong")

		c.render(w, "home.html", map[string]interface{}{
			"PageID":        "_home",
			"Title":         "OBIJudge",
			"Contests":      contests,
			"WrongPassword": wrongpassword,
		})
	} else {
		http.Redirect(w, r, "/overview", http.StatusFound)
	}
}

func (c *context) overviewHandler(session *sessions.Session, w http.ResponseWriter, r *http.Request) {
	contest, err := c.db.getContest(session.Values["contest"].(string))
	if err != nil {
		c.log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tasks, err := c.db.getContestTasks(session.Values["contest"].(string))
	if err != nil {
		c.log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	scores, ok := session.Values["scores"].(scoremap)
	if !ok {
		scores = make(scoremap)
		for _, task := range tasks {
			scores[task.Name] = Verdict{}
		}
		session.Values["scores"] = scores
		if err := session.Save(r, w); err != nil {
			c.log.Printf(err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	c.render(w, "overview.html", map[string]interface{}{
		"PageID": "_overview",
		"Title":  contest.Title,
		"Tasks":  tasks,
		"Scores": scores,
		"Refs":   c.reference,
	})
}

func (c *context) taskHandler(session *sessions.Session, w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	task, err := c.db.getTask(name)
	if err != nil {
		c.log.Printf(err.Error())
		c.notFoundHandler(w, r)
		return
	}

	tasks, err := c.db.getContestTasks(session.Values["contest"].(string))
	if err != nil {
		c.log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	statement, err := c.db.getStatement(name, []byte(session.Values["password"].(string)))
	if err != nil {
		c.log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	c.render(w, "task.html", map[string]interface{}{
		"PageID":        task.Name,
		"Title":         task.Title,
		"Tasks":         tasks,
		"Refs":          c.reference,
		"Task":          task,
		"HasPDF":        len(statement.PDF) > 0,
		"HasHTML":       len(statement.HTML) > 0,
		"HTMLStatement": template.HTML(string(statement.HTML)),
	})
}

func (c *context) submitHandler(session *sessions.Session, w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	task, err := c.db.getTask(name)
	if err != nil {
		c.log.Printf(err.Error())
		c.notFoundHandler(w, r)
		return
	}

	err = r.ParseMultipartForm(32 << 20)
	if err != nil {
		c.log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		c.log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	code, err := ioutil.ReadAll(file)
	if err != nil {
		c.log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(code) == 0 {
		code = []byte(r.Form.Get("code"))
	}
	lang := r.Form.Get("lang")

	result, err := judge(&task, c.db, []byte(session.Values["password"].(string)), code, lang)
	if err != nil {
		c.log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	session.Values["scores"].(scoremap)[name] = result

	if err := session.Save(r, w); err != nil {
		c.log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

func (c *context) pdfHandler(session *sessions.Session, w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	statement, err := c.db.getStatement(name, []byte(session.Values["password"].(string)))
	if err != nil {
		c.log.Printf(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(statement.PDF) > 0 {
		w.Header().Set("Content-Disposition", "attachment; filename="+name+".pdf")
		w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
		w.Write(statement.PDF)
	} else {
		c.log.Printf("No problem statement for \"%s\".", name)
		c.notFoundHandler(w, r)
	}
}

func (c *context) authWrapper(f func(*sessions.Session, http.ResponseWriter, *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := c.store.Get(r, c.sessionid)
		if err != nil {
			c.log.Printf(err.Error())
		}

		if auth, ok := session.Values["authenticated"]; !ok || !auth.(bool) {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		f(session, w, r)
	})
}

func runServer(databaseLocation, referenceLocation string, port uint) error {
	var err error
	ctx := &context{}

	// setup session storage
	ctx.store = sessions.NewCookieStore([]byte("u46IpCV9y5ai9r8YvODJEhgOY8a9JVE4"))
	// ctx.sessionid = "obijudge" + strconv.Itoa(rand.Intn(1000))
	ctx.sessionid = "testing"

	// setup logging
	ctx.log = log.New(os.Stderr, "[OBIJUDGE] ", 2)

	// setup database
	ctx.db, err = openDatabase(databaseLocation)
	if err != nil {
		return err
	}
	defer ctx.db.close()

	// setup templates
	ctx.templates = template.New("")
	templateBox := rice.MustFindBox("templates")
	templateBox.Walk("", func(path string, _ os.FileInfo, _ error) error {
		if path == "" {
			return nil
		}

		templateString, err := templateBox.String(path)
		if err != nil {
			fmt.Errorf("Unable to parse: path=%s, err=%s", path, err)
		}

		ctx.templates.New(path).Parse(templateString)
		return nil
	})

	// setup static pages for language reference
	ctx.reference, ctx.referenceFS, err = initReference(referenceLocation)
	if err != nil {
		return err
	}

	// setup router
	r := mux.NewRouter()

	// setup reference
	r.PathPrefix("/ref/").Handler(http.StripPrefix("/ref/",
		http.FileServer(httpfs.New(*ctx.referenceFS))))

	// load static files and serve
	staticBox := rice.MustFindBox("static").HTTPBox()
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/",
		http.FileServer(staticBox)))

	// default 404
	r.NotFoundHandler = http.HandlerFunc(ctx.notFoundHandler)

	r.HandleFunc("/", ctx.homeHandler).Methods("GET")
	r.HandleFunc("/login", ctx.loginHandler).Methods("POST")
	r.HandleFunc("/logout", ctx.logoutHandler).Methods("POST")

	r.Handle("/overview", ctx.authWrapper(ctx.overviewHandler)).Methods("GET")
	r.Handle("/task/{name}.pdf", ctx.authWrapper(ctx.pdfHandler)).Methods("GET")
	r.Handle("/task/{name}", ctx.authWrapper(ctx.taskHandler)).Methods("GET")
	r.Handle("/task/{name}", ctx.authWrapper(ctx.submitHandler)).Methods("POST")

	return http.ListenAndServe("0.0.0.0:"+strconv.Itoa(int(port)), handlers.LoggingHandler(os.Stderr, r))
}
