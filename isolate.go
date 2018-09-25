// This is a Go port of github.com/ioi/isolate (with some personal changes)
// "A sandbox for securely executing untrusted programs."
// Works by using Linux kernel features like Control Groups and Namespaces
package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/containerd/cgroups"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
)

/*
#include <unistd.h>
*/
import "C"

const (
	boxFirstUID  = 60000
	boxFirstGID  = 60000
	boxRoot      = "/obibox"
	boxNumLimit  = 2
	boxImageSize = 10 << 20 // 10 MB

	errChildFailed = 42
)

const (
	dirFlagRW = 1 << iota
	dirFlagNoExec
	dirFlagOptional
	dirFlagDev
)

var (
	ticksPerSec  = int(C.sysconf(C._SC_CLK_TCK))
	tickDuration = time.Second / time.Duration(ticksPerSec)
	pageSize     = unix.Getpagesize()
)

// StatusCode is an integer indicating the outcome of a program execution
// inside a sandbox instance.
type StatusCode int

const (
	// StatusOK means the execution was completely successful.
	StatusOK StatusCode = iota

	// StatusWTL means the Wall Time Limit was exceeded.
	StatusWTL

	// StatusCTL means the CPU Time Limit was exceeded.
	StatusCTL

	// StatusSig means the program has been signaled or killed.
	StatusSig

	// StatusExit means the program exited with an error code.
	StatusExit

	// StatusError means an error has occurred in the sandbox.
	StatusError
)

// BoxResult stores information related to a single execution inside a sandbox
// instance.
type BoxResult struct {
	// CPUTime usage
	CPUTime time.Duration

	// WallTime usage
	WallTime time.Duration

	// Memory usage in KB
	Memory int64

	// Information about what happened in the sandbox
	Status StatusCode

	// Error information
	Error string

	// Fatal signal the program received
	Signal syscall.Signal

	// If the program wasn't killed, this will contain the exit code
	ExitCode int
}

// Box stores information representing a single sandbox instance.
type Box struct {
	// When running multiple boxes, each should have their own id
	ID int

	// Box location in filesystem (absolute path)
	BoxPath string

	// Box filesystem image path
	BoxImg string
}

// BoxConfig represents the parameters used in a single execution at the
// Sandbox. A BoxConfig struct should not be reused in another execution.
type BoxConfig struct {
	// Executable path
	Path string
	// Arguments to be passed to Exec
	Args []string
	// Environment variables to be passed to Exec
	Env []string
	// io.Reader to be used as the standard input
	Stdin io.Reader
	// io.Writer to be used as the standard output
	Stdout io.Writer
	// io.Writer to be used as the standard error
	Stderr io.Writer

	// Enable use of control groups
	EnableCgroups bool
	// Limit cpu time (will kill if execution reaches this)
	CPUTimeLimit time.Duration
	// Limit wall time (will kill if execution reaches this)
	WallTimeLimit time.Duration
	// Limit memory usage in KB
	MemoryLimit int64
	// Maximum number of processes
	MaxProcesses int

	boxPath   string
	boxUID    int
	boxGID    int
	control   cgroups.Cgroup
	parentPid int
	childPid  int
	errorPipe *os.File
	startTime time.Time
	result    *BoxResult

	childFiles      []*os.File
	closeAfterStart []io.Closer
	closeAfterWait  []io.Closer
	goroutine       []func() error
	errch           chan error // one send per goroutine
}

