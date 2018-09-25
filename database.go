package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// ContestData stores a contest's information
type ContestData struct {
	Name  string
	Title string
	Tasks []TaskData
}

// TaskData stores a task's information
type TaskData struct {
	Name        string
	Title       string
	TimeLimit   int
	MemoryLimit int
	NTests      int
	Batches     []BatchData
}

// BatchData stores information about a batch of test cases
type BatchData struct {
	Value int
	Tests []int
}

// StatementData stores a tasks' html and pdf statements
type StatementData struct {
	Name string
	HTML []byte
	PDF  []byte
}

// TestData stores a test case's input and output
type TestData struct {
	N      int
	Input  []byte
	Output []byte
}

// Database stores information related to a user-specific database
type Database struct {
	path    string
	archive *zip.ReadCloser
	lock    sync.Mutex
}

// OpenDatabase will copy the database file from formFile to a random location
// inside the specified folder and return a Database object representing the
// database.
func OpenDatabase(formFile multipart.File, folder string) (*Database, error) {
	randKey, _ := generateKey(32)
	path := filepath.Join(folder, string(randKey))

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(file, formFile)
	file.Close()
	if err != nil {
		os.Remove(path)
		return nil, err
	}

	archive, err := zip.OpenReader(path)
	if err != nil {
		os.Remove(path)
		return nil, err
	}

	return &Database{
		path:    path,
		archive: archive,
	}, nil
}

// Clear should be called when the database will not be used anymore, probably
// at the end of the program execution or at user logout.
func (db *Database) Clear() error {
	db.lock.Lock()
	defer db.lock.Unlock()

	err := db.archive.Close()
	if err != nil {
		return err
	}

	err = os.Remove(db.path)
	db.path = ""

	return err
}

func (db *Database) filterFolder(path string) []*zip.File {
	db.lock.Lock()
	defer db.lock.Unlock()

	var result []*zip.File
	for _, file := range db.archive.File {
		if !strings.HasSuffix(file.Name, "/") &&
			strings.HasPrefix(file.Name, path) {
			result = append(result, file)
		}
	}

	return result
}

func (db *Database) filterFile(path string) *zip.File {
	db.lock.Lock()
	defer db.lock.Unlock()

	for _, file := range db.archive.File {
		if file.Name == path {
			return file
		}
	}

	return nil
}

func (db *Database) readFile(file *zip.File) ([]byte, error) {
	db.lock.Lock()
	defer db.lock.Unlock()

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

// Authenticate is used to check if user-specified password matches the hash
// located inside the database file. This same password will have to be
// specified in order to access the encrypted contents inside the database file.
func (db *Database) Authenticate(password []byte) (bool, error) {
	file := db.filterFile("/hash")
	if file == nil {
		return false, errors.New("Error: no hash file")
	}

	hash, err := db.readFile(file)
	if err != nil {
		return false, err
	}

	return bcrypt.CompareHashAndPassword(hash, password) == nil, nil
}

// Contest returns a ContestData object corresponding to the contest stored
// inside the database.
func (db *Database) Contest() (ContestData, error) {
	file := db.filterFile("/info.json")
	if file == nil {
		return ContestData{}, errors.New("No info.json file")
	}

	content, err := db.readFile(file)
	if err != nil {
		return ContestData{}, err
	}

	var contest ContestData
	err = json.Unmarshal(content, &contest)
	return contest, err
}

// Tasks returns an array []TaskData corresponding to the tasks stored inside
// the database.
func (db *Database) Tasks() ([]TaskData, error) {
	contest, err := db.Contest()
	if err != nil {
		return []TaskData{}, err
	}

	return contest.Tasks, nil
}

// Task returns a single TaskData corresponding to the task with the specified
// name, stored inside the database.
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

// Statament returns a single StatementData corresponding to the statement of
// the task with the specified name, stored inside the database.
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

// Tests returns an array []TestData corresponding to all the tests of the
// task with the specified name, stored inside the database.
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

// BuildDatabase uses the files from the specified source folder to create a zip
// database in the correct format at the specified target folder. It will
// encrypt any sensitive files with the specified password, or ask for a new
// password. If the writePassword flag is set to true, it will write the used
// password to a file named pass in the current folder, for debug purposes.
func BuildDatabase(source, target string, password []byte, writePassword bool) error {
	source = filepath.Clean(source)
	target = filepath.Clean(target)

	// Initialize zip database
	_ = os.Remove(target)
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	defer file.Close()

	archive := zip.NewWriter(file)
	defer archive.Close()

	// Choose password
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

	// Store the key's hash in the database
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

	// Walk over all files, adding them to the zip database
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
