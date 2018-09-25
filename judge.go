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
	ResultNothing int = iota
	ResultTimeout
	ResultSignal
	ResultFailed
	ResultCorrect
	ResultWrong
)

const (
	ResultCompNothing int = iota
	ResultCompTimeout
	ResultCompSignal
	ResultCompFailed
	ResultCompSuccess
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
	DB   *Database
	Key  []byte
}

type CustomTest struct {
	ID       uint32
	SID      string
	When     time.Time
	TaskName string
	Input    []byte
	Code     []byte
	Lang     Language
}

type TaskVerdict struct {
	VerdictInfo
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

type CustomTestVerdict struct {
	VerdictInfo
	Compilation int
	Result      int
	Time        time.Duration
	Memory      int64
	Output      string
	Error       bool
	Extra       string
}

type VerdictInfo struct {
	ID       uint32
	SID      string
	When     time.Time
	TaskName string
	Code     string
	LangMime string
	LangName string
}

type Judge struct {
	NumWorkers         int
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
				verdict.LangMime = s.Lang.MimeType()
				verdict.LangName = s.Lang.Name()

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
				verdict.LangMime = t.Lang.MimeType()
				verdict.LangName = t.Lang.Name()

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
	if compilationCommand == nil {
		return true, ResultCompSuccess, ""
	}

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
		MemoryLimit:   25 << 19, // 2.5GB
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

	if result.Status == StatusError {
		return false, 0, result.Error
	} else {
		if result.Status == StatusWTL || result.Status == StatusCTL {
			return true, ResultCompTimeout, ""
		} else if result.Status == StatusSig {
			return true, ResultCompSignal, result.Signal.String()
		} else if result.Status == StatusExit {
			return true, ResultCompFailed, "Exit Code: " + strconv.Itoa(result.ExitCode) + "\n" + string(output)
		} else if result.Status == StatusOK {
			return true, ResultCompSuccess, ""
		}
	}

	return true, ResultCompSuccess, ""
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
	} else if compilationResult != ResultCompSuccess {
		return TaskVerdict{Compilation: compilationResult, Extra: compilationExtra}
	}

	var ret TaskVerdict
	ret.Compilation = ResultCompSuccess

	tests, err := s.DB.Tests(s.Task.Name, s.Key)
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
		ret.Batches[batchNumber].Result = ResultCorrect

		for _, i := range batch.Tests {
			test := tests[i]
			if results[i].code == ResultNothing {
				command := s.Lang.EvaluationCommand(s.Task.Name, nil, s.Task.MemoryLimit)

				outputFile, err := os.Create(filepath.Join(box.BoxPath, "box", ".output"))
				if err != nil {
					return TaskVerdict{Error: true, Extra: err.Error()}
				}

				boxConfig := &BoxConfig{
					Path:          command[0],
					Args:          command,
					Env:           ENV,
					Stdin:         bytes.NewReader(test.Input),
					Stdout:        outputFile,
					Stderr:        outputFile,
					EnableCgroups: true,
					CPUTimeLimit:  time.Duration(s.Task.TimeLimit) * time.Millisecond,
					WallTimeLimit: time.Duration(s.Task.TimeLimit) * time.Millisecond,
				}

				if s.Lang.UseMemoryLimit() {
					boxConfig.MemoryLimit = int64(s.Task.MemoryLimit)
				}

				result := box.Run(boxConfig)

				outputFile.Close()
				output, err := ioutil.ReadFile(filepath.Join(box.BoxPath, "box", ".output"))

				if testingFlag {
					fmt.Printf("Test %d output: %s\n", i, string(output))
				}

				if err != nil {
					return TaskVerdict{Error: true, Extra: err.Error()}
				}

				if testingFlag {
					fmt.Printf("Test %d: %+v\n", i, result)
				}

				if result.Status == StatusError {
					return TaskVerdict{Error: true, Extra: result.Error}
				} else {
					results[i].time = result.CPUTime
					results[i].memory = result.Memory

					if result.Status == StatusWTL || result.Status == StatusCTL {
						results[i].code = ResultTimeout
					} else if result.Status == StatusSig {
						results[i].code = ResultSignal
						results[i].extra = result.Signal.String()
					} else if result.Status == StatusExit {
						results[i].code = ResultFailed
						results[i].extra = "Exit Code: " + strconv.Itoa(result.ExitCode)
					} else if result.Status == StatusOK {
						results[i].code = ResultCorrect
					}
				}

				if results[i].code == ResultCorrect {
					if strings.Compare(strip(string(output)), strip(string(test.Output))) != 0 {
						results[i].code = ResultWrong
					}
				}
			}

			if results[i].time > ret.Batches[batchNumber].Time {
				ret.Batches[batchNumber].Time = results[i].time
			}

			if results[i].memory > ret.Batches[batchNumber].Memory {
				ret.Batches[batchNumber].Memory = results[i].memory
			}

			if results[i].code != ResultCorrect {
				ret.Batches[batchNumber].Result = results[i].code
				ret.Batches[batchNumber].Extra = results[i].extra
				break
			}
		}

		if ret.Batches[batchNumber].Result == ResultCorrect {
			ret.Batches[batchNumber].Score = batch.Value
		}
	}

	return ret
}