// Sandbox is used to initialize an instance of the Sandbox corresponding to
// the indicated id, returning a Box object representing such instance.
func Sandbox(id int) (*Box, error) {
	if id < 0 || id >= boxNumLimit {
		return nil, fmt.Errorf("Invalid box number: %d", id)
	}

	b := &Box{
		ID:      id,
		BoxPath: filepath.Join(boxRoot, strconv.Itoa(id)),
		BoxImg:  filepath.Join(boxRoot, strconv.Itoa(id)+".img"),
	}

	if os.Geteuid() != 0 {
		return nil, errors.New("Must be run as root")
	}

	if os.Getegid() != 0 {
		return nil, errors.New("Must be run as root group")
	}

	unix.Umask(077)

	os.RemoveAll(b.BoxPath)
	os.RemoveAll(b.BoxImg)

	if err := os.MkdirAll(boxRoot, 0777); err != nil {
		return nil, err
	}

	img, err := os.Create(b.BoxImg)
	if err != nil {
		return nil, err
	}

	if err := img.Truncate(boxImageSize); err != nil {
		b.Clear()
		return nil, err
	}

	img.Close()
	output, err := exec.Command("mkfs.ext4", "-O", "^has_journal", "-q", b.BoxImg).CombinedOutput()
	if err != nil {
		b.Clear()
		return nil, errors.New(err.Error() + ":" + string(output))
	}

	if err := os.Mkdir(b.BoxPath, 0777); err != nil {
		b.Clear()
		return nil, err
	}

	if err := os.Mkdir(filepath.Join(b.BoxPath, "box"), 0700); err != nil {
		b.Clear()
		return nil, err
	}

	output, err = exec.Command("mount", "-o", "loop,rw,usrquota,grpquota", b.BoxImg, filepath.Join(b.BoxPath, "box")).CombinedOutput()
	if err != nil {
		b.Clear()
		return nil, errors.New(err.Error() + " - " + string(output))
	}

	origUID := os.Getuid()
	origGID := os.Getgid()
	if err := os.Chown(filepath.Join(b.BoxPath, "box"), origUID, origGID); err != nil {
		b.Clear()
		return nil, err
	}

	return b, nil
}

// Clear should be called once the Sandbox is done being used, typically at
// the end of the whole program execution.
func (b *Box) Clear() {
	if len(b.BoxPath) > 0 {
		exec.Command("umount", filepath.Join(b.BoxPath, "box")).Run()
		os.RemoveAll(b.BoxPath)
		os.RemoveAll(b.BoxImg)
	}
}

