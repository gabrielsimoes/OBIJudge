package main

import (
	"fmt"

	rice "github.com/GeertJohan/go.rice"
)
import "strconv"

func run(dir, cmd, input, output string, args []string, time_limit, memory_limit int) int {
	return AC
}

// C++
func (_ *cpp) run(dir, taskname string, time_limit, memory_limit int) int {
	return run(dir, taskname, dir+"/input", dir+"/output",
		[]string{}, time_limit, memory_limit)
}

// C
func (_ *c) run(dir, taskname string, time_limit, memory_limit int) int {
	return run(dir, taskname, dir+"/input", dir+"/output",
		[]string{}, time_limit, memory_limit)
}

// Pascal
func (_ *pas) run(dir, taskname string, time_limit, memory_limit int) int {
	return run(dir, taskname, dir+"/input", dir+"/output",
		[]string{}, time_limit, memory_limit)
}

// Python 2
func (_ *py2) run(dir, taskname string, time_limit, memory_limit int) int {
	return run(dir, "/usr/bin/python2", dir+"/input", dir+"/output",
		[]string{"-BSO", taskname + ".py"}, time_limit, memory_limit)
}

// Python 3
func (_ *py3) run(dir, taskname string, time_limit, memory_limit int) int {
	return run(dir, "/usr/bin/python3", dir+"/input", dir+"/output",
		[]string{"-BSO", taskname + ".py"}, time_limit, memory_limit)
}

// Java
func (_ *java) run(dir, taskname string, time_limit, memory_limit int) int {
	policyBox := rice.MustFindBox("langfiles")
	policyBytes, err := policyBox.Bytes("sandbox_java.policy")
	if err != nil {
		fmt.Println(err)
		return ER
	}

	err = writeNewFile(dir+"/policy", policyBytes)
	if err != nil {
		fmt.Println(err)
		return ER
	}

	return run(dir, "/usr/bin/java", dir+"/input", dir+"/output",
		[]string{"-XX:+UseSerialGC", "-Djava.security.manager=default",
			"-Djava.security.policy==" + dir + "/policy", "-Xss128m",
			"-Xms128m", "-Xmx" + strconv.Itoa(memory_limit) + "m", taskname},
		time_limit, memory_limit)
}
