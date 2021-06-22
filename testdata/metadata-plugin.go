package main

import (
	"flag"
	"fmt"
	plugin "github.com/gorpher/go-idpc-plugin"
	"os"
	"runtime"
)

func main() {
	n := flag.Int("exit-code", 0, "exit code")
	m := flag.String("metadata", "", "metadata")
	na := flag.String("name", "json", "name")
	t := flag.String("type", "metrics", "type")
	flag.Parse()
	args := flag.Args()
	if len(args) > 0 && args[0] == "version" {
		flag.CommandLine.Parse(args[1:])
		fmt.Printf("%s-%s-%s version 0.71.2 (rev 923088e2) [%s %s %s]", plugin.PLUGIN_PREFIX, *na, *t, runtime.GOOS, runtime.GOARCH, runtime.Version())
		os.Exit(*n)
	}
	fmt.Print(*m)
	os.Exit(*n)
}
