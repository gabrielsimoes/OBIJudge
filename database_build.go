package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func buildDatabase(source, target string, password []byte) error {
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

	f, err := archive.Create("hash")
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

		fmt.Println(path)
		return nil
	})

	if err != nil {
		return err
	}

	return nil
}
