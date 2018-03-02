package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
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
type database struct {
	archive *zip.ReadCloser
}

func openDatabase(path string) (*database, error) {
	db := &database{}
	var err error = nil
	db.archive, err = zip.OpenReader(path)
	return db, err
}

func (db *database) close() error {
	return db.archive.Close()
}

func (db *database) filterFolder(path string) []*zip.File {
	var result []*zip.File
	for _, file := range db.archive.File {
		if !strings.HasSuffix(file.Name, "/") &&
			strings.HasPrefix(file.Name, path) {
			result = append(result, file)
		}
	}

	return result
}

func (db *database) filterFile(path string) *zip.File {
	for _, file := range db.archive.File {
		if file.Name == path {
			return file
		}
	}

	return nil
}

func (db *database) readFile(file *zip.File) ([]byte, error) {
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

func (db *database) readSecure(file *zip.File, key []byte) ([]byte, error) {
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

func (db *database) authenticate(password []byte) bool {
	file := db.filterFile("hash")
	if file == nil {
		fmt.Fprintln(os.Stderr, "Error: no hash file")
	}

	hash, err := db.readFile(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return false
	}

	return bcrypt.CompareHashAndPassword(hash, password) == nil
}

func (db *database) getContests() ([]ContestData, error) {
	file := db.filterFile("contests.json")
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

func (db *database) getContest(name string) (ContestData, error) {
	contests, err := db.getContests()
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

func (db *database) getTasks() ([]TaskData, error) {
	file := db.filterFile("tasks.json")
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

func (db *database) getTask(name string) (TaskData, error) {
	tasks, err := db.getTasks()
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

func (db *database) getContestTasks(name string) ([]TaskData, error) {
	contest, err := db.getContest(name)
	if err != nil {
		return []TaskData{}, err
	}

	// TODO: make this faster:

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

	var err error = nil
	pdfFile := db.filterFile(name + "/statements/statement.pdf")
	if pdfFile != nil {
		statement.PDF, err = db.readSecure(pdfFile, key)
		if err != nil {
			return statement, err
		}
	}

	htmlFile := db.filterFile(name + "/statements/statement.html")
	if htmlFile != nil {
		statement.HTML, err = db.readSecure(htmlFile, key)
		if err != nil {
			return statement, err
		}
	}

	return statement, nil
}

func (db *database) getTests(name string, key []byte) ([]TestData, error) {
	testFiles := db.filterFolder(name + "/tests/")
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

// Compress a []byte with gzip
func compress(data []byte) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write(data)
	w.Close()
	return b.Bytes()
}

// Decompress a gzipped []byte
func decompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return []byte{}, err
	}

	b, _ := ioutil.ReadAll(r)
	if err != nil {
		return []byte{}, err
	}

	return b, nil
}

// Generates a 16-byte 128-bit AES alphanumeric key
func generateAES16Key() ([]byte, error) {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz1"

	key := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, key[:])
	if err != nil {
		return []byte{}, err
	}

	for i, b := range key {
		key[i] = letters[b%byte(len(letters))]
	}

	return key, nil
}

// Encrypt encrypts data using 128-bit AES-GCM.  This both hides the content of
// the data and provides a check that it hasn't been altered. Output takes the
// form nonce|ciphertext|tag where '|' indicates concatenation.
func encrypt(plaintext []byte, key []byte) (ciphertext []byte, err error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts data using 128-bit AES-GCM.  This both hides the content of
// the data and provides a check that it hasn't been altered. Expects input
// form nonce|ciphertext|tag where '|' indicates concatenation.
func decrypt(ciphertext []byte, key []byte) (plaintext []byte, err error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("malformed ciphertext")
	}

	return gcm.Open(nil,
		ciphertext[:gcm.NonceSize()],
		ciphertext[gcm.NonceSize():],
		nil,
	)
}
