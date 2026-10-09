// Command apic runs .http request files for humans and AI agents.
package main

import (
	"os"

	"github.com/hungovercoders/apic/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
