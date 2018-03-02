package main

import (
	"bytes"
	"fmt"
	"os/exec"
)

type runner interface {
	sourceName(taskname string) string
	prepare(dir, taskname string) error
	run(dir, taskname string, time_limit, memory_limit int) int
}

var runnerRegistry = map[string]runner{
	"cpp":  &cpp{},
	"c":    &c{},
	"pas":  &pas{},
	"py2":  &py2{},
	"py3":  &py3{},
	"java": &java{},
}

// C++
type cpp struct{}

func (_ *cpp) sourceName(taskname string) string {
	return taskname + ".cpp"
}

func (_ *cpp) prepare(dir, taskname string) error {
	cmd := exec.Command("g++", "-static", "-pipe", "-lm", "-x", "c++", "-O2", "-std=c++14", "-o", dir+"/"+taskname, dir+"/"+taskname+".cpp")
	return cmd.Run()
}

// C
type c struct{}

func (_ *c) sourceName(taskname string) string {
	return taskname + ".c"
}

func (_ *c) prepare(dir, taskname string) error {
	cmd := exec.Command("gcc", "-static", "-pipe", "-lm", "-O2", "-std=gnu11", "-o", dir+"/"+taskname, dir+"/"+taskname+".c")
	return cmd.Run()
}

// Pascal
type pas struct{}

func (_ *pas) sourceName(taskname string) string {
	return taskname + ".pas"
}

func (_ *pas) prepare(dir, taskname string) error {
	cmd := exec.Command("fpc", "-XS", "-Xt", "-O2", dir+"/"+taskname+".pas")
	// var outb, errb bytes.Buffer
	// cmd.Stdout = &outb
	// cmd.Stderr = &errb
	// err := cmd.Run()
	// if outb.Len() > 0 || errb.Len() > 0 {
	// 	fmt.Println(outb.String(), errb.String())
	// }
	// return err
	return cmd.Run()
}

// Python 2
type py2 struct{}

func (_ *py2) sourceName(taskname string) string {
	return taskname + ".py"
}

func (_ *py2) prepare(dir, taskname string) error {
	return nil
}

// Python 3
type py3 struct{}

func (_ *py3) sourceName(taskname string) string {
	return taskname + ".py"
}

func (_ *py3) prepare(dir, taskname string) error {
	return nil
}

// Java
type java struct{}

func (_ *java) sourceName(taskname string) string {
	return taskname + ".java"
}

func (_ *java) prepare(dir, taskname string) error {
	cmd := exec.Command("javac", "-encoding", "UTF-8", dir+"/"+taskname+".java")
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	err := cmd.Run()
	if outb.Len() > 0 || errb.Len() > 0 {
		fmt.Println(outb.String(), errb.String())
	}
	return err
	// return cmd.Run()
}
