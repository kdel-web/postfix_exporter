package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
)

const (
	defaultTargetFile = "/home/kdellinger/postfix_exporter/outputs/BogusAmalgamTest.log"
	postSmtpd         = "postfix/smtpd"
	postNoQueue       = "NOQUEUE"
	postSmtp          = "postfix/smtp"
	postFast          = "postfix-fast/smtp"
	postSlow          = "postfix-slow/smtp"
	postMed           = "postfix-medium/smtp"
	postRes           = "postfix-restrictive/smtp"
	postSent          = "status=sent"
	postDefer         = "status=deferred"
	postBounce        = "status=bounced"
	postClean         = "postfix/cleanup"
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
	/*
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
	*/
	//runningMsgs := make(map[string]int)

	infoLine("go go filerunner")
	go fileRunner(*targetFile, lines)
	matchCounter := 0

	for line := range lines {
		lineCounter++
		newLogMatches := newMsgLineMatch.FindStringSubmatch(line)
		if newLogMatches != nil {
			//matchCounter++
			//fmt.Printf("%q\n", newLogMatches[1])
			newProcess := newLogMatches[1]
			switch newProcess {
			case postSmtp, postFast, postSlow, postMed, postRes, "opendkim":
				matchCounter++
				fmt.Println(newProcess, " -> ", line)
			}

		}
	}
	fmt.Println("Line Count: ", lineCounter)
	fmt.Println("Match Count: ", matchCounter)
}

/*
	if usingRegex.MatchString(line) {

				switch {
				case strings.Contains(line, postSmtp), strings.Contains(line, postFast), strings.Contains(line, postSlow), strings.Contains(line, postMed), strings.Contains(line, postRes):
					if lineDelays := msgDelaysMatch.FindStringSubmatch(line); lineDelays != nil {
						splitDelays := strings.Split(lineDelays[1], "/")
						fmt.Println(line)
						fmt.Println("'lineDelays' length: ", len(splitDelays))
						fmt.Println("'lineDelays 1: ", splitDelays[0])
						fmt.Println("'lineDelays 2: ", splitDelays[1])
						fmt.Println("'lineDelays 3: ", splitDelays[2])
						fmt.Println("'lineDelays 4: ", splitDelays[3])
					}
				}
			}
			newLogMatches := usingRegex.FindStringSubmatch(line)
			if newLogMatches != nil {
				newProcess := newLogMatches[1]
				fmt.Println(newProcess)
				switch newProcess {
				case postSmtpd:
					fmt.Println("case true: ", line)
				}
			} else {
				fmt.Println("WHAT: ", line)
*/

func infoLine(a ...any) {
	if *targetPrint == true {
		clogger.Println(a...)
	}
}
