package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// struct for a contest's information
type ContestData struct {
	Name  string
	Title string
	Tasks []string
}

// struct for a task's information
type TaskData struct {
	Name        string
	Title       string
	TimeLimit   int
	MemoryLimit int
	NTests      int
	Batches     []BatchData
}

// struct for information about a batch of test cases
type BatchData struct {
	Value int
	Tests []int
}

// struct for a tasks' html and pdf statements
type StatementData struct {
	Name string
	HTML []byte
	PDF  []byte
}

// struct for test cases input and output
type TestData struct {
	N      int
	Input  []byte
	Output []byte
}

// a database handler
type Database struct {
	Archive *zip.ReadCloser
	Logger  *log.Logger
}

func OpenDatabase(path string) (*Database, error) {
	db := &Database{}
	var err error = nil
	db.Archive, err = zip.OpenReader(path)
	return db, err
}

func (db *Database) Close() error {
	return db.Archive.Close()
}

func (db *Database) filterFolder(path string) []*zip.File {
	var result []*zip.File
	for _, file := range db.Archive.File {
		if !strings.HasSuffix(file.Name, "/") &&
			strings.HasPrefix(file.Name, path) {
			result = append(result, file)
		}
	}

	return result
}

func (db *Database) filterFile(path string) *zip.File {
	for _, file := range db.Archive.File {
		if file.Name == path {
			return file
		}
	}

	return nil
}

func (db *Database) readFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	content, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	return content, nil
}

func (db *Database) readSecure(file *zip.File, key []byte) ([]byte, error) {
	content, err := db.readFile(file)
	if err != nil {
		return nil, err
	}

	content, err = decrypt(content, key)
	if err != nil {
		return nil, err
	}

	content, err = decompress(content)
	if err != nil {
		return nil, err
	}

	return content, nil
}

func (db *Database) Authenticate(password []byte) bool {
	file := db.filterFile("/hash")
	if file == nil {
		db.Logger.Print("Error: no hash file")
		return false
	}

	hash, err := db.readFile(file)
	if err != nil {
		db.Logger.Print(err)
		return false
	}

	return bcrypt.CompareHashAndPassword(hash, password) == nil
}

func (db *Database) Contests() ([]ContestData, error) {
	file := db.filterFile("/contests.json")
	if file == nil {
		return []ContestData{}, errors.New("No contests.json file")
	}

	content, err := db.readFile(file)
	if err != nil {
		return []ContestData{}, err
	}

	var contests []ContestData
	err = json.Unmarshal(content, &contests)
	return contests, err
}

func (db *Database) Contest(name string) (ContestData, error) {
	contests, err := db.Contests()
	if err != nil {
		return ContestData{}, err
	}

	for _, contest := range contests {
		if contest.Name == name {
			return contest, nil
		}
	}

	return ContestData{}, errors.New("No contest named " + name)
}

func (db *Database) Tasks() ([]TaskData, error) {
	file := db.filterFile("/tasks.json")
	if file == nil {
		return nil, errors.New("No tasks.json file")
	}

	content, err := db.readFile(file)
	if err != nil {
		return nil, err
	}

	var tasks []TaskData
	err = json.Unmarshal(content, &tasks)
	return tasks, err
}

func (db *Database) Task(name string) (TaskData, error) {
	tasks, err := db.Tasks()
	if err != nil {
		return TaskData{}, err
	}

	for _, task := range tasks {
		if task.Name == name {
			return task, nil
		}
	}

	return TaskData{}, errors.New("No task named " + name)
}

func (db *Database) ContestTasks(name string) ([]TaskData, error) {
	contest, err := db.Contest(name)
	if err != nil {
		return []TaskData{}, err
	}

	// TODO: make this faster:

	var tasks []TaskData
	for _, taskname := range contest.Tasks {
		task, err := db.Task(taskname)
		if err != nil {
			return []TaskData{}, err
		}
		tasks = append(tasks, task)
	}

	return tasks, err
}

func (db *Database) Statement(name string, key []byte) (StatementData, error) {
	statement := StatementData{}

	var err error = nil
	pdfFile := db.filterFile("/" + name + "/statements/statement.pdf")
	if pdfFile != nil {
		statement.PDF, err = db.readSecure(pdfFile, key)
		if err != nil {
			return statement, err
		}
	}

	htmlFile := db.filterFile("/" + name + "/statements/statement.html")
	if htmlFile != nil {
		statement.HTML, err = db.readSecure(htmlFile, key)
		if err != nil {
			return statement, err
		}
	}

	return statement, nil
}

func (db *Database) Tests(name string, key []byte) ([]TestData, error) {
	testFiles := db.filterFolder("/" + name + "/tests/")
	tests := make([]TestData, len(testFiles)/2)
	var err error = nil
	for _, file := range testFiles {
		info := strings.Split(filepath.Base(file.Name), ".")
		if info[1] == "in" {
			ix, _ := strconv.Atoi(info[0])
			tests[ix].Input, err = db.readSecure(file, key)
			if err != nil {
				return []TestData{}, err
			}
		} else {
			ix, _ := strconv.Atoi(info[0])
			tests[ix].Output, err = db.readSecure(file, key)
			if err != nil {
				return []TestData{}, err
			}
		}
	}

	return tests, err
}

func BuildDatabase(source, target string, password []byte, writePassword bool) error {
	source = filepath.Clean(source)
	target = filepath.Clean(target)

	// first, lets initialize our zip database
	_ = os.Remove(target)
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	defer file.Close()

	archive := zip.NewWriter(file)
	defer archive.Close()

	// choose password
	if len(password) != 0 && len(password) != 16 {
		return errors.New("Password has to be 16-letters long")
	} else if len(password) == 0 {
		password, err = generateKey(16)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Files encrypted with the key: '%s' (write it down!)\n", password)

	if writePassword {
		ioutil.WriteFile("pass", password, 0644)
	}

	// now lets store this key's hash in our database
	hash, err := bcrypt.GenerateFromPassword(password, 14)
	if err != nil {
		return err
	}

	f, err := archive.Create("/hash")
	if err != nil {
		return err
	}

	_, err = f.Write(hash)
	if err != nil {
		return err
	}

	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		header.Name = strings.TrimPrefix(path, source)
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		content, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		if filepath.Ext(path) != ".json" {
			content = compress(content)
			content, err = encrypt(content, password)
			if err != nil {
				return err
			}
		}

		_, err = io.Copy(writer, bytes.NewReader(content))
		if err != nil {
			return err
		}

		fmt.Println(path, "->", header.Name)
		return nil
	})

	if err != nil {
		return err
	}

	return nil
}
