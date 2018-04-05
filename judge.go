package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
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
	ID   uint32
	SID  string
	When time.Time
	Task *TaskData
	Code []byte
	Lang Language
	Key  []byte
}

type TaskVerdict struct {
	VerdictInfo
	Score       int
	Compilation int
	Batches     []BatchVerdict
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

type CustomTest struct {
	ID    uint32
	SID   string
	When  time.Time
	Input []byte
	Code  []byte
	Lang  Language
}

type CustomTestVerdict struct {
	VerdictInfo
	Compilation int
	Result      int
	Time        time.Duration
	Memory      int64
	Error       bool
	Extra       string
}

type VerdictInfo struct {
	ID       uint32
	SID      string
	When     time.Time
	TaskName string
	Code     string
	Lang     Language
}

type Judge struct {
	NumWorkers         int
	DB                 *Database
	SubmissionChannel  chan<- Submission
	TaskVerdictChannel <-chan TaskVerdict
	TestChannel        chan<- CustomTest
	TestVerdictChannel <-chan CustomTestVerdict

	subID   uint32
	testID  uint32
	workers []*judgeWorker
}

func (j *Judge) Start() {
	submissionChannel := make(chan Submission, 100)
	taskVerdictChannel := make(chan TaskVerdict, 100)
	testChannel := make(chan CustomTest, 100)
	testVerdictChannel := make(chan CustomTestVerdict, 100)

	j.SubmissionChannel = submissionChannel
	j.TaskVerdictChannel = taskVerdictChannel
	j.TestChannel = testChannel
	j.TestVerdictChannel = testVerdictChannel

	for id := 0; id < j.NumWorkers; id++ {
		worker := &judgeWorker{
			id:                 id,
			db:                 j.DB,
			submissionChannel:  submissionChannel,
			taskVerdictChannel: taskVerdictChannel,
			testChannel:        testChannel,
			testVerdictChannel: testVerdictChannel,
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
	close(j.TestChannel)
}

func (j *Judge) SendSubmission(s Submission) uint32 {
	s.ID = atomic.AddUint32(&j.subID, 1)
	j.SubmissionChannel <- s
	return s.ID
}

func (j *Judge) SendCustomTest(t CustomTest) uint32 {
	t.ID = atomic.AddUint32(&j.testID, 1)
	j.TestChannel <- t
	return t.ID
}

type judgeWorker struct {
	id                 int
	db                 *Database
	submissionChannel  <-chan Submission
	taskVerdictChannel chan<- TaskVerdict
	testChannel        <-chan CustomTest
	testVerdictChannel chan<- CustomTestVerdict
	stopChannel        chan bool
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

				verdict.ID = s.ID
				verdict.SID = s.SID
				verdict.When = s.When
				verdict.TaskName = s.Task.Name
				verdict.Code = string(s.Code)
				verdict.Lang = s.Lang

				if testingFlag {
					fmt.Printf("%+v\n\n", verdict)
				}
				w.taskVerdictChannel <- verdict
			case t := <-w.testChannel:
				verdict := w.test(t)

				verdict.ID = t.ID
				verdict.SID = t.SID
				verdict.When = t.When
				verdict.TaskName = "_test"
				verdict.Code = string(t.Code)
				verdict.Lang = t.Lang

				if testingFlag {
					fmt.Printf("%+v\n\n", verdict)
				}
				w.testVerdictChannel <- verdict
			}
		}
	}()
}

func (w *judgeWorker) stop() {
	w.stopChannel <- true
}

func (w *judgeWorker) prepare(lang Language, source []byte, sourceFilename string) (*Box, error) {
	box, err := Sandbox(w.id)
	if err != nil {
		return nil, err
	}

	err = lang.CopyExtraFiles(box.BoxPath)
	if err != nil {
		return nil, err
	}

	err = ioutil.WriteFile(filepath.Join(box.BoxPath, "box", sourceFilename), source, 0666)
	if err != nil {
		return nil, err
	}

	return box, nil
}

func (w *judgeWorker) compile(box *Box, compilationCommand []string) (bool, int, string) {
	outputFile, err := os.Create(filepath.Join(box.BoxPath, "box", ".output"))
	if err != nil {
		return true, 0, err.Error()
	}

	result := box.Run(&BoxConfig{
		Path:          compilationCommand[0],
		Args:          compilationCommand,
		Env:           ENV,
		Stdout:        outputFile,
		Stderr:        outputFile,
		EnableCgroups: true,
		MemoryLimit:   1 << 20, // 1GB
		CPUTimeLimit:  2 * time.Minute,
		WallTimeLimit: 2 * time.Minute,
	})

	outputFile.Close()
	output, err := ioutil.ReadFile(filepath.Join(box.BoxPath, "box", ".output"))

	if err != nil {
		return true, 0, result.Error
	}

	if testingFlag {
		fmt.Printf("Compilation: %+v %s\n", result, string(output))
	}

	if result.Status == STATUS_ERR {
		return false, 0, result.Error
	} else {
		if result.Status == STATUS_WTL || result.Status == STATUS_CTL {
			return true, RESULT_COMP_TIMEOUT, ""
		} else if result.Status == STATUS_SIG {
			return true, RESULT_COMP_SIGNAL, result.Signal.String()
		} else if result.Status == STATUS_EXT {
			return true, RESULT_COMP_FAILED, "Exit Code: " + strconv.Itoa(result.ExitCode) + "\n" + string(output)
		} else if result.Status == STATUS_OK {
			return true, RESULT_COMP_SUCCESS, ""
		}
	}

	return true, RESULT_COMP_SUCCESS, ""
}

