package main

import "os/exec"

type Language interface {
	// Returns a name that identifies the language. e.g. C++11 / g++
	name() string

	// Extensions to be used
	sourceExtension() string

	// Whether this language requires multithreading
	requiresMultithreading() bool

	// Returns the compilation commands
	compilationCommand(sourceFilenames []string, executableFilename string) []string

	// Copies language-specific files to build directory
	copyExtraFiles(location string) error

	// Returns the evalutaion command
	evaluationCommand(executableFilename string, args []string) []string
}

var languageRegistry = map[string]Language{
	"cpp": &cpp{},
	// "c":    &c{},
	// "pas":  &pas{},
	// "py2":  &py2{},
	// "py3":  &py3{},
	// "java": &java{},
}

type cpp struct{}

func (_ *cpp) name() string                 { return "C++11 / g++" }
func (_ *cpp) sourceExtension() string      { return ".cpp" }
func (_ *cpp) requiresMultithreading() bool { return false }
func (_ *cpp) compilationCommand(sourceFilenames []string, executableFilename string) []string {
	path, _ := exec.LookPath("g++")
	command := []string{path, "-DEVAL", "-std=c++11", "-O2", "-pipe", "-static", "-s", "-o", executableFilename}
	return append(command, sourceFilenames...)
}
func (_ *cpp) copyExtraFiles(location string) error { return nil }
func (_ *cpp) evaluationCommand(executableFilename string, args []string) []string {
	return append([]string{"./" + executableFilename}, args...)
}

//// C
//type c struct{}

//func (_ *c) sourceName(taskname string) string {
//	return taskname + ".c"
//}

//func (_ *c) prepare(dir, taskname string) error {
//	cmd := exec.Command("gcc", "-static", "-pipe", "-lm", "-O2", "-std=gnu11", "-o", dir+"/"+taskname, dir+"/"+taskname+".c")
//	return cmd.Run()
//}

//// Pascal
//type pas struct{}

//func (_ *pas) sourceName(taskname string) string {
//	return taskname + ".pas"
//}

//func (_ *pas) prepare(dir, taskname string) error {
//	cmd := exec.Command("fpc", "-XS", "-Xt", "-O2", dir+"/"+taskname+".pas")
//	// var outb, errb bytes.Buffer
//	// cmd.Stdout = &outb
//	// cmd.Stderr = &errb
//	// err := cmd.Run()
//	// if outb.Len() > 0 || errb.Len() > 0 {
//	// 	fmt.Println(outb.String(), errb.String())
//	// }
//	// return err
//	return cmd.Run()
//}

//// Python 2
//type py2 struct{}

//func (_ *py2) sourceName(taskname string) string {
//	return taskname + ".py"
//}

//func (_ *py2) prepare(dir, taskname string) error {
//	return nil
//}

//// Python 3
//type py3 struct{}

//func (_ *py3) sourceName(taskname string) string {
//	return taskname + ".py"
//}

//func (_ *py3) prepare(dir, taskname string) error {
//	return nil
//}

//// Java
//type java struct{}

//func (_ *java) sourceName(taskname string) string {
//	return taskname + ".java"
//}

//func (_ *java) prepare(dir, taskname string) error {
//	cmd := exec.Command("javac", "-encoding", "UTF-8", dir+"/"+taskname+".java")
//	var outb, errb bytes.Buffer
//	cmd.Stdout = &outb
//	cmd.Stderr = &errb
//	err := cmd.Run()
//	if outb.Len() > 0 || errb.Len() > 0 {
//		fmt.Println(outb.String(), errb.String())
//	}
//	return err
//	// return cmd.Run()
//}

//// C++
//func (_ *cpp) run(dir, taskname string, time_limit, memory_limit int) int {
//	return run(dir, taskname, dir+"/input", dir+"/output",
//		[]string{}, time_limit, memory_limit,
//		append(SYSCALL_WHITELIST, C.sys_none),
//		append(FILESYSTEM_WHITELIST, dir), 0)
//}

//// C
//func (_ *c) run(dir, taskname string, time_limit, memory_limit int) int {
//	return run(dir, taskname, dir+"/input", dir+"/output",
//		[]string{}, time_limit, memory_limit,
//		append(SYSCALL_WHITELIST, C.sys_none),
//		append(FILESYSTEM_WHITELIST, dir), 0)
//}

//// Pascal
//func (_ *pas) run(dir, taskname string, time_limit, memory_limit int) int {
//	return run(dir, taskname, dir+"/input", dir+"/output",
//		[]string{}, time_limit, memory_limit,
//		append(SYSCALL_WHITELIST, C.sys_none),
//		append(FILESYSTEM_WHITELIST, dir), 0)
//}

//// Python 2
//func (_ *py2) run(dir, taskname string, time_limit, memory_limit int) int {
//	return run(dir, "/usr/bin/python2", dir+"/input", dir+"/output",
//		[]string{"-BSO", taskname + ".py"}, time_limit, memory_limit,
//		append(SYSCALL_WHITELIST, C.sys_none),
//		append(FILESYSTEM_WHITELIST, dir), 0)
//}

//// Python 3
//func (_ *py3) run(dir, taskname string, time_limit, memory_limit int) int {
//	return run(dir, "/usr/bin/python3", dir+"/input", dir+"/output",
//		[]string{"-BSO", taskname + ".py"}, time_limit, memory_limit,
//		append(SYSCALL_WHITELIST, C.sys_none),
//		append(FILESYSTEM_WHITELIST, dir), 0)
//}

//// Java
//func (_ *java) run(dir, taskname string, time_limit, memory_limit int) int {
//	policyBox := rice.MustFindBox("langfiles")
//	policyBytes, err := policyBox.Bytes("sandbox_java.policy")
//	if err != nil {
//		fmt.Println(err)
//		return ER
//	}

//	err = writeNewFile(dir+"/policy", policyBytes)
//	if err != nil {
//		fmt.Println(err)
//		return ER
//	}

//	return run(dir, "/usr/bin/java", dir+"/input", dir+"/output",
//		[]string{"-XX:+UseSerialGC", "-Djava.security.manager=default",
//			"-Djava.security.policy==" + dir + "/policy", "-Xss128m",
//			"-Xms128m", "-Xmx" + strconv.Itoa(memory_limit) + "m", taskname},
//		time_limit+1000, -1, []int32{}, []string{}, -1)
//}
