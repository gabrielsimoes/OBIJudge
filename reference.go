//parse references available as .zip
package main

import (
	"archive/zip"

	"golang.org/x/tools/godoc/vfs"
	"golang.org/x/tools/godoc/vfs/zipfs"
	yaml "gopkg.in/yaml.v2"
)

type ReferenceData struct {
	Name  string `yaml:"name"`
	Title string `yaml:"title"`
	Index string `yaml:"index"`
}

func initReference(location string) ([]ReferenceData, vfs.FileSystem, error) {
	rc, err := zip.OpenReader(location)
	if err != nil {
		return []ReferenceData{}, nil, err
	}

	fs := zipfs.New(rc, "")
	info, err := vfs.ReadFile(fs, "/info.yml")
	if err != nil {
		return []ReferenceData{}, nil, err
	}

	var reference []ReferenceData
	err = yaml.Unmarshal(info, &reference)
	if err != nil {
		return []ReferenceData{}, nil, err
	}

	return reference, fs, err
}
