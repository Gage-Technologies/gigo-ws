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
user_input = input("Please enter some data: \n")
print(f"You entered: {user_input}")`

const goScript = `package main

import (
	"os"
	"bufio"
	"fmt"
)

func main() {
	fmt.Println("Waiting input")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	fmt.Println(input)
}`

func main() {
	stdout := make(chan string)
	stderr := make(chan string)

	go func() {
		for {
			select {
			case l := <-stdout:
				os.Stdout.Write([]byte(l + "\n"))
			case l := <-stderr:
				os.Stderr.Write([]byte(l + "\n"))
			}
		}
	}()

	cmd, err := core.ExecCode(context.TODO(), pythonScript, models.Python, slog.Make(sloghuman.Sink(os.Stdout)))
	if err != nil {
		panic(err)
	}

	go func() {
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		cmd.Stdin.Write([]byte(input))
	}()

	for r := range cmd.ResponseChan {
		buf, _ := json.MarshalIndent(r, "", "  ")
		fmt.Println(string(buf))
	}
}