// Run is used to make an atomic execution of a program. It receives a
// BoxConfig file as argument, which should be used only once and indicates
// all the execution configurate. It returns a BoxResult object representing
// the result of the program execution.
func (b *Box) Run(c *BoxConfig) *BoxResult {
	c.result = &BoxResult{}

	if os.Geteuid() != 0 {
		c.result.Status = StatusError
		c.result.Error = "Must be run as root"
		return c.result
	}

	if os.Getegid() != 0 {
		c.result.Status = StatusError
		c.result.Error = "Must be run as root group"
		return c.result
	}

	unix.Umask(077)

	if c.EnableCgroups {
		for _, dir := range []string{"", "memory", "cpuacct", "cpuset"} {
			if err := testDir(filepath.Join("/sys/fs/cgroup", dir)); err != nil {
				c.result.Status = StatusError
				c.result.Error = "Can't support cgroups: " + err.Error()
				return c.result
			}
		}
	}

	c.boxUID = boxFirstUID + b.ID
	c.boxGID = boxFirstGID + b.ID
	c.boxPath = b.BoxPath

	if c.EnableCgroups {
		var err error
		c.control, err = cgroups.New(cgroups.V1, cgroups.StaticPath(fmt.Sprintf("box-%d-%d", b.ID, rand.Intn(1))), &specs.LinuxResources{})
		if err != nil {
			c.result.Status = StatusError
			c.result.Error = err.Error()
			return c.result
		}
		defer c.control.Delete()

		if c.MemoryLimit != 0 {
			memlimit := int64(c.MemoryLimit) << 10
			err := c.control.Update(&specs.LinuxResources{
				Memory: &specs.LinuxMemory{
					Limit: &memlimit,
					Swap:  &memlimit,
				},
			})
			if err != nil {
				c.result.Status = StatusError
				c.result.Error = err.Error()
				return c.result
			}
		}
	}

	if err := filepath.Walk(filepath.Join(c.boxPath, "box"), func(name string, info os.FileInfo, err error) error {
		if err == nil {
			err = os.Chown(name, c.boxUID, c.boxGID)
		}
		return err
	}); err != nil {
		c.result.Status = StatusError
		c.result.Error = err.Error()
		return c.result
	}

	type F func(*BoxConfig) (*os.File, error)
	for _, setupFd := range []F{(*BoxConfig).stdin, (*BoxConfig).stdout, (*BoxConfig).stderr} {
		f, err := setupFd(c)
		if err != nil {
			c.closeDescriptors(c.closeAfterStart)
			c.closeDescriptors(c.closeAfterWait)
			c.result.Status = StatusError
			c.result.Error = err.Error()
			return c.result
		}
		c.childFiles = append(c.childFiles, f)
	}

	epr, epw, err := os.Pipe()
	if err != nil {
		c.closeDescriptors(c.closeAfterStart)
		c.closeDescriptors(c.closeAfterWait)
		c.result.Status = StatusError
		c.result.Error = err.Error()
		return c.result
	}
	c.parentPid = os.Getpid()

	syscall.ForkLock.Lock()

	r1, _, err1 := unix.RawSyscall(unix.SYS_CLONE,
		uintptr(unix.SIGCHLD|unix.CLONE_NEWIPC|unix.CLONE_NEWNET|unix.CLONE_NEWNS|unix.CLONE_NEWPID),
		0, 0)

	if err1 != 0 {
		c.closeDescriptors(c.closeAfterStart)
		c.closeDescriptors(c.closeAfterWait)
		c.result.Status = StatusError
		c.result.Error = "Clone failed: " + err1.Error()
		return c.result
	}

	if r1 == 0 {
		epr.Close()
		c.errorPipe = epw

		errCode := c.runChild()
		c.errorPipe.Write([]byte{byte(errCode)})
		c.errorPipe.Close()
		os.Exit(errChildFailed)
		return nil
	}

	syscall.ForkLock.Unlock()
	c.childPid = int(r1)

	epw.Close()
	c.errorPipe = epr
	c.closeDescriptors(c.closeAfterStart)

	c.errch = make(chan error, len(c.goroutine))
	for _, fn := range c.goroutine {
		go func(fn func() error) {
			c.errch <- fn()
		}(fn)
	}

	err = c.runParent()

	var copyError error
	for range c.goroutine {
		if err := <-c.errch; err != nil && copyError == nil {
			copyError = err
		}
	}

	c.errorPipe.Close()
	c.closeDescriptors(c.closeAfterWait)

	if err != nil {
		c.result.Status = StatusError
		c.result.Error = err.Error()
	} else if copyError != nil {
		c.result.Status = StatusError
		c.result.Error = copyError.Error()
	}

	return c.result
}

func (c *BoxConfig) end(err error) error {
	unix.Kill(-c.childPid, unix.SIGKILL)
	unix.Kill(c.childPid, unix.SIGKILL)

	return err
}

func (c *BoxConfig) updateResult(rusage *unix.Rusage) {
	c.result.WallTime = time.Since(c.startTime)

	if c.EnableCgroups {
		stats, err := c.control.Stat(cgroups.IgnoreNotExist)
		if err == nil {
			c.result.CPUTime = time.Duration(stats.CPU.Usage.Total)
			c.result.Memory = int64(stats.Memory.Usage.Max >> 10)
			if int64(stats.Memory.Swap.Usage>>10) > c.result.Memory {
				c.result.Memory = int64(stats.Memory.Swap.Max >> 10)
			}
			return
		}
	}

	if rusage != nil {
		c.result.CPUTime = time.Duration(rusage.Utime.Nano() + rusage.Stime.Nano())
		c.result.Memory = int64(rusage.Maxrss)
		return
	}

	stat, err := os.Open(filepath.Join("/proc", strconv.Itoa(c.childPid), "stat"))
	if err == nil {
		defer stat.Close()

		bs, err := ioutil.ReadAll(stat)
		if err == nil {
			parsed := strings.Split(string(bs), " ")
			stime, _ := strconv.ParseInt(parsed[13], 10, 64)
			utime, _ := strconv.ParseInt(parsed[14], 10, 64)
			c.result.CPUTime = (time.Second * time.Duration(utime+stime)) / time.Duration(ticksPerSec)

			rss, _ := strconv.ParseInt(parsed[23], 10, 64)
			c.result.Memory = (rss * int64(pageSize)) >> 10
		}
	}
}

