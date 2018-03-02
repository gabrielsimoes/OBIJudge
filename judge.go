package main

import (
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unicode"
)

type Verdict struct {
	Result []int
	Score  int
}

const (
	NO int = 0
	AC int = 1
	WA int = 2
	ML int = 3
	TL int = 4
	RE int = 5
	CE int = 6
	RV int = 7
	ER int = 8
)

func getCodeText(code int) string {
	switch code {
	case NO:
		return "-"
	case AC:
		return "Resposta correta."
	case WA:
		return "Resposta incorreta."
	case ML:
		return "Limite de memória excedido."
	case TL:
		return "Tempo limite excedido."
	case RE:
		return "Erro em tempo de execução."
	case CE:
		return "Erro de compilação."
	case RV:
		return "Violação de recursos permitidos."
	case ER:
		return "ERRO!"
	default:
		return "ERRO!"
	}
}

func (v Verdict) Text() string {
	if len(v.Result) == 0 {
		return "-"
	} else if len(v.Result) == 1 {
		return getCodeText(v.Result[0])
	} else {
		var text string
		for i, code := range v.Result {
			if i > 0 {
				text += "\n"
			}
			text += "Conjunto " + strconv.Itoa(i) + ": " + getCodeText(code)
		}
		return text
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

func judge(task *TaskData, db *database, key []byte, code []byte, lang string) (Verdict, error) {
	// load language runner
	if _, ok := runnerRegistry[lang]; !ok {
		err := errors.New("Language " + lang + " doesn't have a runner!")
		return Verdict{[]int{ER}, 0}, err
	}
	r := runnerRegistry[lang]

	// temporary directory and files
	tmpdir, err := ioutil.TempDir("", "obijudge")
	if err != nil {
		return Verdict{[]int{ER}, 0}, err
	}
	defer os.RemoveAll(tmpdir)

	err = writeNewFile(tmpdir+"/"+r.sourceName(task.Name), code)
	if err != nil {
		return Verdict{[]int{ER}, 0}, err
	}

	err = r.prepare(tmpdir, task.Name)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return Verdict{[]int{CE}, 0}, nil
		} else {
			return Verdict{[]int{ER}, 0}, err
		}
	}

	tests, err := db.getTests(task.Name, key)
	if err != nil {
		return Verdict{[]int{ER}, 0}, err
	}

	if len(task.Batches) == 0 {
		tests := make([]int, task.NTests)
		for i := 0; i < task.NTests; i++ {
			tests[i] = i
		}
		task.Batches = []BatchData{{100, tests}}
	}

	results := make([]int, len(tests))
	ret := Verdict{make([]int, len(task.Batches)), 0}
	for batchix, batch := range task.Batches {
		var ok bool = true

		for _, i := range batch.Tests {
			test := tests[i]
			if results[i] == NO {
				err = writeNewFile(tmpdir+"/input", test.Input)
				if err != nil {
					return Verdict{[]int{ER}, 0}, err
				}
				err = writeNewFile(tmpdir+"/output", []byte{})
				if err != nil {
					return Verdict{[]int{ER}, 0}, err
				}

				results[i] = r.run(tmpdir, task.Name, task.TimeLimit, task.MemoryLimit)

				if results[i] == AC {
					answer, err := ioutil.ReadFile(tmpdir + "/output")
					if err != nil {
						return Verdict{[]int{ER}, 0}, err
					}

					// fmt.Println("Output: ", strip(string(test.Output)))
					// fmt.Println("Answer: ", strip(string(answer)))

					if strings.Compare(strip(string(answer)), strip(string(test.Output))) != 0 {
						results[i] = WA
					}
				}
			}

			if results[i] != AC {
				ok = false
				ret.Result[batchix] = results[i]
				break
			}
		}

		if ok {
			ret.Result[batchix] = AC
			ret.Score += batch.Value
		}
	}

	return ret, nil
}
