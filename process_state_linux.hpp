#ifdef __linux__

#include <sys/param.h>
#include <sys/ptrace.h>
#include <sys/user.h>
#include "syscalls_tab.h"

#if defined(__i386__)
#define I386
#elif defined(__x86_64__)
#if defined(__ILP32__)
#define X32
#else
#define X86_64
#endif
#endif

typedef unsigned long param_t;

void init_process_state();

class process_state {
 public:
  process_state(pid_t pid);

  enum SYSCALL get_syscall();
  void set_syscall(enum SYSCALL sys);

  const char* get_syscall_name(enum SYSCALL sys);

  param_t get_param(size_t i);
  void set_param(size_t i, param_t val);

  pid_t get_pid();
  bool error();

 private:
  pid_t pid;
  size_t pers;
  bool error_state;

#if defined(I386)
  struct user_regs_struct i386_regs;
#elif defined(X32) || defined(X86_64)
  struct user_regs_struct x86_64_regs;
#else
#error "UNKNOWN ARCHITECTURE"
#endif
  enum SYSCALL sys;
};

#endif  //__linux__