// runParent is the main function used inside the parent process. Everything in
// the parent process after the fork happens here. It is mainly responsible
// for watching and killing the child process.
func (c *BoxConfig) runParent() error {
	c.startTime = time.Now()

	type waitResult struct {
		stat   unix.WaitStatus
		rusage unix.Rusage
	}

	waitch := make(chan waitResult)
	waiterrch := make(chan error)

	go func() {
		var result waitResult
		p, err := unix.Wait4(c.childPid, &result.stat, 0, &result.rusage)
		if err != nil {
			waiterrch <- err
			return
		}

		if p != c.childPid {
			waiterrch <- fmt.Errorf("Wait4: Unknown pid %d exited", p)
			return
		}

		waitch <- result
	}()

	ticker := time.NewTicker(tickDuration)
	for range ticker.C {
		select {
		case result := <-waitch:
			c.updateResult(&result.rusage)

			if result.stat.Exited() {
				if result.stat.ExitStatus() == errChildFailed {
					errorBytes, err := ioutil.ReadAll(c.errorPipe)
					if err != nil {
						return err
					}
					return fmt.Errorf("runChild returned error code: %d", errorBytes[0])
				}
				c.result.ExitCode = result.stat.ExitStatus()
				if c.result.ExitCode != 0 {
					c.result.Status = StatusExit
				}
				return nil
			} else if result.stat.Signaled() {
				c.result.Signal = result.stat.Signal()
				c.result.Status = StatusSig
				return nil
			} else if result.stat.Stopped() {
				c.result.Signal = result.stat.StopSignal()
				c.result.Status = StatusSig
				return nil
			} else {
				return c.end(fmt.Errorf("Wait4: Unknown status %+v", result.stat))
			}

		case err := <-waiterrch:
			return c.end(err)

		default:
			c.updateResult(nil)
			if c.WallTimeLimit != 0 && c.result.WallTime > c.WallTimeLimit {
				c.result.Status = StatusWTL
				return c.end(nil)
			}

			if c.CPUTimeLimit != 0 && c.result.CPUTime > c.CPUTimeLimit {
				c.result.Status = StatusCTL
				return c.end(nil)
			}
		}
	}

	return nil
}

// setupRoot is used to configure the folder where the program will run. It
// should be called inside the child process, before executing the program.
func (c *BoxConfig) setupRoot() error {
	if err := os.Chdir(c.boxPath); err != nil {
		return err
	}

	os.RemoveAll("root")
	if err := os.Mkdir("root", 0750); err != nil {
		return err
	}

	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return err
	}

	if err := unix.Mount("none", "root", "tmpfs", 0, "mode=755"); err != nil {
		return err
	}

	if err := os.Mkdir("root/tmp", 0750); err != nil {
		return err
	}

	for _, rule := range []struct {
		in    string
		out   string
		flags int
	}{
		{"box", "./box", dirFlagRW},
		{"bin", "/bin", 0},
		{"dev", "/dev", dirFlagDev},
		{"lib", "/lib", 0},
		{"lib64", "/lib64", dirFlagOptional},
		{"proc", "/proc", 0},
		{"usr", "/usr", 0},
		{"etc", "/etc", 0},
	} {
		if testDir(rule.out) != nil {
			if rule.flags&dirFlagOptional != 0 {
				continue
			}

			return errors.New("There is no " + rule.out + " directory")
		}

		if err := os.MkdirAll(filepath.Join("root", rule.in), 0777); err != nil {
			return err
		}

		var mountFlags uintptr
		if rule.flags&dirFlagRW == 0 {
			mountFlags |= unix.MS_RDONLY
		}
		if rule.flags&dirFlagNoExec != 0 {
			mountFlags |= unix.MS_NOEXEC
		}
		if rule.flags&dirFlagDev == 0 {
			mountFlags |= unix.MS_NODEV
		}

		if rule.in == "proc" {
			if err := unix.Mount("none", filepath.Join("root", rule.in), "proc", mountFlags, "hidepid=2"); err != nil {
				return err
			}
		} else {
			mountFlags |= unix.MS_BIND | unix.MS_NOSUID
			if err := unix.Mount(rule.out, filepath.Join("root", rule.in), "none", mountFlags, ""); err != nil {
				return err
			}
		}
	}

	if err := unix.Chroot("root"); err != nil {
		return err
	}

	if err := os.Chdir("root/box"); err != nil {
		return err
	}

	return nil
}

