/**
 * This file contains the sandbox code for linux. Works by tracing syscalls
 * with ptrace and blocking unwanted ones. Also tracks running time and memory
 * usage.
 */
#ifdef __linux__

#include "sandbox_linux.hpp"
#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <signal.h>
#include <stdio.h>
#include <string.h>
#include <sys/ptrace.h>
#include <sys/reg.h>
#include <sys/resource.h>
#include <sys/time.h>
#include <sys/types.h>
#include <sys/user.h>
#include <sys/wait.h>
#include <unistd.h>
#include <chrono>
#include <experimental/filesystem>
#include <future>
#include <iostream>
#include <map>
#include <regex>
#include <set>
#include <string>
#include <thread>
#include <vector>
#include "process_state_linux.hpp"
namespace fs = std::experimental::filesystem;
using namespace std::chrono;

std::atomic<bool> thread_stop;
bool exec_failed;
bool first_syscall;
std::set<int> syscall_whitelist;
std::regex filesystem_whitelist;

/**
 * @brief Gets the memory usage of a process in MB.
 *
 * @param[in] pid The process pid
 *
 * @return -1 if error, otherwise the memory usage in MB.
 */
int get_memory_usage(pid_t pid) {
  // statm file will be used to compute memory usage.
  char statm_path[40];
  sprintf(statm_path, "/proc/%d/statm", pid);

  FILE *fs = fopen(statm_path, "r");
  if (!fs) {
    return -1;
  }

  int pages_used = 0;
  for (int i = 0; i < 6; i++) {
    fscanf(fs, "%d", &pages_used);
  }
  fclose(fs);

  return (pages_used * getpagesize()) / MB;
}

void setlimit(int res, rlim_t softlimit, rlim_t hardlimit) {
  struct rlimit rl;
  rl.rlim_cur = softlimit;
  rl.rlim_max = hardlimit;
  setrlimit(res, &rl);
}

void setlimit(int res, rlim_t softlimit) {
  setlimit(res, softlimit, softlimit + softlimit);
}

/**
 * Child process part
 */
void run_child(Config *config, pid_t parent) {
  if (config->memory >= 0) {
    setlimit(RLIMIT_DATA, (config->memory + 10) * MB);
    setlimit(RLIMIT_AS, (config->memory + 10) * MB);
  }

  if (config->time >= 0)
    setlimit(RLIMIT_CPU, (2 * config->time + 999) / 1000,
             (3 * config->time) / 1000);

  if (config->nproc >= 0) setlimit(RLIMIT_NPROC, config->nproc);

  if (config->dir && *config->dir) chdir(config->dir);

  // increase stack limit
  setlimit(RLIMIT_STACK, RLIM_INFINITY, RLIM_INFINITY);

  // disable core dumps
  setlimit(RLIMIT_CORE, 0);

  // mirror file descriptors for file streams
  if (config->stdin >= 0)
    dup2(open(config->stdin, O_RDONLY), STDIN_FILENO);
  else
    fclose(stdin);

  if (config->stdout)
    dup2(open(config->stdout, O_WRONLY), STDOUT_FILENO);
  else
    fclose(stdout);

  if (config->stderr >= 0)
    dup2(open(config->stderr, O_WRONLY), STDERR_FILENO);
  else
    fclose(stderr);

  ptrace(PTRACE_TRACEME);  // allow tracing

  execve(config->cmd, config->argv, config->envp);

  // execve failed
  kill(parent, SIGUSR1);    // warn parent
  kill(getpid(), SIGKILL);  // kill myself
}

int time_tracker(pid_t child, int time_limit) {
  if (time_limit < 0) return 0;

  high_resolution_clock::time_point start_time, current_time;
  start_time = high_resolution_clock::now();
  int running_time;

  while (!thread_stop) {
    if (kill(child, 0) != 0) {
      std::cout << "Running time: " << running_time << std::endl;
      return 0;
    }

    current_time = high_resolution_clock::now();
    running_time =
        duration_cast<milliseconds>(current_time - start_time).count();

    if (running_time > time_limit) {
      std::cout << "Running time: " << running_time << std::endl;
      kill(child, SIGKILL);
      return 1;
    }
  }

  std::cout << "Running time: " << running_time << std::endl;
  return 0;
}

