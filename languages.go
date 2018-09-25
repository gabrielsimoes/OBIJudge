package main

import (
	"os/exec"
	"strconv"
)

// AllLanguages is an array with all the programming languages support by the judge.
var AllLanguages = []Language{&cpp{}, &c{}, &java{}, &pas{}, &py2{}, &py3{}, &js{}}

// Language keeps information and methods related to a single programming
// language, indicating how it should be judged.
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

func (*cpp) Name() string                 { return "C++11 (g++)" }
func (*cpp) SourceExtension() string      { return ".cpp" }
func (*cpp) MimeType() string             { return "text/x-c++src" }
func (*cpp) RequiresMultithreading() bool { return false }
func (*cpp) UseMemoryLimit() bool         { return true }
func (*cpp) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	path, _ := exec.LookPath("g++")
	command := []string{path, "-DEVAL", "-std=c++11", "-O2", "-lm", "-pipe", "-static", "-s", "-o", executableFilename}
	return append(command, sourceFilenames...)
}
func (*cpp) CopyExtraFiles(location string) error { return nil }
func (*cpp) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	return append([]string{"./" + executableFilename}, args...)
}

type c struct{}

func (*c) Name() string                 { return "C (gcc)" }
func (*c) SourceExtension() string      { return ".c" }
func (*c) MimeType() string             { return "text/x-csrc" }
func (*c) RequiresMultithreading() bool { return false }
func (*c) UseMemoryLimit() bool         { return true }
func (*c) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	path, _ := exec.LookPath("gcc")
	command := []string{path, "-DEVAL", "-O2", "-lm", "-pipe", "-static", "-s", "-o", executableFilename}
	return append(command, sourceFilenames...)
}
func (*c) CopyExtraFiles(location string) error { return nil }
func (*c) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	return append([]string{"./" + executableFilename}, args...)
}

type java struct{}

func (*java) Name() string                 { return "Java (JDK)" }
func (*java) SourceExtension() string      { return ".java" }
func (*java) MimeType() string             { return "text/x-java" }
func (*java) RequiresMultithreading() bool { return true }
func (*java) UseMemoryLimit() bool         { return false }
func (*java) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	path, _ := exec.LookPath("javac")
	command := []string{path, "-encoding", "UTF-8", "-sourcepath", ".", "-d", "."}
	return append(command, sourceFilenames...)
}
func (*java) CopyExtraFiles(location string) error { return nil }
func (*java) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	path, _ := exec.LookPath("java")
	return append([]string{path, "-Dfile.encoding=UTF-8", "-XX:+UseSerialGC", "-Xss64m", "-Xmx" + strconv.Itoa(memoryLimit) + "k", executableFilename}, args...)
}

type pas struct{}

func (*pas) Name() string                 { return "Pascal (fpc)" }
func (*pas) SourceExtension() string      { return ".pas" }
func (*pas) MimeType() string             { return "text/x-pascal" }
func (*pas) RequiresMultithreading() bool { return false }
func (*pas) UseMemoryLimit() bool         { return true }
func (*pas) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	path, _ := exec.LookPath("fpc")
	command := []string{path, "-dEVAL", "-XS", "-Xt", "-O2", "-o" + executableFilename}
	return append(command, sourceFilenames...)
}
func (*pas) CopyExtraFiles(location string) error { return nil }
func (*pas) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	return append([]string{"./" + executableFilename}, args...)
}

type py2 struct{}

func (*py2) Name() string                 { return "Python 2" }
func (*py2) SourceExtension() string      { return ".py" }
func (*py2) MimeType() string             { return "text/x-python" }
func (*py2) RequiresMultithreading() bool { return false }
func (*py2) UseMemoryLimit() bool         { return true }
func (*py2) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	path, _ := exec.LookPath("python2")
	command := []string{path, "-m", "py_compile"}
	return append(command, sourceFilenames...)
}
func (*py2) CopyExtraFiles(location string) error { return nil }
func (*py2) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	path, _ := exec.LookPath("python2")
	return append([]string{path, executableFilename + ".pyc"}, args...)
}

type py3 struct{}

func (*py3) Name() string                 { return "Python 3" }
func (*py3) SourceExtension() string      { return ".py" }
func (*py3) MimeType() string             { return "text/x-python" }
func (*py3) RequiresMultithreading() bool { return false }
func (*py3) UseMemoryLimit() bool         { return true }
func (*py3) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	path, _ := exec.LookPath("python3")
	command := []string{path, "-c", "import py_compile as m; m.compile(\"" + sourceFilenames[0] + "\", \"" + executableFilename + "\", doraise=True)"}
	return command
}
func (*py3) CopyExtraFiles(location string) error { return nil }
func (*py3) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	path, _ := exec.LookPath("python3")
	return append([]string{path, executableFilename}, args...)
}

type js struct{}

func (*js) Name() string                 { return "JavaScript (Node.js)" }
func (*js) SourceExtension() string      { return ".js" }
func (*js) MimeType() string             { return "text/javascript" }
func (*js) RequiresMultithreading() bool { return false }
func (*js) UseMemoryLimit() bool         { return false }
func (*js) CompilationCommand(sourceFilenames []string, executableFilename string) []string {
	return nil
}
func (*js) CopyExtraFiles(location string) error { return nil }
func (*js) EvaluationCommand(executableFilename string, args []string, memoryLimit int) []string {
	path, _ := exec.LookPath("node")
	return append([]string{path, "--max-old-space-size=" + strconv.Itoa(memoryLimit>>10), executableFilename + ".js"}, args...)
}