func (w *judgeWorker) judge(s Submission) TaskVerdict {
	box, err := w.prepare(s.Lang, s.Code, s.Task.Name+s.Lang.SourceExtension())
	if err != nil {
		return TaskVerdict{Error: true, Extra: err.Error()}
	}
	defer box.Clear()

	compilationCommand := s.Lang.CompilationCommand([]string{s.Task.Name + s.Lang.SourceExtension()}, s.Task.Name)

	ok, compilationResult, compilationExtra := w.compile(box, compilationCommand)
	if !ok {
		return TaskVerdict{Error: true, Extra: compilationExtra}
	} else if compilationResult != RESULT_COMP_SUCCESS {
		return TaskVerdict{Compilation: compilationResult, Extra: compilationExtra}
	}

	var ret TaskVerdict
	ret.Compilation = RESULT_COMP_SUCCESS

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

				outputFile, err := os.Create(filepath.Join(box.BoxPath, "box", ".output"))
				if err != nil {
					return TaskVerdict{Error: true, Extra: err.Error()}
				}

				result := box.Run(&BoxConfig{
					Path:          command[0],
					Args:          command,
					Env:           ENV,
					Stdin:         bytes.NewReader(test.Input),
					Stdout:        outputFile,
					EnableCgroups: true,
					MemoryLimit:   int64(s.Task.MemoryLimit),
					CPUTimeLimit:  time.Duration(s.Task.TimeLimit) * time.Millisecond,
					WallTimeLimit: time.Duration(s.Task.TimeLimit) * time.Millisecond,
				})

				outputFile.Close()
				output, err := ioutil.ReadFile(filepath.Join(box.BoxPath, "box", ".output"))

				if err != nil {
					return TaskVerdict{Error: true, Extra: err.Error()}
				}

				if testingFlag {
					fmt.Printf("Test %d: %+v\n", i, result)
				}

				if result.Status == STATUS_ERR {
					return TaskVerdict{Error: true, Extra: result.Error}
				} else {
					results[i].time = result.CPUTime
					results[i].memory = result.Memory

					if result.Status == STATUS_WTL || result.Status == STATUS_CTL {
						results[i].code = RESULT_TIMEOUT
					} else if result.Status == STATUS_SIG {
						results[i].code = RESULT_SIGNAL
						results[i].extra = result.Signal.String()
					} else if result.Status == STATUS_EXT {
						results[i].code = RESULT_FAILED
						results[i].extra = "Exit Code: " + strconv.Itoa(result.ExitCode)
					} else if result.Status == STATUS_OK {
						results[i].code = RESULT_CORRECT
					}
				}

				if results[i].code == RESULT_CORRECT {
					if strings.Compare(strip(string(output)), strip(string(test.Output))) != 0 {
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

func (w *judgeWorker) test(t CustomTest) CustomTestVerdict {
	box, err := w.prepare(t.Lang, t.Code, "test"+t.Lang.SourceExtension())
	if err != nil {
		return CustomTestVerdict{Error: true, Extra: err.Error()}
	}
	defer box.Clear()

	compilationCommand := t.Lang.CompilationCommand([]string{"test" + t.Lang.SourceExtension()}, "test")

	ok, compilationResult, compilationExtra := w.compile(box, compilationCommand)
	if !ok {
		return CustomTestVerdict{Error: true, Extra: compilationExtra}
	} else if compilationResult != RESULT_COMP_SUCCESS {
		return CustomTestVerdict{Compilation: compilationResult, Extra: compilationExtra}
	}

	var ret CustomTestVerdict
	ret.Compilation = RESULT_COMP_SUCCESS

	command := t.Lang.EvaluationCommand("test", nil)
	var output bytes.Buffer
	result := box.Run(&BoxConfig{
		Path:          command[0],
		Args:          command,
		Env:           ENV,
		Stdin:         bytes.NewReader(t.Input),
		Stdout:        &output,
		EnableCgroups: true,
		MemoryLimit:   2 << 20, // 2GB
		CPUTimeLimit:  2 * time.Minute,
		WallTimeLimit: 2 * time.Minute,
	})

	if result.Status == STATUS_ERR {
		return CustomTestVerdict{Error: true, Extra: result.Error}
	}

	ret.Time = result.CPUTime
	ret.Memory = result.Memory

	if result.Status == STATUS_WTL || result.Status == STATUS_CTL {
		ret.Result = RESULT_TIMEOUT
	} else if result.Status == STATUS_SIG {
		ret.Result = RESULT_SIGNAL
		ret.Extra = result.Signal.String()
	} else if result.Status == STATUS_EXT {
		ret.Result = RESULT_FAILED
		ret.Extra = "Exit Code: " + strconv.Itoa(result.ExitCode)
	} else if result.Status == STATUS_OK {
		ret.Result = RESULT_CORRECT
	}

	return ret
}
