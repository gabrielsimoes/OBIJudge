package main

import (
	"os/exec"
	"strconv"
)

var (
	Languages = []Language{&cpp{}, &c{}, &java{}, &pas{}, &py2{}, &py3{}, &js{}}
)

type Language interface {
	// Returns a name that identifies the language. e.g. C++11 (g++)
	Name() string

	// Extension to be used
	SourceExtension() string

	// Mime type
	MimeType() string

	// Whether this language requires multithreading
	RequiresMultithreading() bool

	// Whether this language memory usage should be restricted
	UseMemoryLimit() bool

	// Returns the compilation commands
	CompilationCommand(sourceFilenames []string, executableFilename string) []string

	// Copies language-specific files to build directory
	CopyExtraFiles(location string) error

	// Returns the evalutaion command
	EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string
}

type cpp struct{}

func (_ *cpp) Name() string                 { return "C++11 (g++)" }
func (_ *cpp) SourceExtension() string      { return ".cpp" }
func (_ *cpp) MimeType() string             { return "text/x-c++src" }
func (_ *cpp) RequiresMultithreading() bool { return false }
func (_ *cpp) UseMemoryLimit() bool         { return true }
func (_ *cpp) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	path, _ := exec.LookPath("g++")
	command := []string{path, "-DEVAL", "-std=c++11", "-O2", "-lm", "-pipe", "-static", "-s", "-o", executableFilename}
	return append(command, sourceFilenames...)
}
func (_ *cpp) CopyExtraFiles(location string) error { return nil }
func (_ *cpp) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	return append([]string{"./" + executableFilename}, args...)
}

type c struct{}

func (_ *c) Name() string                 { return "C (gcc)" }
func (_ *c) SourceExtension() string      { return ".c" }
func (_ *c) MimeType() string             { return "text/x-csrc" }
func (_ *c) RequiresMultithreading() bool { return false }
func (_ *c) UseMemoryLimit() bool         { return true }
func (_ *c) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	path, _ := exec.LookPath("gcc")
	command := []string{path, "-DEVAL", "-O2", "-lm", "-pipe", "-static", "-s", "-o", executableFilename}
	return append(command, sourceFilenames...)
}
func (_ *c) CopyExtraFiles(location string) error { return nil }
func (_ *c) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	return append([]string{"./" + executableFilename}, args...)
}

type java struct{}

func (_ *java) Name() string                 { return "Java (JDK)" }
func (_ *java) SourceExtension() string      { return ".java" }
func (_ *java) MimeType() string             { return "text/x-java" }
func (_ *java) RequiresMultithreading() bool { return true }
func (_ *java) UseMemoryLimit() bool         { return false }
func (_ *java) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	path, _ := exec.LookPath("javac")
	command := []string{path, "-encoding", "UTF-8", "-sourcepath", ".", "-d", "."}
	return append(command, sourceFilenames...)
}
func (_ *java) CopyExtraFiles(location string) error { return nil }
func (_ *java) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	path, _ := exec.LookPath("java")
	return append([]string{path, "-Dfile.encoding=UTF-8", "-XX:+UseSerialGC", "-Xss64m", "-Xmx" + strconv.Itoa(memoryLimit) + "k", executableFilename}, args...)
}

type pas struct{}

func (_ *pas) Name() string                 { return "Pascal (fpc)" }
func (_ *pas) SourceExtension() string      { return ".pas" }
func (_ *pas) MimeType() string             { return "text/x-pascal" }
func (_ *pas) RequiresMultithreading() bool { return false }
func (_ *pas) UseMemoryLimit() bool         { return true }
func (_ *pas) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	path, _ := exec.LookPath("fpc")
	command := []string{path, "-dEVAL", "-XS", "-Xt", "-O2", "-o" + executableFilename}
	return append(command, sourceFilenames...)
}
func (_ *pas) CopyExtraFiles(location string) error { return nil }
func (_ *pas) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	return append([]string{"./" + executableFilename}, args...)
}

type py2 struct{}

func (_ *py2) Name() string                 { return "Python 2" }
func (_ *py2) SourceExtension() string      { return ".py" }
func (_ *py2) MimeType() string             { return "text/x-python" }
func (_ *py2) RequiresMultithreading() bool { return false }
func (_ *py2) UseMemoryLimit() bool         { return true }
func (_ *py2) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	path, _ := exec.LookPath("python2")
	command := []string{path, "-m", "py_compile"}
	return append(command, sourceFilenames...)
}
func (_ *py2) CopyExtraFiles(location string) error { return nil }
func (_ *py2) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	path, _ := exec.LookPath("python2")
	return append([]string{path, executableFilename + ".pyc"}, args...)
}

type py3 struct{}

func (_ *py3) Name() string                 { return "Python 3" }
func (_ *py3) SourceExtension() string      { return ".py" }
func (_ *py3) MimeType() string             { return "text/x-python" }
func (_ *py3) RequiresMultithreading() bool { return false }
func (_ *py3) UseMemoryLimit() bool         { return true }
func (_ *py3) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	path, _ := exec.LookPath("python3")
	command := []string{path, "-c", "import py_compile as m; m.compile(\"" + sourceFilenames[0] + "\", \"" + executableFilename + "\", doraise=True)"}
	return command
}
func (_ *py3) CopyExtraFiles(location string) error { return nil }
func (_ *py3) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	path, _ := exec.LookPath("python3")
	return append([]string{path, executableFilename}, args...)
}

type js struct{}

func (_ *js) Name() string                 { return "JavaScript (Node.js)" }
func (_ *js) SourceExtension() string      { return ".js" }
func (_ *js) MimeType() string             { return "text/javascript" }
func (_ *js) RequiresMultithreading() bool { return false }
func (_ *js) UseMemoryLimit() bool         { return false }
func (_ *js) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	return nil
}
func (_ *js) CopyExtraFiles(location string) error { return nil }
func (_ *js) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	path, _ := exec.LookPath("node")
	return append([]string{path, "--max-old-space-size=" + strconv.Itoa(memoryLimit>>10), "--max-new-space-size=" + strconv.Itoa(memoryLimit), executableFilename + ".js"}, args...)
}
