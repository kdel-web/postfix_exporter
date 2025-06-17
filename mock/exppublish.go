package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

const (
	postNoQueue  = "NOQUEUE"
	postSmtpd    = "postfix/smtpd"
	postSmtp     = "postfix/smtp"
	postFast     = "postfix-fast/smtp"
	postSlow     = "postfix-slow/smtp"
	postMed      = "postfix-medium/smtp"
	postRes      = "postfix-restrictive/smtp"
	postClean    = "postfix/cleanup"
	postDiscard  = "postfix/discard"
	postQmgr     = "postfix/qmgr"
	postSent     = "status=sent"
	postDefer    = "status=deferred"
	postBounce   = "status=bounced"
	postOpendkim = "opendkim"
)

var (
	ctx    = context.Background()
	fFile  = flag.String("file", "", "File to read from")
	fInfo  = flag.Bool("debug", false, "Enable arbitrary info lines printed to stdout")
	fClose = flag.Bool("close", false, "Close file and exit program instead of tailing")
)

func init() {
	flag.Parse()
	if *fFile == "" {
		log.Fatalln("Expected `file` not provided. Nothing to do!")
	}
}

func main() {
	infoLine("Debug printing enabled")
	var wg sync.WaitGroup
	ctx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	lines := make(chan string)
	go fileRunner(ctx, *fFile, lines)

	wg.Add(1)
	go func() {
		defer wg.Done()
		infoLine("Call consumer")
		for line := range lines {
			fmt.Println(line)
		}
	}()
	wg.Wait()
}

// log line producer
// tails (follows / sleeps ) opened file and
// sends log lines through channel
func fileRunner(ctx context.Context, file string, lines chan<- string) {
	if ctx.Err() != nil {
		return
	}
	ofile, err := os.Open(file)
	if err != nil {
		log.Fatalf("Error opening file: %q", file)
	}
	defer ofile.Close()

	r := bufio.NewReader(ofile)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				infoLine("fileRunner: EOF")
				if *fClose {
					infoLine("`close` provided, cleaning up and exiting")
					close(lines)
					return
				}
				time.Sleep(5 * time.Second)
				continue
			}
			log.Fatalf("fileRunner: Error -> %q", err)
		}
		lines <- line
	}
}

//func maillogWorker()

func infoLine(a ...any) {
	if *fInfo {
		fmt.Println("Info Line ->: ", a)
	}
}
