package main

import (
	"github.com/asdine/storm"
	"github.com/boltdb/bolt"
	"golang.org/x/crypto/bcrypt"
)

// where we will keep the password hash
const HASH_BUCKET = "HASH_BUCKET"
const HASH_KEY = "HASH_KEY"

// struct for a contest's information
type ContestData struct {
	ID    int      `storm:"id,increment=1"`
	Name  string   `yaml:"name", storm:"unique"`
	Title string   `yaml:"title"`
	Tasks []string `yaml:"tasks"`
}

// struct for a task's information
type TaskData struct {
	ID          int         `storm:"id,increment=1"`
	Name        string      `yaml:"name", storm:"unique"`
	Title       string      `yaml:"title"`
	TimeLimit   int         `yaml:"time_limit"`
	MemoryLimit int         `yaml:"memory_limit"`
	NTests      int         `yaml:"n_tests"`
	Batches     []BatchData `yaml:"batches"`
}

// struct for information about a batch of test cases
type BatchData struct {
	Value int   `yaml:"value"`
	Tests []int `yaml:"tests"`
}

// struct for a tasks' html and pdf statements
type StatementData struct {
	ID   int    `storm:"id,increment=1"`
	Name string `storm:"unique"`
	HTML []byte
	PDF  []byte
}

// struct for test cases input and output
type TestData struct {
	ID     int `storm:"id,increment=1"`
	N      int `storm:"unique"`
	Input  []byte
	Output []byte
}

// a database handler
type database struct {
	stormdb *storm.DB
}

func openDatabase(path string) (*database, error) {
	db := &database{}
	var err error = nil
	db.stormdb, err = storm.Open(path,
		storm.BoltOptions(0400, &bolt.Options{ReadOnly: true}))

	if err != nil {
		return &database{}, err
	}

	return db, nil
}

func (db *database) close() error {
	return db.stormdb.Close()
}

func (db *database) authenticate(password []byte) bool {
	var hash []byte
	db.stormdb.Get(HASH_BUCKET, HASH_KEY, &hash)
	return bcrypt.CompareHashAndPassword(hash, password) == nil
}

func (db *database) getContests() ([]ContestData, error) {
	var contests []ContestData
	err := db.stormdb.All(&contests)
	return contests, err
}

func (db *database) getContest(name string) (ContestData, error) {
	contest := ContestData{}
	err := db.stormdb.One("Name", name, &contest)
	return contest, err
}

func (db *database) getTasks() ([]TaskData, error) {
	var tasks []TaskData
	err := db.stormdb.All(&tasks)
	return tasks, err
}

func (db *database) getTask(name string) (TaskData, error) {
	task := TaskData{}
	err := db.stormdb.One("Name", name, &task)
	return task, err
}

func (db *database) getContestTasks(name string) ([]TaskData, error) {
	contest, err := db.getContest(name)
	if err != nil {
		return []TaskData{}, err
	}

	var tasks []TaskData
	for _, taskname := range contest.Tasks {
		task, err := db.getTask(taskname)
		if err != nil {
			return []TaskData{}, err
		}
		tasks = append(tasks, task)
	}

	return tasks, err
}

func (db *database) getStatement(name string, key []byte) (StatementData, error) {
	statement := StatementData{}
	err := db.stormdb.One("Name", name, &statement)
	if err != nil {
		return StatementData{}, err
	}

	statement.HTML, err = decrypt(statement.HTML, key)
	if err != nil {
		return StatementData{}, err
	}

	statement.PDF, err = decrypt(statement.PDF, key)
	if err != nil {
		return StatementData{}, err
	}

	statement.HTML, err = decompress(statement.HTML)
	if err != nil {
		return StatementData{}, err
	}

	statement.PDF, err = decompress(statement.PDF)
	if err != nil {
		return StatementData{}, err
	}

	return statement, nil
}

func (db *database) getTests(name string, key []byte) ([]TestData, error) {
	var tests []TestData
	err := db.stormdb.From(name).All(&tests)
	if err != nil {
		return []TestData{}, err
	}

	for i := 0; i < len(tests); i++ {
		tests[i].Input, err = decrypt(tests[i].Input, key)
		if err != nil {
			return []TestData{}, err
		}

		tests[i].Output, err = decrypt(tests[i].Output, key)
		if err != nil {
			return []TestData{}, err
		}

		tests[i].Input, err = decompress(tests[i].Input)
		if err != nil {
			return []TestData{}, err
		}

		tests[i].Output, err = decompress(tests[i].Output)
		if err != nil {
			return []TestData{}, err
		}
	}

	return tests, err
}
