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
	"time"
)

var (
	DEFAULTFILE = "/home/kdellinger/postfix_exporter/outputs/BogusAmalgamTest.log"
	//DEFAULTFILE = "/var/log/maillog"
	fFile    = flag.String("file", DEFAULTFILE, "File to read from")
	fInfo    = flag.Bool("debug", false, "Enable arbitrary info lines printed to stdout")
	fClose   = flag.Bool("close", false, "Close file and exit program instead of tailing")
	fPort    = flag.String("listen-address", ":9004", "HTTP listen address for expvar")
	fCDefers = flag.Bool("defer-count", false, "Print total count of deferred (only valid with --close)")
	fCNoQs   = flag.Bool("noq-count", false, "Print no queue email addresses (only valid with --close)")

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
	emailAddrMatch  = regexp.MustCompile(`<(.*?@?.*?)>`) // removed `:` between last > and `
)

func init() {
	flag.Parse()
	if *fFile == "" {
		log.Fatalln("Expected `file` not provided. Nothing to do!")
	}
}

func main() {
	infoLine("Debug printing enabled")

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	// perhaps this is not even needed since it doesn't seem to function any differently with or without it

	MCount := NewMessageCounter()
	NoQ := NewNoQueueAddrs()
	InDefers := NewIndividDefers()
	expvar.Publish("No Queue Email Addresses", NoQ)
	expvar.Publish("Individually Deferred Tries", InDefers)

	//http.Handle("/debug/expvars", expvar.Handler())
	// "only needed if adjusting the path" or something
	if !*fClose {
		go func() {
			infoLine("Start http server")
			err := http.ListenAndServe(*fPort, nil)
			if err != nil {
				log.Fatalln("Error starting HTTP server", err)
			}
		}()
	}

	for line := range fileRunner(ctx, *fFile) {
		maillogWorker(line, MCount, NoQ, InDefers)
	}
	fmt.Println("Message Counts:")
	fmt.Printf("Accepted In:%7v\n", MCount.Accepted_in)
	fmt.Printf("Sent Messages:%5v\n", MCount.Sent)
	fmt.Printf("No Queues:%9v\n", MCount.No_queue)
	fmt.Printf("Bounced:%11v\n", MCount.Bounced)
	fmt.Printf("Deferred Tries:%4v\n", MCount.Deferred_tries)
	if *fCDefers {
		fmt.Println("Individual defers: ", InDefers.Individual_Defers)
	}
	if *fCNoQs {
		fmt.Println("No Queue Addresses: ", NoQ.NoQueues)
	}

}

// log line producer
// tails (follows / sleeps ) opened file and
// sends log lines through channel
func fileRunner(ctx context.Context, file string) <-chan string {

	ofile, err := os.Open(file)
	if err != nil {
		log.Fatalf("Error opening file: %q", file)
	}
	lines := make(chan string)
	r := bufio.NewReader(ofile)

	go func() {
		defer ofile.Close()
		for {
			if ctx.Err() != nil {
				infoLine("ctx fileRun done")
				return
			}
			line, err := r.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					infoLine("fileRunner: EOF")
					if *fClose {
						infoLine("closing")
						close(lines)
						//<-ctx.Done()
						return
					}
					time.Sleep(5 * time.Second)
					continue
				}
				log.Fatalf("fileRunner: Error -> %q", err)
			}
			lines <- line
		}
	}()
	return lines
}

// log line consumer
// receives log lines and parses them, providing counts for message stats
func maillogWorker(line string, m *MessageCounter, q *NoQueueAddrs, i *IndividDefers) {
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
					q.addNoQueue(noq_addr[1])
					if *fNoQ {
						fmt.Println("No Queue Email Address ->:", noq_addr[1])
					}
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
				if *fDefer {
					fmt.Print("Deferred Message ->:", line)
				}
				if emaddr := emailAddrMatch.FindStringSubmatch(line); emaddr != nil {
					i.addDefer(emaddr[1])
					if *fDefer {
						fmt.Println("Deferred Email Address ->:", emaddr[1])
					}
				}
				// get queue id and address, etc
				// and use addDefer method to count
			case strings.Contains(line, postBounce):
				m.Bounced.Add(1)
				// get queue id and address, etc
				if *fBounce {
					fmt.Print("Bounced Message ->:", line)
				}
				if baddr := emailAddrMatch.FindStringSubmatch(line); baddr != nil {
					if *fBounce {
						fmt.Println("Bounced Email Address ->:", baddr[1])
					}
				}
			default:
				if *fSMTP || *fAllIgnored {
					fmt.Print("SMTP Ignored ->:", line)
				}
			}
		}

		if *fAllIgnored {
			fmt.Print("Unmatched & Ignored Line ->:", line)
		}
	}
}