int memory_tracker(pid_t child, int memory_limit) {
  if (memory_limit < 0) return 0;

  while (!thread_stop) {
    if (kill(child, 0) != 0) return 0;

    int memory_usage = get_memory_usage(child);

    if (memory_usage < 0) return -1;

    if (memory_usage > memory_limit) {
      kill(child, SIGKILL);
      return 1;
    }
  }

  return 0;
}

std::string read_param(pid_t pid, param_t addr) {
  if (addr == 0) return std::string();

  int allocated = 1024, copied = 0;
  char *buffer = (char *)malloc(allocated);
  unsigned long word;

  while (1) {
    if (copied + sizeof(word) > allocated) {
      allocated *= 2;
      buffer = (char *)realloc(buffer, allocated);
    }

    word = ptrace(PTRACE_PEEKDATA, pid, addr + copied);
    if (errno) {
      buffer[copied] = 0;
      break;
    }

    memcpy(buffer + copied, &word, sizeof(word));

    // If we've already encountered null, break and return
    if (memchr(&word, 0, sizeof(word)) != NULL) break;

    copied += sizeof(word);
  }

  std::string result(buffer);
  free(buffer);
  return result;
}

std::string do_readlink(std::string const &path) {
  char buff[PATH_MAX];
  ssize_t len = ::readlink(path.c_str(), buff, sizeof(buff) - 1);
  if (len != -1) {
    buff[len] = '\0';
    return std::string(buff);
  } else {
    return std::string();
  }
}

std::string getcwd_pid(pid_t pid) {
  return do_readlink("/proc/" + std::to_string(pid) + "/cwd");
}

std::string getfd_pid(pid_t pid, int fd) {
  return do_readlink("/proc/" + std::to_string(pid) + "/fd/" +
                     std::to_string(fd));
}

std::string get_full_path(pid_t pid, std::string relative_file,
                          int dirfd = AT_FDCWD) {
  dirfd = (dirfd & 0x7FFFFFFF) - (dirfd & 0x80000000);
  relative_file = fs::path(relative_file);

  if (relative_file.find("/") == 0) {
    return relative_file;
  } else {
    if (dirfd == AT_FDCWD)
      return fs::path(getcwd_pid(pid)) / fs::path(relative_file);
    else
      return fs::path(getfd_pid(pid, dirfd)) / fs::path(relative_file);
  }
}

bool handle_open(process_state &st, bool atdir) {
  long dirfd = atdir ? st.get_param(0) : AT_FDCWD;
  std::string file = read_param(st.get_pid(), st.get_param(atdir ? 1 : 0));
  std::string full_path = get_full_path(st.get_pid(), file, dirfd);
  return std::regex_search(full_path, filesystem_whitelist);
}

bool handle_kill(process_state &st) { return st.get_param(0) == st.get_pid(); }

bool handle_prctl(process_state &st) {
  switch (st.get_param(0)) {
    case 3:
    case 15:
      return true;
    default:
      return false;
  }
}

bool handle_syscall(process_state &st) {
  if (first_syscall) {
    if (st.get_syscall() == sys_execve) {
      first_syscall = false;
      return true;
    } else {
      return false;
    }
  }

  if (syscall_whitelist.empty()) return true;

  switch (st.get_syscall()) {
    case sys_openat:
    case sys_faccessat:
    case sys_readlinkat:
    case sys_fstatat64:
    case sys_newfstatat:
      return handle_open(st, true);

    case sys_open:
    case sys_access:
    case sys_mkdir:
    case sys_unlink:
    case sys_readlink:
    case sys_oldstat:
    case sys_stat:
    case sys_stat64:
    case sys_oldfstat:
    case sys_fstat:
    case sys_fstat64:
    case sys_oldlstat:
    case sys_lstat:
    case sys_lstat64:
      return handle_open(st, false);

    case sys_tgkill:
    case sys_tkill:
    case sys_kill:
      return handle_kill(st);

    case sys_prctl:
      return handle_prctl(st);

    default:
      return syscall_whitelist.find(st.get_syscall()) !=
             syscall_whitelist.end();
  }
}

/**
 * Parent process part
 */
