package main

import (
	"bufio"
	"flag"
	"io"
	"log"
	"os"
	"time"
)

var (
	fFile = flag.String("file", "", "File to be read")
)

func followFile(file string) string {
	// open a file and return lines
	// but returning would end the function?
	// idk
	// NOTE: this works currently.
	// BUT -> what if a goroutine were used, which would pass lines to a channel,
	// where another function parses or does something or something something
	f, err := os.Open(file)
	if err != nil {
		log.Fatalf("Error opening %q. Error: %q", file, err)
	}
	defer f.Close()
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				log.Println("Hit EOF")
				time.Sleep(5 * time.Second)
				log.Println("I'm awake! Reading . . .")
				continue
			}
			log.Fatalf("Some other error here: %q", err)
		}
		log.Println("Line: ", line)
	}
}

func main() {
	flag.Parse()
	if *fFile == "" {
		log.Println("Args expected! Nothing to do")
		return
	}
	followFile(*fFile)
}
