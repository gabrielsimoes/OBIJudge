package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	RESULT_NOTHING int = iota
	RESULT_TIMEOUT
	RESULT_SIGNAL
	RESULT_FAILED
	RESULT_CORRECT
	RESULT_WRONG
)

const (
	RESULT_COMP_NOTHING int = iota
	RESULT_COMP_TIMEOUT
	RESULT_COMP_SIGNAL
	RESULT_COMP_FAILED
	RESULT_COMP_SUCCESS
)

const (
	NUM_WORKERS = 2
	ENV_HOME    = "HOME=/box"
	ENV_PATH    = "PATH=/usr/bin:/usr/local/bin:/box"
)

var (
	ENV         []string        = []string{ENV_HOME, ENV_PATH}
	submissions chan submission = make(chan submission, 100)
)

type TaskVerdict struct {
	Score       int
	Batches     []BatchVerdict
	Compilation int
	Error       bool
	Extra       string
	When        time.Time
	Code        string
}

type BatchVerdict struct {
	Result int
	Score  int
	Time   time.Duration
	Memory int64
	Extra  string
}

type submission struct {
	task *TaskData
	db   *database
	key  []byte
	code []byte
	lang language
}

func (s *submission) judge(boxId int) TaskVerdict {
	box, err := Sandbox(boxId)
	if err != nil {
		return TaskVerdict{Error: true, Extra: err.Error()}
	}
	defer box.Clear()

	err = s.lang.copyExtraFiles(box.BoxPath)
	if err != nil {
		return TaskVerdict{Error: true, Extra: err.Error()}
	}

	err = writeNewFile(filepath.Join(box.BoxPath, "box", s.task.Name+s.lang.sourceExtension()), s.code)
	if err != nil {
		return TaskVerdict{Error: true, Extra: err.Error()}
	}

	compilationCommand := s.lang.compilationCommand([]string{s.task.Name + s.lang.sourceExtension()}, s.task.Name)
	var compilationOutput bytes.Buffer
	compilationResult := box.Run(&BoxConfig{
		Path:          compilationCommand[0],
		Args:          compilationCommand,
		Env:           ENV,
		Stdout:        &compilationOutput,
		Stderr:        &compilationOutput,
		EnableCgroups: true,
		MemoryLimit:   1 << 20, // 1GB
		CPUTimeLimit:  2 * time.Minute,
		WallTimeLimit: 2 * time.Minute,
	})

	var ret TaskVerdict

	if compilationResult.Status == STATUS_ERR {
		return TaskVerdict{Error: true, Extra: compilationResult.Error}
	} else {
		if compilationResult.Status == STATUS_WTL || compilationResult.Status == STATUS_CTL {
			ret.Compilation = RESULT_COMP_TIMEOUT
			return ret
		} else if compilationResult.Status == STATUS_SIG {
			ret.Compilation = RESULT_COMP_SIGNAL
			ret.Extra = strconv.Itoa(int(compilationResult.Signal))
			return ret
		} else if compilationResult.Status == STATUS_EXT {
			ret.Compilation = RESULT_COMP_FAILED
			ret.Extra = strconv.Itoa(compilationResult.ExitCode) + "\n" + compilationOutput.String()
			return ret
		} else if compilationResult.Status == STATUS_OK {
			ret.Compilation = RESULT_COMP_SUCCESS
		}
	}

	tests, err := s.db.getTests(s.task.Name, s.key)
	if err != nil {
		return TaskVerdict{Error: true, Extra: err.Error()}
	}

	if len(s.task.Batches) == 0 {
		tests := make([]int, s.task.NTests)
		for i := 0; i < s.task.NTests; i++ {
			tests[i] = i
		}
		s.task.Batches = []BatchData{{100, tests}}
	}

	results := make([]struct {
		code   int
		extra  string
		time   time.Duration
		memory int64
	}, len(tests))
	ret.Batches = make([]BatchVerdict, len(s.task.Batches))

	for batchNumber, batch := range s.task.Batches {
		ret.Batches[batchNumber].Result = RESULT_CORRECT

		for _, i := range batch.Tests {
			test := tests[i]
			if results[i].code == RESULT_NOTHING {
				command := s.lang.evaluationCommand(s.task.Name, nil)
				var output bytes.Buffer
				result := box.Run(&BoxConfig{
					Path:          command[0],
					Args:          command,
					Env:           ENV,
					Stdin:         bytes.NewReader(test.Input),
					Stdout:        &output,
					EnableCgroups: true,
					MemoryLimit:   int64(s.task.MemoryLimit),
					CPUTimeLimit:  time.Duration(s.task.TimeLimit) * time.Millisecond,
					WallTimeLimit: time.Duration(s.task.TimeLimit) * time.Millisecond,
				})

				if result.Status == STATUS_ERR {
					return TaskVerdict{Error: true, Extra: result.Error}
				} else {
					results[i].time = result.CPUTime
					results[i].memory = result.Memory

					if result.Status == STATUS_WTL || result.Status == STATUS_CTL {
						results[i].code = RESULT_TIMEOUT
					} else if result.Status == STATUS_SIG {
						results[i].code = RESULT_SIGNAL
						results[i].extra = strconv.Itoa(int(result.Signal))
					} else if result.Status == STATUS_EXT {
						results[i].code = RESULT_FAILED
						results[i].extra = strconv.Itoa(result.ExitCode)
					} else if result.Status == STATUS_OK {
						results[i].code = RESULT_CORRECT
					}
				}

				if results[i].code == RESULT_CORRECT {
					if strings.Compare(strip(output.String()), strip(string(test.Output))) != 0 {
						results[i].code = RESULT_WRONG
					}
				}
			}

			if results[i].time > ret.Batches[batchNumber].Time {
				ret.Batches[batchNumber].Time = results[i].time
			}

			if results[i].memory > ret.Batches[batchNumber].Memory {
				ret.Batches[batchNumber].Memory = results[i].memory
			}

			if results[i].code != RESULT_CORRECT {
				ret.Batches[batchNumber].Result = results[i].code
				ret.Batches[batchNumber].Extra = results[i].extra
				break
			}
		}

		if ret.Batches[batchNumber].Result == RESULT_CORRECT {
			ret.Batches[batchNumber].Score = batch.Value
			ret.Score += batch.Value
		}
	}

	return ret
}

func runJudge() {
	for id := 0; id < NUM_WORKERS; id++ {
		go func(id int) {
			for s := range submissions {
				when := time.Now()
				verdict := s.judge(id)
				verdict.When = when
				verdict.Code = string(s.code)
				fmt.Printf("%+v\n", verdict)
			}
		}(id)
	}
}

func strip(in string) string {
	white := false
	var out string

	for _, c := range in {
		if unicode.IsSpace(c) {
			if !white {
				out = out + " "
			}
			white = true
		} else {
			out = out + string(c)
			white = false
		}
	}

	return out
}

func writeNewFile(path string, text []byte) error {
	_ = os.Remove(path)

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	_, err = file.Write(text)
	if err != nil {
		return err
	}

	err = file.Close()
	if err != nil {
		return err
	}

	return nil
}
