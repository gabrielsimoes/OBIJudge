package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	ENV []string = []string{ENV_HOME, ENV_PATH}
)

type Submission struct {
	SID  string
	When time.Time
	Task *TaskData
	Code []byte
	Lang Language
	Key  []byte
}

type TaskVerdict struct {
	SID       string
	When      time.Time
	TaskTitle string
	TaskName  string
	Code      string
	Lang      Language

	Score       int
	Batches     []BatchVerdict
	Compilation int
	Error       bool
	Extra       string
}

type BatchVerdict struct {
	Result int
	Score  int
	Time   time.Duration
	Memory int64
	Extra  string
}

type Judge struct {
	NumWorkers        int
	DB                *Database
	SubmissionChannel chan<- Submission
	VerdictChannel    <-chan TaskVerdict

	workers []*judgeWorker
}

func (j *Judge) Start() {
	subChan := make(chan Submission, 100)
	verChan := make(chan TaskVerdict, 100)

	j.SubmissionChannel = subChan
	j.VerdictChannel = verChan

	for id := 0; id < j.NumWorkers; id++ {
		worker := &judgeWorker{
			id:                id,
			db:                j.DB,
			submissionChannel: subChan,
			verdictChannel:    verChan,
		}

		j.workers = append(j.workers, worker)

		worker.start()
	}
}

func (j *Judge) Stop() {
	for _, worker := range j.workers {
		worker.stop()
	}

	close(j.SubmissionChannel)
}

type judgeWorker struct {
	id                int
	db                *Database
	submissionChannel <-chan Submission
	verdictChannel    chan<- TaskVerdict

	stopChannel chan bool
}

func (w *judgeWorker) start() {
	w.stopChannel = make(chan bool)
	go func() {
		for {
			select {
			case <-w.stopChannel:
				return
			case s := <-w.submissionChannel:
				verdict := w.judge(s)

				verdict.SID = s.SID
				verdict.When = s.When
				verdict.TaskTitle = s.Task.Title
				verdict.TaskName = s.Task.Name
				verdict.Code = string(s.Code)
				verdict.Lang = s.Lang

				fmt.Printf("%+v\n", verdict)
				w.verdictChannel <- verdict
			}
		}
	}()
}

func (w *judgeWorker) stop() {
	w.stopChannel <- true
}

func (w *judgeWorker) judge(s Submission) TaskVerdict {
	box, err := Sandbox(w.id)
	if err != nil {
		return TaskVerdict{Error: true, Extra: err.Error()}
	}
	defer box.Clear()

	err = s.Lang.CopyExtraFiles(box.BoxPath)
	if err != nil {
		return TaskVerdict{Error: true, Extra: err.Error()}
	}

	err = writeNewFile(filepath.Join(box.BoxPath, "box", s.Task.Name+s.Lang.SourceExtension()), s.Code)
	if err != nil {
		return TaskVerdict{Error: true, Extra: err.Error()}
	}

	compilationCommand := s.Lang.CompilationCommand([]string{s.Task.Name + s.Lang.SourceExtension()}, s.Task.Name)
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
			return TaskVerdict{Compilation: RESULT_COMP_TIMEOUT}
		} else if compilationResult.Status == STATUS_SIG {
			return TaskVerdict{Compilation: RESULT_COMP_SIGNAL, Extra: strconv.Itoa(int(compilationResult.Signal))}
		} else if compilationResult.Status == STATUS_EXT {
			return TaskVerdict{Compilation: RESULT_COMP_FAILED, Extra: strconv.Itoa(compilationResult.ExitCode) + "\n" + compilationOutput.String()}
		} else if compilationResult.Status == STATUS_OK {
			ret.Compilation = RESULT_COMP_SUCCESS
		}
	}

	tests, err := w.db.Tests(s.Task.Name, s.Key)
	if err != nil {
		return TaskVerdict{Error: true, Extra: err.Error()}
	}

	if len(s.Task.Batches) == 0 {
		tests := make([]int, s.Task.NTests)
		for i := 0; i < s.Task.NTests; i++ {
			tests[i] = i
		}
		s.Task.Batches = []BatchData{{100, tests}}
	}

	results := make([]struct {
		code   int
		extra  string
		time   time.Duration
		memory int64
	}, len(tests))

	ret.Batches = make([]BatchVerdict, len(s.Task.Batches))

	for batchNumber, batch := range s.Task.Batches {
		ret.Batches[batchNumber].Result = RESULT_CORRECT

		for _, i := range batch.Tests {
			test := tests[i]
			if results[i].code == RESULT_NOTHING {
				command := s.Lang.EvaluationCommand(s.Task.Name, nil)
				var output bytes.Buffer
				result := box.Run(&BoxConfig{
					Path:          command[0],
					Args:          command,
					Env:           ENV,
					Stdin:         bytes.NewReader(test.Input),
					Stdout:        &output,
					EnableCgroups: true,
					MemoryLimit:   int64(s.Task.MemoryLimit),
					CPUTimeLimit:  time.Duration(s.Task.TimeLimit) * time.Millisecond,
					WallTimeLimit: time.Duration(s.Task.TimeLimit) * time.Millisecond,
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
