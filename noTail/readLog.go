package main

import (
	"log"
	"os"
	"io"
	"bufio"
	"time"
)

func tailFileOps(file string, lines chan<- string) {
	ofile, err := os.Open(file)
	if err != nil {
		log.Fatalf("Error opening file %q", file)
	}
	defer ofile.Close()
	r := bufio.NewReader(ofile)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				infoLine("readingLog -> snoozing ", *cmdTime, " seconds")
				time.Sleep(time.Second * *cmdTime)
				infoLine("readingLog -> waking up!")
				continue
			}
			log.Fatalf("Error reading file %q", err)
		}
		lines <- line
	}
}

func main() {
	// to be deleted
	flag.Parse()
	if
}
