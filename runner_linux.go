package main

/*#cgo LDFLAGS: -pthread -lstdc++ -lstdc++fs
#include "sandbox_linux.hpp"
#include "syscalls_tab.h"
#include <stdlib.h>
static char **makeCharArray(int size) { return calloc(sizeof(char*), size+1); }
static void setArrayString(char **a, char *s, int pos) { a[pos] = s; }
static void freeCharArray(char **a, int size) {
	for (int i = 0; i < size; i++) free(a[i]);
	free(a);
}*/
import "C"
import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	rice "github.com/GeertJohan/go.rice"
)

var FILESYSTEM_WHITELIST = []string{
	"\\/dev\\/(?:null|tty|zero|u?random)$",
	"\\/usr\\/(?!home)",
	"\\/lib(?:32|64)?\\/",
	"\\/opt\\/",
	"\\/etc$",
	"\\/etc\\/(?:localtime|timezone|nsswitch.conf|resolv.conf|passwd|malloc.conf)$",
	"\\/usr$",
	"\\/tmp$",
	"\\/$",
	"\\/sys\\/devices\\/system\\/cpu(?:$|\\/online)",
	"\\/etc\\/selinux\\/config$",
	"\\/proc\\/self\\/(?:maps|exe|auxv)$", "\\/proc\\/self$",
	"\\/proc\\/(?:meminfo|stat|cpuinfo|filesystems)$",
	"\\/proc\\/sys\\/vm\\/overcommit_memory$",
	"\\/etc\\/ld\\.so\\.(?:nohwcap|preload|cache)$",
}

var SYSCALL_WHITELIST = []int32{
	C.sys_readv,
	C.sys_read,
	C.sys_write,
	C.sys_writev,
	C.sys_statfs,
	C.sys_statfs64,
	C.sys_getpgrp,
	C.sys_restart_syscall,
	C.sys_select,
	C.sys__newselect,
	C.sys_pselect6,
	C.sys_modify_ldt,
	C.sys_poll,
	C.sys_ppoll,

	C.sys_getgroups,
	C.sys_getgroups32,
	C.sys_sched_getaffinity,
	C.sys_sched_getparam,
	C.sys_sched_getscheduler,
	C.sys_sched_get_priority_min,
	C.sys_sched_get_priority_max,
	C.sys_time,
	C.sys_timer_create,
	C.sys_timer_delete,
	C.sys_timerfd_create,
	C.sys_timerfd_gettime,
	C.sys_timerfd_settime,
	C.sys_timer_getoverrun,
	C.sys_timer_gettime,
	C.sys_timer_settime,

	C.sys_sigprocmask,
	C.sys_rt_sigprocmask,
	C.sys_sigreturn,
	C.sys_rt_sigreturn,
	C.sys_clock_nanosleep,
	C.sys_nanosleep,
	C.sys_sysinfo,
	C.sys_getrandom,

	C.sys_close,
	C.sys_dup,
	C.sys_dup2,
	C.sys_dup3,
	C.sys_mmap,
	C.sys_mmap2,
	C.sys_mremap,
	C.sys_mprotect,
	C.sys_madvise,
	C.sys_munmap,
	C.sys_brk,
	C.sys_fcntl,
	C.sys_fcntl64,
	C.sys_arch_prctl,
	C.sys_set_tid_address,
	C.sys_set_robust_list,
	C.sys_futex,
	C.sys_getegid,
	C.sys_sigaction,
	C.sys_sigprocmask,
	C.sys_rt_sigaction,
	C.sys_rt_sigprocmask,
	C.sys_getrlimit,
	C.sys_ugetrlimit,
	C.sys_getrusage,
	C.sys_ioctl,
	C.sys_getcwd,
	C.sys_getegid,
	C.sys_getegid32,
	C.sys_geteuid,
	C.sys_geteuid32,
	C.sys_getgid,
	C.sys_getgid32,
	C.sys_getpgid,
	C.sys_getpid,
	C.sys_getppid,
	C.sys_getresgid,
	C.sys_getresgid32,
	C.sys_getresuid,
	C.sys_getresuid32,
	C.sys_getsid,
	C.sys_gettid,
	C.sys_getuid,
	C.sys_getuid32,
	C.sys_getdents,
	C.sys_getdents64,
	C.sys_lseek,
	C.sys__llseek,
	C.sys_sigaltstack,

	C.sys_pipe,
	C.sys_pipe2,
	C.sys_clock_gettime,
	C.sys_clock_getres,
	C.sys_gettimeofday,
	C.sys_sched_yield,
	C.sys_clone,
	C.sys_exit,
	C.sys_exit_group,
	C.sys_set_thread_area,
	C.sys_oldolduname,
	C.sys_olduname,
	C.sys_uname,
	C.sys_prlimit64, // seems unsafe, but python3 needs it
}

