package main

import (
	"fmt"
	"os"

	"github.com/teddymail/bagualu/internal/modules/subscription_output"
)

func main() {
	data, err := os.ReadFile("/tmp/bagualu-real-sub.txt")
	if err != nil {
		panic(err)
	}
	result, err := subscription_output.Parse(data, "local-validation")
	if err != nil {
		fmt.Printf("parse_error=%v nodes=%d skipped=%d\n", err, len(result.Nodes), len(result.Skipped))
		return
	}
	fmt.Printf("nodes=%d skipped=%d\n", len(result.Nodes), len(result.Skipped))
	for _, node := range result.Nodes {
		fmt.Printf("%s protocol=%s endpoint=%s:%d\n", node.Name, node.Protocol, node.Address, node.Port)
	}
}