// setupRlimits is used to configure Rlimits inside the child process. It
// should run inside the child process, before executing the program.
func (c *BoxConfig) setupRlimits() error {
	if c.MemoryLimit != 0 {
		memlimit := uint64(c.MemoryLimit) << 10
		if err := unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: memlimit, Max: memlimit}); err != nil {
			return err
		}
	}

	if err := unix.Setrlimit(unix.RLIMIT_STACK, &unix.Rlimit{Cur: unix.RLIM_INFINITY, Max: unix.RLIM_INFINITY}); err != nil {
		return err
	}

	if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &unix.Rlimit{Cur: 64, Max: 64}); err != nil {
		return err
	}

	if err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		return err
	}

	if c.MaxProcesses != 0 {
		if err := unix.Setrlimit(unix.RLIMIT_NPROC, &unix.Rlimit{Cur: uint64(c.MaxProcesses), Max: uint64(c.MaxProcesses)}); err != nil {
			return err
		}
	}

	return nil
}

// setupCredentials is used to configure permissions in the child process. It
// should be called inside the child process, before executing the program.
func (c *BoxConfig) setupCredentials() error {
	if err := unix.Setresgid(c.boxGID, c.boxGID, c.boxGID); err != nil {
		return err
	}

	if err := unix.Setgroups(nil); err != nil {
		return err
	}

	if err := unix.Setresuid(c.boxUID, c.boxUID, c.boxUID); err != nil {
		return err
	}

	if err := unix.Setpgid(0, 0); err != nil {
		return err
	}

	return nil
}

// setupFds is used to configure the file descriptors used in the child process.
// It will close some of the open ones, and redirect (duplicate) others.
// It should be called inside the child process, before executing the program.
func (c *BoxConfig) setupFds() error {
	var lastfd int
	for _, f := range c.childFiles {
		if int(c.errorPipe.Fd()) == lastfd {
			lastfd++
		}

		if f != nil {
			err := unix.Dup2(int(f.Fd()), lastfd)
			if err != nil {
				return err
			}
		} else {
			unix.Close(lastfd)
		}

		lastfd++
	}

	fds, err := ioutil.ReadDir("/proc/self/fd")
	if err != nil {
		return err
	}

	for _, f := range fds {
		fd, _ := strconv.Atoi(f.Name())
		if fd >= lastfd && fd != int(c.errorPipe.Fd()) {
			unix.Close(fd)
		}
	}

	return nil
}

// runChild is the main function used inside the child process. Everything in
// the child process happens from here. It is mainly responsible for
// configuring the environment and executing the program.
func (c *BoxConfig) runChild() int {
	if c.EnableCgroups {
		if err := c.control.Add(cgroups.Process{Pid: os.Getpid()}); err != nil {
			return 1
		}
	}

	if err := c.setupRoot(); err != nil {
		return 2
	}

	if err := c.setupRlimits(); err != nil {
		return 3
	}

	if err := c.setupCredentials(); err != nil {
		return 4
	}

	if err := c.setupFds(); err != nil {
		return 5
	}

	c.Env = append(c.Env, "LIBC_FATAL_STDERR_=1")

	if err := unix.Exec(c.Path, c.Args, c.Env); err != nil {
		return 6
	}

	return 7
}

func testDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("testDir: %s should be a directory", dir)
	}

	return nil
}

//// Below code was taken from golang.org/src/os/exec/exec.go
//// Licensed under the following license: https://golang.org/LICENSE

func interfaceEqual(a, b interface{}) bool {
	defer func() {
		recover()
	}()
	return a == b
}

func skipStdinCopyError(err error) bool {
	pe, ok := err.(*os.PathError)
	return ok &&
		pe.Op == "write" && pe.Path == "|1" &&
		pe.Err == syscall.EPIPE
}

