package main

import (
	"os"

	"gonv/internal/shim"
)

func main() {
	os.Exit(shim.Run())
}
