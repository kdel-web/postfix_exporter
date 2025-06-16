package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
)

const (
	defaultTargetFile = "/home/kdellinger/postfix_exporter/outputs/BogusAmalgamTest.log"
	postSmtpd         = "postfix/smtpd"
	postNoQueue       = "NOQUEUE"
	//postSmtp = "postfix/smtp"
	postFast   = "postfix-fast/smtp"
	postSlow   = "postfix-slow/smtp"
	postMed    = "postfix-medium/smtp"
	postRes    = "postfix-restrictive/smtp"
	postSent   = "status=sent"
	postDefer  = "status=deferred"
	postBounce = "status=bounced"
	postClean  = "postfix/cleanup"
)

var (
	usingRegex    *regexp.Regexp
	clogger       = log.New(os.Stdout, "INFO-LINE ", log.LstdFlags|log.Lshortfile)
	targetFile    = flag.String("file", defaultTargetFile, "Maillog file to parse")
	targetPrint   = flag.Bool("v", false, "Print arbitrary info messages to stdout")
	targetNoMatch = flag.Bool("not-matched", false, "Print lines that were not matched")
	targetChoice  = flag.Bool("kumina", false, "Use kumina regex instead")
	targetUnknown = flag.Bool("uprint", false, "Enable printing of 'unknown' lines")

	newMsgLineMatch = regexp.MustCompile(`(opendkim|postfix(?:-slow)?(?:-fast)?(?:-medium)?(?:-restrictive)?\/(?:smtpd?|scache|cleanup|qmgr|bounce|error|warning|fatal|panic|discard)?)`)
	msgDelaysMatch  = regexp.MustCompile(`delays=([0-9.?\/]+)\,`)
	msgIDMatch      = regexp.MustCompile(`\s([A-F0-9]{6,}):`)
	emailAddrMatch  = regexp.MustCompile(`<(.*?@?.*?)>:`)

	// kumina regex
	logLine = regexp.MustCompile(` ?(postfix|opendkim)(/(\w+))?\[\d+\]: ((?:(warning|error|fatal|panic): )?.*)`)
)

func fileRunner(file string, lines chan<- string) {
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
				close(lines)
				break
			}
			log.Fatalf("fileRunner: Error -> %q", err)
		}
		lines <- line
	}
}

func main() {
	flag.Parse()
	if *targetFile == "" {
		log.Println("Args expected. Nothing to do!")
		return
	}
	if *targetChoice == true {
		usingRegex = logLine
	} else {
		usingRegex = newMsgLineMatch
	}

	lineCounter := 0
	lines := make(chan string)

	msgCounter := map[string]int{
		"no_queue":       0,
		"accepted_in":    0,
		"sent_msgs":      0,
		"opendkim":       0,
		"cleanup":        0,
		"bounced":        0,
		"deferred_tries": 0,
		"scache":         0,
		"queue_manager":  0,

		"unknown": 0,
	}

	runningMsgs := make(map[string]int)

	infoLine("go go filerunner")
	go fileRunner(*targetFile, lines)

	infoLine("go parser")
	for line := range lines {
		lineCounter++
		if usingRegex.MatchString(line) {
			switch {
			case strings.Contains(line, postSmtpd):
				if strings.Contains(line, postNoQueue) {
					// after a bunch of troubleshooting, realized that there were actually
					// only 2 smtpd messages accepted inbound...
					// added 20, so 22 inbound is expected count.
					addOne("no_queue", msgCounter)
					infoLine("No Queue: ", line)
				} else if msgIn := msgIDMatch.FindStringSubmatch(line); msgIn != nil {
					addOne("accepted_in", msgCounter)
					infoLine("Accepted In: ", line)
					msgTrackAdd(msgIn[1], runningMsgs)
				} else {
					infoLine("Unmatched SMTPD line: ", line)
				}
			case strings.Contains(line, "postfix/qmgr"):
				addOne("queue_manager", msgCounter)
			case strings.Contains(line, "postfix/scache"):
				addOne("scache", msgCounter)
			case strings.Contains(line, "opendkim"):
				addOne("opendkim", msgCounter)
			case strings.Contains(line, postSent):
				msgId := msgIDMatch.FindStringSubmatch(line)
				addOne("sent_msgs", msgCounter)
				infoLine("Sent Message: ", line)
				msgTrackRemove(msgId[1], runningMsgs)
			case strings.Contains(line, postClean):
				addOne("cleanup", msgCounter)
				infoLine("Cleanup Line: ", line)
			case strings.Contains(line, postBounce):
				msgId := msgIDMatch.FindStringSubmatch(line)
				addOne("bounced", msgCounter)
				infoLine("Bounced Message: ", line)
				msgTrackRemove(msgId[1], runningMsgs)
			case strings.Contains(line, postDefer):
				msgId := msgIDMatch.FindStringSubmatch(line)
				addOne("deferred_tries", msgCounter)
				infoLine("Deferred Line: ", line)
				msgTrackAdd(msgId[1], runningMsgs)
			default:
				addOne("unknown", msgCounter)
				if *targetUnknown == true {
					log.Println(line)
					// unknown lines here would be:
					// untrusted tls connection... host said user over quota... qmgr lines, opendkim lines, postfix/discard lines, scache lines
				}
			}
		}
	}
	fmt.Println("Total lines: ", lineCounter)
	fmt.Println("No queues: ", msgCounter["no_queue"])
	fmt.Println("Accepted in: ", msgCounter["accepted_in"])
	fmt.Println("Sent Messages: ", msgCounter["sent_msgs"])
	fmt.Println("Cleanup Lines: ", msgCounter["cleanup"])
	fmt.Println("Bounced: ", msgCounter["bounced"])
	fmt.Println("Deferred Message Attempts: ", msgCounter["deferred_tries"])
	fmt.Println("Unknown Lines: ", msgCounter["unknown"])
	fmt.Println("Opendkim Lines: ", msgCounter["opendkim"])
	fmt.Println("Cache Lines: ", msgCounter["scache"])
	fmt.Println("Queue Manager Lines: ", msgCounter["queue_manager"])
	//fmt.Println("Message Tracker: ", runningMsgs)
	fmt.Println("Message Tracker count: ", len(runningMsgs))
}

func addOne(s string, m map[string]int) {
	m[s]++
}

func infoLine(a ...any) {
	if *targetPrint == true {
		clogger.Println(a...)
	}
}

func msgTrackAdd(s string, m map[string]int) {
	_, ok := m[s]
	if ok == true {
		m[s]++
	} else {
		s = strings.TrimSpace(s)
		m[s] = 1
	}
}

func msgTrackRemove(s string, m map[string]int) {
	s = strings.TrimSpace(s)
	delete(m, s)
}
