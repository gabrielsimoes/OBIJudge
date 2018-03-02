package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"

	"github.com/asdine/storm"
	"golang.org/x/crypto/bcrypt"
	yaml "gopkg.in/yaml.v2"
)

func buildDatabase(source, target string, password []byte) error {
	// first, lets initialize our database
	_ = os.Remove(target)
	db, err := storm.Open(target)
	if err != nil {
		return err
	}
	defer db.Close()

	// parse contests.yml
	contestsYaml, err := ioutil.ReadFile(path.Join(source, "contests.yml"))
	if err != nil {
		return err
	}

	var contests []ContestData
	err = yaml.Unmarshal(contestsYaml, &contests)
	if err != nil {
		return err
	}

	for i, _ := range contests {
		err = db.Save(&contests[i])
		if err != nil {
			return err
		}
	}

	// choose password
	if len(password) != 0 && len(password) != 16 {
		return errors.New("Password has to be 16-letters long")
	} else if len(password) == 0 {
		password, err = generateAES16Key()

		if err != nil {
			return err
		}
	}

	fmt.Printf("Files encrypted with the key: '%s' (write it down!)\n", password)

	// now lets store this key's hash in our database
	hash, err := bcrypt.GenerateFromPassword(password, 14)
	if err != nil {
		return err
	}

	db.Set(HASH_BUCKET, HASH_KEY, hash)

	// load folders inside source (each should contain a task)
	taskFolders, err := ioutil.ReadDir(source)
	if err != nil {
		return err
	}

	for _, taskFolder := range taskFolders {
		if !taskFolder.IsDir() {
			continue
		}

		// get yaml task info from directory
		taskYaml, err := ioutil.ReadFile(path.Join(source, taskFolder.Name(), "task.yml"))
		if err != nil {
			return err
		}

		// parse the yaml task info
		var task TaskData
		err = yaml.Unmarshal(taskYaml, &task)
		if err != nil {
			return err
		}

		// build a full batch if none
		if len(task.Batches) == 0 {
			tests := make([]int, task.NTests)
			for i := 0; i < task.NTests; i++ {
				tests[i] = i
			}
			task.Batches = []BatchData{{100, tests}}
		}

		statement := StatementData{}
		statement.Name = task.Name

		// store html statement
		html, err := ioutil.ReadFile(path.Join(source, taskFolder.Name(), "statements", "statement.html"))
		if err != nil { // we will only store if file exists
			html = []byte{}
		}
		html = compress(html)
		html, err = encrypt(html, password)
		if err != nil {
			return err
		}
		statement.HTML = html

		// store pdf statement
		pdf, err := ioutil.ReadFile(path.Join(source, taskFolder.Name(), "statements", "statement.pdf"))
		if err != nil { // we will only store if file exists
			pdf = []byte{}
		}
		pdf = compress(pdf)
		pdf, err = encrypt(pdf, password)
		if err != nil {
			return err
		}
		statement.PDF = pdf

		// store task info into database
		err = db.Save(&task)
		if err != nil {
			return err
		}

		err = db.Save(&statement)
		if err != nil {
			return err
		}

		// we will keep task testcases in a specific bucket
		taskBucket := db.From(task.Name)

		// parse and store testcases
		dir := path.Join(source, taskFolder.Name(), "tests")
		for i := 0; i < task.NTests; i++ {
			fmt.Println(taskFolder.Name(), i)

			in, err := ioutil.ReadFile(path.Join(dir, strconv.Itoa(i)+".in"))
			if err != nil {
				return err
			}

			out, err := ioutil.ReadFile(path.Join(dir, strconv.Itoa(i)+".out"))
			if err != nil {
				return err
			}

			in = compress(in)
			in, err = encrypt(in, password)
			if err != nil {
				return err
			}

			out = compress(out)
			out, err = encrypt(out, password)
			if err != nil {
				return err
			}

			test := TestData{N: i, Input: in, Output: out}
			err = taskBucket.Save(&test)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
