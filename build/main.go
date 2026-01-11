package main

import (
	"os"
	"os/exec"

	"github.com/goyek/goyek/v2"
)

var vet = goyek.Define(goyek.Task{
	Name:  "vet",
	Usage: "Run go vet on all packages",
	Action: func(a *goyek.A) {
		cmd := exec.Command("go", "vet", "./...")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			a.Error(err)
		}
	},
})

func main() {
	goyek.Main(os.Args[1:])
}
