#ifdef __linux__

#ifdef __i386__
#define I386
#endif

#ifdef __x86_64__
#ifdef __ILP32__
#define X32
#else
#define X86_64
#endif
#endif

typedef enum { _32, _64, _UNKNOWN } Arch;
typedef enum {
  NO = 0,
  AC = 1,
  WA = 2,
  ML = 3,
  TL = 4,
  RE = 5,
  CE = 6,
  RV = 7,
  ER = 8
} Verdict;

typedef struct {
  int time;                       // time limit in milliseconds
  int memory;                     // memory limit in MB
  int nproc;                      // maximum number of processes
  char *dir;                      // dir to chdir to
  char *cmd;                      // file to execute
  char **argv;                    // argument parameters
  char **envp;                    // environment parameters
  char *stdin, *stdout, *stderr;  // file name to redirect as standard streams
                                  // (if unset, the stream will be closed)
  int *syscall_whitelist;         // allowed syscalls
  char *filesystem_whitelist;     // allowed files (read/write)
} Config;

#ifdef __cplusplus
extern "C" {
#endif

int run(Config config);
Config get_default_config();

#ifdef __cplusplus
}
#endif

static const unsigned int MB = 1024 * 1024;

#endif
