package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
)

var (
	targetFile = flag.String("file", "", "File to regex through by line")
	// mine.
	// total count for matches in CP.207.wpn.maillog_27: 714612
	// Group[0] for msgLineMatch.FindStringSubmatch(line) is "postfix/qmgr" || "postfix-slow/smtp" || "postfix/smtp" || "postfix/cleanup" || "postfix/smtpd" ||
	// Group[1] for same is "postfix/cleanup" || actually yeah group two looks like always the same as group 1. which tracks now that I'm looking
	// Group[2] is empty
	// Group[3] is the "qmgr" || "smtp" || etc
	msgLineMatch = regexp.MustCompile(`(postfix(-slow|-fast|-medium|-restrictive)?\/(smtpd?|scache|cleanup|qmgr|bounce|error|warning|fatal|panic))`)

	// mine on the fly to try and combine with 'logLine'
	// total count for matches in CP.207.wpn.maillog_27: 714612 (mmkay)
	msgLineMatchKim = regexp.MustCompile(`(postfix|opendkim)(-slow|-fast|-medium|-restrictive)?\/(smtpd?|scache|cleanup|qmgr|bounce|error|warning|fatal|panic)`)

	// kumina
	// total count for matches in CP.207.wpn.maillog_27: 688641
	// Group[0] is [example] "postfix/qmgr[1237078]: $queueID: removed" || "postfix/discard"[1293699]: warning: deliver_request_get: error receiving common attributes" || "postfix/smtp[1293636] %queueID: to= blablba out of strage full log line"
	// Group[1] is "postfix" (or probably opendkim those log lines might not actually be in here)
	// Group[2] is "/smtp" or "/discard" or "/qmgr
	// Group[3] is "smtp" or "discard" or "qmgr"
	// Group[4] is "$queueID: removed" || "warning: deliver_request_get: error receiving common attributes" || "warning: unexpected attribute nrequest from defer socket (expecting: flags)"
	// Group[4] could also be a full log line out of storage message, Untrusted TLS connection line, "(Host of domain name not found. Name service error for name= [. . .]")
	logLine = regexp.MustCompile(` ?(postfix|opendkim)(/(\w+))?\[\d+\]: ((?:(warning|error|fatal|panic): )?.*)`)
)

func countOne(k string, m map[string]int) {
	m[k]++
}

func main() {
	flag.Parse()
	if *targetFile == "" {
		fmt.Println("Args expected! Nothing to do")
		return
	}

	var counterMap = map[string]int{
		"msgLineMatch":    0,
		"msgLineMatchKim": 0,
		"logLine":         0,
	}

	f, err := os.Open(*targetFile)
	if err != nil {
		fmt.Printf("fuckin err here %q dog\n", err)
		return
	}
	defer f.Close()

	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("<EOF>")
				break
			}
			fmt.Printf("fucking err here dog: %q\n", err)
			return
		}
		if msgLineMatch.MatchString(line) {
			countOne("msgLineMatch", counterMap)
			//	x := msgLineMatch.FindStringSubmatch(line)
			//	fmt.Println("Group one of msgLineMatch is: ", x[0])
			//	fmt.Println("Group two of msgLineMatch is: ", x[1])
			//	fmt.Println("Group four of msgLineMatch is: ", x[3])
		}
		if msgLineMatchKim.MatchString(line) {
			countOne("msgLineMatchKim", counterMap)
		}
		if logLine.MatchString(line) {
			countOne("logLine", counterMap)
			x := logLine.FindStringSubmatch(line)
			//fmt.Println("Group one of logLine is: ", x[0])
			//fmt.Println("Group two of logLine is: ", x[1])
			//fmt.Println("Group three of logLine is: ", x[2])
			//fmt.Println("Group four of logLine is: ", x[3])
			fmt.Println("Group five of logLine is: ", x[4])
		}
	}
	fmt.Println(counterMap)
}
