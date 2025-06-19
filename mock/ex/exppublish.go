package main

// This program tails the default log text file for Postfix SMTP server,
// and parses the log lines via regex patterns to expose message counting stastics
// via an http endpoint.

import (
	"bufio"
	"context"
	"expvar"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	TESTFILE = "/home/kdellinger/postfix_exporter/outputs/BogusAmalgamTest.log"
	fFile    = flag.String("file", TESTFILE, "File to read from")
	fInfo    = flag.Bool("debug", false, "Enable arbitrary info lines printed to stdout")
	fClose   = flag.Bool("close", false, "Close file and exit program instead of tailing")
	fPort    = flag.String("listen-address", ":9004", "HTTP listen address for expvar")

	// mostly troubleshooting / debugging but perhaps useful otherwise
	//fSleep      = flag.Duration("s", 5, "Time to sleep between message counter prints")
	fAllIgnored = flag.Bool("ignored", false, "Enable printing of ignored lines to stdout")
	fNotIgnored = flag.Bool("print-all", false, "Enable printing of all matched lines stdout")
	fSMTPD      = flag.Bool("print-inbound", false, "Enable printing of all inbound lines to stdout")
	fSMTP       = flag.Bool("print-outbound", false, "Enable printing of all outbound lines to stdout")
	fBounce     = flag.Bool("print-bounced", false, "Enable printing of all bounced lines to stdout")
	fSent       = flag.Bool("print-sends", false, "Enable printing of sent messages to stdout")
	fDefer      = flag.Bool("print-defers", false, "Enable printing of deferred messages to stdout")
	fNoQ        = flag.Bool("print-noqueues", false, "Enable printing of no queues to stdout")

	newMsgLineMatch = regexp.MustCompile(`(opendkim|postfix(?:-slow)?(?:-fast)?(?:-medium)?(?:-restrictive)?\/(?:smtpd?|scache|cleanup|qmgr|bounce|error|warning|fatal|panic|discard)?)`)
	msgDelaysMatch  = regexp.MustCompile(`delays=([0-9.?\/]+)\,`)
	msgIDMatch      = regexp.MustCompile(`\s([A-F0-9]{6,}):`)
	emailAddrMatch  = regexp.MustCompile(`<(.*?@?.*?)>:`)
)

func init() {
	flag.Parse()
	if *fFile == "" {
		log.Fatalln("Expected `file` not provided. Nothing to do!")
	}
}

func main() {
	infoLine("Debug printing enabled")
	var (
		wg sync.WaitGroup
	)

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	// perhaps this is not even needed since it doesn't seem to function any differently with or without it

	lines := make(chan string)
	MCount := NewMessageCounter()
	MDetails := NewMessageDetails()
	expvar.Publish("No Queue Addrs", MDetails)

	wg.Add(1)
	go func() {
		defer wg.Done()
		infoLine("Call runner")
		fileRunner(ctx, *fFile, lines)
		// this seeming poor design stems from not comprehending context
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		infoLine("Call consumer")
		maillogWorker(ctx, MCount, MDetails, lines)
	}()

	//http.Handle("/debug/expvars", expvar.Handler())
	// "only needed if adjusting the path" or something

	wg.Add(1)
	go func() {
		defer wg.Done()
		infoLine("Start http server")
		err := http.ListenAndServe(*fPort, nil)
		if err != nil {
			log.Fatalln("Error starting HTTP server", err)
		}
	}()

	wg.Wait()
}

// log line producer
// tails (follows / sleeps ) opened file and
// sends log lines through channel
func fileRunner(ctx context.Context, file string, lines chan<- string) {
	if ctx.Err() != nil {
		infoLine("fileRunner ctx err")
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
					// both the close and the return are required for proper functionality
					// I think the close is needed so the consumer can exit
					// and I think the return is needed bc the waitgroup closure
				}
				time.Sleep(5 * time.Second)
				continue
			}
			log.Fatalf("fileRunner: Error -> %q", err)
		}
		lines <- line
	}
}

// log line consumer
// receives log lines and parses them, providing counts for message stats
func maillogWorker(ctx context.Context, m *MessageCounter, d *MessageDetails, lines <-chan string) {
	if ctx.Err() != nil {
		infoLine("maillogWorker ctx err")
		return
	}
	for line := range lines {
		if line != "" {
			lineMatch := newMsgLineMatch.FindStringSubmatch(line)
			if lineMatch != nil {
				if *fNotIgnored {
					fmt.Print("Matched Line ->:", line)
				}
				switch lineMatch[1] {
				case postSmtpd:
					if *fSMTPD {
						fmt.Print("SMTPD Line ->:", line)
					}
					if strings.Contains(line, postNoQueue) {
						if *fNoQ {
							fmt.Print("No Queue Line ->:", line)
						}
						m.No_queue.Add(1)
						if noq_addr := emailAddrMatch.FindStringSubmatch(line); noq_addr != nil {
							//fmt.Println("No Queues: ", noq_addr[1])
							d.addNoQueue(noq_addr[1])

						}
					} else if msgIDMatch.MatchString(line) {
						m.Accepted_in.Add(1)
						// do something with queue id
					} else {
						if *fSMTPD || *fAllIgnored {
							fmt.Print("SMTPD Ignored Line ->:", line)
						}
					}
				case postSmtp, postFast, postSlow, postMed, postRes:
					if *fSMTP {
						fmt.Print("SMTP Line ->:", line)
					}
					switch {
					case strings.Contains(line, postSent):
						m.Sent.Add(1)
						// queue id? addr?
						if *fSent {
							fmt.Print("Sent Message ->:", line)
						}
					case strings.Contains(line, postDefer):
						m.Deferred_tries.Add(1)
						// get queue id and address, etc
						// and use addDefer method to count
						if *fDefer {
							fmt.Print("Deferred Message ->:", line)
						}
					case strings.Contains(line, postBounce):
						m.Bounced.Add(1)
						// get queue id and address, etc
						if *fBounce {
							fmt.Print("Bounced Message ->:", line)
						}
					default:
						if *fSMTP || *fAllIgnored {
							fmt.Print("SMTP Ignored ->:", line)
						}
					}

				}
			} else {
				if *fAllIgnored {
					fmt.Print("Unmatched & Ignored Line ->:", line)
				}
			}
		} else {
			// will never print
			infoLine("maillogWorker done")
			return
		}
	}
}
