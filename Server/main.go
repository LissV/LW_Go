package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gocraft/web"
	"github.com/golang/glog"
	_ "github.com/lib/pq"
)

const (
	// указать собственные значения
	username         = "your_name"
	password         = "your_password"
	dbName           = "db_name"
	connectionString = "postgres://" + username + ":" + password + "@127.0.0.1:5432/" + dbName + "?sslmode=disable"
)

type сontext struct {
	err error
}

type err struct {
	code int    `json:"code"`
	text string `json:"text"`
}

type serverResponse struct {
	currentError *err  `json:"currentError,omitempty"`
	data         *docs `json:"data,omitempty"`
}

type doc struct {
	id      string `json:"id"`
	docname string `json:"docname"`
	mime    string `json:"mime"`
	public  bool   `json:"public"`
	created string `json:"created"`
}

type docs struct {
	list []*doc `json:"list,omitempty"`
}

var (
	db    *sql.DB
	lock  sync.RWMutex
	cache = make(map[string]string)
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Set("v", "2")
	flag.Parse()

	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		glog.Fatal(err)
	}

	err = db.Ping()
	if err != nil {
		glog.Fatal(err)
	}

	router := web.New(сontext{})
	router.Middleware((*сontext).handleErrors)
	router.Middleware((*сontext).Log)
	router.Get("/docs/", (*сontext).getAllDocuments)
	router.Get("/docs/:id", (*сontext).getCertainDocument)

	glog.Infof("Listening port 3000")
	err = http.ListenAndServe(":3000", nil)
	if err != nil {
		glog.Fatal(err)
	}
}

func (c *сontext) handleErrors(rw web.ResponseWriter, req *web.Request, next web.NextMiddlewareFunc) {
	next(rw, req)

	if c.err != nil {
		glog.Infof("Ошибка: %+v", c.err)
		errCode := rw.StatusCode()
		errText := c.err.Error()
		resp := &serverResponse{
			currentError: &err{
				code: errCode,
				text: errText,
			},
		}
		c.showResponse(rw, req, resp)
		return
	}
}

func (c *сontext) getAllDocuments(rw web.ResponseWriter, req *web.Request) {
	docs := c.selectAllDocuments()
	if c.err != nil {
		return
	}

	resp := &serverResponse{
		data: docs,
	}

	c.showResponse(rw, req, resp)
	return
}

func (c *сontext) getCertainDocument(rw web.ResponseWriter, req *web.Request) {
	var container string

	id := req.PathParams["id"]

	args := req.URL.Query()
	if args.Get("force") != "1" {
		container = c.findDocInCash(id)
	}

	if container == "" {
		glog.Info("cache miss")
		container = c.readDocument(id)
		if c.err != nil {
			return
		}
		c.writeDocInCash(id, container)
	}

	if container == "" {
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	io.WriteString(rw, container)
}

func (c *сontext) findDocInCash(id string) string {
	lock.RLock()
	defer lock.RUnlock()
	return cache[id]
}

func (c *сontext) writeDocInCash(id, text string) {
	lock.Lock()
	defer lock.Unlock()
	cache[id] = text
}

func (c *сontext) readDocument(id string) string {
	var err error
	var intID int
	var data []byte

	intID, err = strconv.Atoi(id)
	if err != nil {
		c.err = err
		return ""
	}

	doc := c.selectDocument(intID)
	if c.err != nil {
		return ""
	}
	if doc == nil {
		return ""
	}

	data, err = json.Marshal(doc)
	if err != nil {
		c.err = err
		return ""
	}

	return string(data)
}

func (c *сontext) selectAllDocuments() (selectedDocs *docs) {

	selectedDocs.list = make([]*doc, 0, 100)

	data, err := db.Query("SELECT id, docname, mime, public, created FROM docs;")
	if err != nil {
		c.err = err
		return
	}
	defer data.Close()

	for data.Next() {

		doc := new(doc)
		err = data.Scan(&doc.id, &doc.docname, &doc.mime, &doc.public, &doc.created)
		if err != nil {
			c.err = err
			return
		}
		selectedDocs.list = append(selectedDocs.list, doc)
	}

	err = data.Err()
	if err != nil {
		c.err = err
		return
	}
	return
}

func (c *сontext) selectDocument(id int) (selectedDocument *doc) {
	data, err := db.Query("SELECT id, docname, mime, public, created FROM docs WHERE id = $1;", id)
	if err != nil {
		c.err = err
		return
	}
	defer data.Close()

	if data.Next() {

		doc := new(doc)
		err = data.Scan(&doc.id, &doc.docname, &doc.mime, &doc.public, &doc.created)
		if err != nil {
			c.err = err
			return
		}

	}

	err = data.Err()
	if err != nil {
		c.err = err
		return
	}
	return
}

func (c *сontext) Log(rw web.ResponseWriter, req *web.Request, next web.NextMiddlewareFunc) {
	start := time.Now()
	next(rw, req)
	glog.Infof("[ %s ][ %s ] %s", time.Since(start), req.Method, req.URL)
}

func (c *сontext) showResponse(rw web.ResponseWriter, req *web.Request, resp *serverResponse) {
	var data []byte
	data, err := json.Marshal(resp)
	if err != nil {
		c.err = err
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	rw.Write(data)
}
