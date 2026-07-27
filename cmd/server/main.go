package main

import (
	"fmt"
	"os"

	"bb_erp_echo/internal/app"
)

func main() {
	erp, err := app.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := erp.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
