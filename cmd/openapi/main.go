// Command openapi prints the OpenAPI spec to stdout (see `make openapi`).
package main

import (
	"fmt"
	"os"

	"tradepulse/internal/api"
)

func main() {
	b, err := api.OpenAPISpec()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}
