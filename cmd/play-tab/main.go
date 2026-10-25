// Command play-tab synthesizes a guitar tab stored in a YAML file.
//
// Usage:
//
//	play-tab [-o output.wav] tab.yaml
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adambezecny/digitar/tablature"
)

func main() {
	output := flag.String("o", "", "output WAV file (default: <tab name>.wav)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: play-tab [-o output.wav] tab.yaml")
		os.Exit(2)
	}

	path := flag.Arg(0)
	if *output == "" {
		name := filepath.Base(path)
		*output = strings.TrimSuffix(name, filepath.Ext(name)) + ".wav"
	}

	if err := tablature.Play(path, *output); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
