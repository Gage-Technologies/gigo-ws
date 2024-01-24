package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"gigo-ws/coder/agent/agent/server/core"
	"os"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/gage-technologies/gigo-lib/db/models"
)

const pythonScript = `# your_script.py
import time
print("Sleeping 5s")
time.sleep(5)
user_input = input("Please enter some data: ")
print(f"You entered: {user_input}")`

const goScript = `package main

import (
	"os"
	"bufio"
	"fmt"
)

func main() {
	fmt.Printf("Waiting input: ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	fmt.Println(input)
}`

func main() {
	cmd, err := core.ExecCode(context.TODO(), pythonScript, models.Python, slog.Make(sloghuman.Sink(os.Stdout)))
	if err != nil {
		panic(err)
	}

	go func() {
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		cmd.Stdin.Write([]byte(input))
	}()

	stdoutIdx := 0
	stderrIdx := 0
	for r := range cmd.ResponseChan {
		fmt.Println("\n-------------------\n")
		buf, _ := json.Marshal(r)
		fmt.Println(string(buf))
		fmt.Println("\n-------------------\n")
		if len(r.StdOut) > stdoutIdx {
			for _, l := range r.StdOut[stdoutIdx:] {
				os.Stdout.WriteString(l.Content)
			}
			stderrIdx = len(r.StdOut)
		}
		if len(r.StdErr) > stderrIdx {
			for _, l := range r.StdErr[stderrIdx:] {
				os.Stderr.WriteString(l.Content)
			}
			stderrIdx = len(r.StdErr)
		}
	}
}
