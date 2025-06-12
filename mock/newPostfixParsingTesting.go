package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
)

const (
	cMatch   = "matched"
	cNoMatch = "not matched"
)

var (
	targetFile   = flag.String("file", "", "File to regex through by line")
	targetPrint  = flag.Bool("v", false, "Print various messages to stdout")
	msgLineMatch = regexp.MustCompile(`(postfix|opendkim)(-slow|-fast|-medium|-restrictive)?\/(smtpd?|scache|cleanup|qmgr|bounce|error|warning|fatal|panic)`)

	// logLine = regexp.MustCompile(` ?(postfix|opendkim)(/(\w+))?\[\d+\]: ((?:(warning|error|fatal|panic): )?.*)`)
)

func countOne(k string, m map[string]int) {
	m[k]++
}

func vPrinter(a ...any) {
	if *targetPrint == true {
		fmt.Println(a)
	}
}

func main() {
	flag.Parse()
	if *targetFile == "" {
		fmt.Println("Args expected! Nothing to do")
		return
	}

	rCount := map[string]int{
		cMatch:   0,
		cNoMatch: 0,
	}

	f, err := os.Open(*targetFile)
	if err != nil {
		fmt.Printf("Thar be error here! %q\n", err)
		return
	}
	defer f.Close()

	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("Hit <EOF>")
				break // or return //  or not. . .
			}
			fmt.Printf("Thar she goes again! %q\n", err)
			return
		}
		matched := msgLineMatch.FindStringSubmatch(line)
		if matched != nil {
			vPrinter(matched)
			countOne(cMatch, rCount)
		} else {
			vPrinter(matched)
			countOne(cNoMatch, rCount)
		}
	}
	totallines := rCount[cMatch] + rCount[cNoMatch]
	fmt.Println("Total finds: ", rCount[cMatch])
	fmt.Println("Total lines read: ", totallines)
}