func (w *judgeWorker) test(t CustomTest) CustomTestVerdict {
	box, err := w.prepare(t.Lang, t.Code, t.TaskName+t.Lang.SourceExtension())
	if err != nil {
		return CustomTestVerdict{Error: true, Extra: err.Error()}
	}
	defer box.Clear()

	compilationCommand := t.Lang.CompilationCommand([]string{t.TaskName + t.Lang.SourceExtension()}, t.TaskName)

	ok, compilationResult, compilationExtra := w.compile(box, compilationCommand)
	if !ok {
		return CustomTestVerdict{Error: true, Extra: compilationExtra}
	} else if compilationResult != ResultCompSuccess {
		return CustomTestVerdict{Compilation: compilationResult, Extra: compilationExtra}
	}

	var ret CustomTestVerdict
	ret.Compilation = ResultCompSuccess

	command := t.Lang.EvaluationCommand(t.TaskName, nil, 25<<19) // 2.5GB

	outputFile, err := os.Create(filepath.Join(box.BoxPath, "box", ".output"))
	if err != nil {
		return CustomTestVerdict{Error: true, Extra: err.Error()}
	}

	boxConfig := &BoxConfig{
		Path:          command[0],
		Args:          command,
		Env:           ENV,
		Stdin:         bytes.NewReader(t.Input),
		Stdout:        outputFile,
		EnableCgroups: true,
		CPUTimeLimit:  2 * time.Minute,
		WallTimeLimit: 2 * time.Minute,
	}

	if t.Lang.UseMemoryLimit() {
		boxConfig.MemoryLimit = 25 << 19 // 2.5GB
	}

	result := box.Run(boxConfig)

	outputFile.Close()
	output, err := ioutil.ReadFile(filepath.Join(box.BoxPath, "box", ".output"))

	if err != nil {
		return CustomTestVerdict{Error: true, Extra: err.Error()}
	}

	if len(output) > 1024 {
		ret.Output = string(output[:1024]) + "\n\n(...)"
	} else {
		ret.Output = string(output)
	}

	if result.Status == StatusError {
		return CustomTestVerdict{Error: true, Extra: result.Error}
	}

	ret.Time = result.CPUTime
	ret.Memory = result.Memory

	if result.Status == StatusWTL || result.Status == StatusCTL {
		ret.Result = ResultTimeout
	} else if result.Status == StatusSig {
		ret.Result = ResultSignal
		ret.Extra = result.Signal.String()
	} else if result.Status == StatusExit {
		ret.Result = ResultFailed
		ret.Extra = "Exit Code: " + strconv.Itoa(result.ExitCode)
	} else if result.Status == StatusOK {
		ret.Result = ResultCorrect
	}

	return ret
}