func (c *BoxConfig) stdin() (f *os.File, err error) {
	if c.Stdin == nil {
		f, err = os.Open(os.DevNull)
		if err != nil {
			return
		}
		c.closeAfterStart = append(c.closeAfterStart, f)
		return
	}

	if f, ok := c.Stdin.(*os.File); ok {
		return f, nil
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return
	}

	c.closeAfterStart = append(c.closeAfterStart, pr)
	c.closeAfterWait = append(c.closeAfterWait, pw)
	c.goroutine = append(c.goroutine, func() error {
		_, err := io.Copy(pw, c.Stdin)
		if skip := skipStdinCopyError; skip != nil && skip(err) {
			err = nil
		}
		if err1 := pw.Close(); err == nil {
			err = err1
		}
		return err
	})
	return pr, nil
}

func (c *BoxConfig) stdout() (f *os.File, err error) {
	return c.writerDescriptor(c.Stdout)
}

func (c *BoxConfig) stderr() (f *os.File, err error) {
	if c.Stderr != nil && interfaceEqual(c.Stderr, c.Stdout) {
		return c.childFiles[1], nil
	}
	return c.writerDescriptor(c.Stderr)
}

func (c *BoxConfig) writerDescriptor(w io.Writer) (f *os.File, err error) {
	if w == nil {
		f, err = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		c.closeAfterStart = append(c.closeAfterStart, f)
		return
	}

	if f, ok := w.(*os.File); ok {
		return f, nil
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return
	}

	c.closeAfterStart = append(c.closeAfterStart, pw)
	c.closeAfterWait = append(c.closeAfterWait, pr)
	c.goroutine = append(c.goroutine, func() error {
		_, err := io.Copy(w, pr)
		pr.Close()
		return err
	})
	return pw, nil
}

func (c *BoxConfig) closeDescriptors(closers []io.Closer) {
	for _, fd := range closers {
		fd.Close()
	}
}

// StdinPipe returns a pipe that will be connected to the command's
// standard input when the command starts.
// The pipe will be closed automatically after Wait sees the command exit.
// A caller need only call Close to force the pipe to close sooner.
// For example, if the command being run will not exit until standard input
// is closed, the caller must close the pipe.
func (c *BoxConfig) StdinPipe() (io.WriteCloser, error) {
	if c.Stdin != nil {
		return nil, errors.New("exec: Stdin already set")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	c.Stdin = pr
	c.closeAfterStart = append(c.closeAfterStart, pr)
	wc := &closeOnce{File: pw}
	c.closeAfterWait = append(c.closeAfterWait, wc)
	return wc, nil
}

type closeOnce struct {
	*os.File

	once sync.Once
	err  error
}

func (c *closeOnce) Close() error {
	c.once.Do(c.close)
	return c.err
}

func (c *closeOnce) close() {
	c.err = c.File.Close()
}

// StdoutPipe returns a pipe that will be connected to the command's
// standard output when the command starts.
//
// Wait will close the pipe after seeing the command exit, so most callers
// need not close the pipe themselves; however, an implication is that
// it is incorrect to call Wait before all reads from the pipe have completed.
// For the same reason, it is incorrect to call Run when using StdoutPipe.
// See the example for idiomatic usage.
func (c *BoxConfig) StdoutPipe() (io.ReadCloser, error) {
	if c.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	c.Stdout = pw
	c.closeAfterStart = append(c.closeAfterStart, pw)
	c.closeAfterWait = append(c.closeAfterWait, pr)
	return pr, nil
}

// StderrPipe returns a pipe that will be connected to the command's
// standard error when the command starts.
//
// Wait will close the pipe after seeing the command exit, so most callers
// need not close the pipe themselves; however, an implication is that
// it is incorrect to call Wait before all reads from the pipe have completed.
// For the same reason, it is incorrect to use Run when using StderrPipe.
// See the StdoutPipe example for idiomatic usage.
func (c *BoxConfig) StderrPipe() (io.ReadCloser, error) {
	if c.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	c.Stderr = pw
	c.closeAfterStart = append(c.closeAfterStart, pw)
	c.closeAfterWait = append(c.closeAfterWait, pr)
	return pr, nil
}