int run_parent(Config *config, pid_t child) {
  signal(SIGUSR1, [](int signum) { exec_failed = true; });

  if (config->syscall_whitelist) {
    for (int i = 0; config->syscall_whitelist[i]; i++) {
      syscall_whitelist.insert(config->syscall_whitelist[i]);
    }
  }

  if (config->filesystem_whitelist) {
    filesystem_whitelist = std::regex(config->filesystem_whitelist);
  } else {
    filesystem_whitelist = std::regex(".*");
  }

  auto time_future = std::async(time_tracker, child, config->time);
  auto mem_future = std::async(memory_tracker, child, config->memory);

  auto end = [&time_future, &mem_future, child](Verdict def) {
    kill(child, SIGCONT);
    kill(child, SIGKILL);

    int time_result = time_future.get();
    int mem_result = mem_future.get();

    if (exec_failed) {
      fprintf(stderr, "[sandbox] Exec failed\n");
      return ER;
    } else if (time_result < 0) {
      fprintf(stderr, "[sandbox] Time thread errored!\n");
      return ER;
    } else if (mem_result < 0) {
      fprintf(stderr, "[sandbox] Memory thread errored!\n");
      return ER;
    } else if (time_result) {
      fprintf(stderr, "[sandbox] Time limit reached\n");
      return TL;
    } else if (mem_result) {
      fprintf(stderr, "[sandbox] Memory limit reached\n");
      return ML;
    } else {
      return def;
    }
  };

  // loop syscalls monitoring
  while (true) {
    int status;

    // wait for syscall
    ptrace(PTRACE_SYSCALL, child, NULL, NULL);
    waitpid(child, &status, 0);

    if (WIFSIGNALED(status)) {  // program was sigkilled!
      fprintf(stderr, "[sandbox] Program was sigkilled\n");
      return end(RE);
    } else if (WIFEXITED(status)) {    // program has exited
      if (WEXITSTATUS(status) == 0) {  // everything ok
        fprintf(stderr, "[sandbox] Program exited without problems\n");
        return AC;
      } else {  // return code indicates error
        fprintf(stderr, "[sandbox] Program exited with error code\n");
        return RE;
      }
    } else if (WIFSTOPPED(status)) {  // process was stopped by tracing
      int signal = WSTOPSIG(status);

      if (signal == SIGTRAP) {
        // we got a syscall
        process_state st(child);
        if (!handle_syscall(st)) {  // got a syscall
          fprintf(stderr, "[sandbox] Bad syscall: %s (%d)\n",
                  st.get_syscall_name(st.get_syscall()), st.get_syscall());
          return end(RV);
        }

      } else if (signal == SIGXCPU) {
        fprintf(stderr, "[sandbox] CPU limit reached");
        return end(TL);
      } else if (signal == SIGABRT) {
        fprintf(stderr, "[sandbox] Program aborted\n");
        return end(RE);
      } else if (signal == SIGSEGV) {
        fprintf(stderr, "[sandbox] Program received SIGSEGV\n");
        return end(RE);
      } else if (signal == SIGFPE) {
        fprintf(stderr, "[sandbox] Program received SIGFPE\n");
        return end(RE);
      } else {  // this shouldn't have happened
        fprintf(stderr, "[sandbox] Program stopped by signal %d\n", signal);
        return end(ER);
      }
    }
  }

  return end(ER);
}

/**
 * Main function used to sandbox the execution
 */
int run(Config config) {
  thread_stop = false;
  exec_failed = false;
  first_syscall = true;
  syscall_whitelist.clear();
  filesystem_whitelist = std::regex();
  init_process_state();

  pid_t parent = getpid();
  pid_t pid = fork();

  switch (pid) {
    case -1:  // fork didn't go well
      fprintf(stderr, "[sandbox] Fork failed\n");
      return ER;
    case 0:  // 0 means we're inside child process
      run_child(&config, parent);
      exit(0);
    default:  // else we are the parent
      return run_parent(&config, pid);
  }
}

Config get_default_config() {
  return {
      -1,    // time
      -1,    // memory
      -1,    // nproc
      NULL,  // dir
      NULL,  // cmd
      NULL,  // argv
      NULL,  // envp
      NULL,  // stdin
      NULL,  // stderr
      NULL,  // stdout
      NULL,  // syscall_whitelist
      NULL   // filesystem_whitelist
  };
}

#endif  //__linux__