func run(dir, cmd, input, output string,
	args []string, time_limit, memory_limit int,
	syscall_whitelist []int32, filesystem_whitelist []string,
	nproc int) int {

	config := C.get_default_config()
	config.time = C.int(time_limit)
	config.memory = C.int(memory_limit)
	config.nproc = C.int(nproc)

	config.stdin = C.CString(input)
	defer C.free(unsafe.Pointer(config.stdin))

	config.stdout = C.CString(output)
	defer C.free(unsafe.Pointer(config.stdout))

	config.dir = C.CString(dir)
	defer C.free(unsafe.Pointer(config.dir))

	config.cmd = C.CString(cmd)
	defer C.free(unsafe.Pointer(config.cmd))

	newargs := append([]string{cmd}, args...)
	config.argv = C.makeCharArray(C.int(len(newargs)))
	defer C.freeCharArray(config.argv, C.int(len(newargs)))
	for i, arg := range newargs {
		C.setArrayString(config.argv, C.CString(arg), C.int(i))
	}

	if len(syscall_whitelist) > 0 {
		config.syscall_whitelist = (*C.int)(&syscall_whitelist[0])
	}

	if len(filesystem_whitelist) > 0 {
		config.filesystem_whitelist = C.CString(strings.Join(filesystem_whitelist, "|"))
		defer C.free(unsafe.Pointer(config.filesystem_whitelist))
	}

	return int(C.run(config))
}

// C++
func (_ *cpp) run(dir, taskname string, time_limit, memory_limit int) int {
	return run(dir, taskname, dir+"/input", dir+"/output",
		[]string{}, time_limit, memory_limit,
		append(SYSCALL_WHITELIST, C.sys_none),
		append(FILESYSTEM_WHITELIST, dir), 0)
}

// C
func (_ *c) run(dir, taskname string, time_limit, memory_limit int) int {
	return run(dir, taskname, dir+"/input", dir+"/output",
		[]string{}, time_limit, memory_limit,
		append(SYSCALL_WHITELIST, C.sys_none),
		append(FILESYSTEM_WHITELIST, dir), 0)
}

// Pascal
func (_ *pas) run(dir, taskname string, time_limit, memory_limit int) int {
	return run(dir, taskname, dir+"/input", dir+"/output",
		[]string{}, time_limit, memory_limit,
		append(SYSCALL_WHITELIST, C.sys_none),
		append(FILESYSTEM_WHITELIST, dir), 0)
}

// Python 2
func (_ *py2) run(dir, taskname string, time_limit, memory_limit int) int {
	return run(dir, "/usr/bin/python2", dir+"/input", dir+"/output",
		[]string{"-BSO", taskname + ".py"}, time_limit, memory_limit,
		append(SYSCALL_WHITELIST, C.sys_none),
		append(FILESYSTEM_WHITELIST, dir), 0)
}

// Python 3
func (_ *py3) run(dir, taskname string, time_limit, memory_limit int) int {
	return run(dir, "/usr/bin/python3", dir+"/input", dir+"/output",
		[]string{"-BSO", taskname + ".py"}, time_limit, memory_limit,
		append(SYSCALL_WHITELIST, C.sys_none),
		append(FILESYSTEM_WHITELIST, dir), 0)
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
		time_limit+1000, -1, []int32{}, []string{}, -1)
}
