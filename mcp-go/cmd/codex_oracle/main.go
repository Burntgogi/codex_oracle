package main

import (
	"fmt"
	"os"

	"codex_oracle/internal/mcpserver"
)

func main() {
	server := mcpserver.New(mcpserver.Options{})
	if err := server.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "codex_oracle MCP server failed: %v\n", err)
		os.Exit(1)
	}
}
