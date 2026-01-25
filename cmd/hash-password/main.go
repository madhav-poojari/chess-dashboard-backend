package main

import (
	"fmt"
	"os"

	"github.com/madhava-poojari/dashboard-api/internal/utils"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run cmd/hash-password/main.go <password>")
		os.Exit(1)
	}
	hash, err := utils.HashPassword(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(hash)
}
