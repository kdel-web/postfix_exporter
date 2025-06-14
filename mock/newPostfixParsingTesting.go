package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
)

// what we need to do is basically capture everything the current exporter is capturing
// then add the stuff we want
// then adjust the groups, so that
// 1. the current metrics remain pretty much the same (but are improved), and
// 2. additional metrics become available.

const (
	cMatch   = "matched"
	cNoMatch = "not matched"
)

var (
	targetFile    = flag.String("file", "/home/kdellinger/postfix_exporter/outputs/BogusAmalgamTest.log", "File to regex through by line")
	targetPrint   = flag.Bool("v", false, "Print various messages to stdout")
	fMatchPrint   = flag.Bool("print-match", false, "Print matches to stdout")
	fNoMatchPrint = flag.Bool("no-match", false, "Print not matched lines to stdout")
	targetChoice  = flag.Bool("kumina", false, "Use kumina regex instead")
	targetNew     = flag.Bool("npattern", false, "Use 'New' regex pattern instead")
	fGroup        = flag.Int("group", 0, "Print the number subgroup")
	msgLineMatch  = regexp.MustCompile(`(postfix)(-slow|-fast|-medium|-restrictive)?\/(smtpd?|scache|cleanup|qmgr|bounce|error|warning|fatal|panic|discard)?`)

	// we are matching everything in the sample file with the new pattern below.
	// we are also matching every single line in the newest full day sample log file from postfix202, saved as outputs/New-maillog-20250613
	// *** Best yet: newMsgLineMatch = regexp.MustCompile(`(opendkim)|(postfix)(-slow|-fast|-medium|-restrictive)?\/(smtpd?|scache|cleanup|qmgr|bounce|error|warning|fatal|panic|discard)?`)
	// groups will be a bit different.
	// looks like:
	// group[1] should be "postfix/smtp" || "postfix-slow/smtp" || "postfix-fast/smtp" || "postfix/smtpd" || etc etc
	// group[2] should be "postfix" (of "postfix/smtp", above, etc )

	// Testing, using non-capturing groups
	newMsgLineMatch = regexp.MustCompile(`(opendkim|postfix(?:-slow)?(?:-fast)?(?:-medium)?(?:-restrictive)?\/(?:smtpd?|scache|cleanup|qmgr|bounce|error|warning|fatal|panic|discard)?)`)
	// if using this non-capture group method, then group 1 is always going to be:
	// "postfix/smtpd" || "postfix-slow/smtp" || "postfix-fast/smtp" || "postfix-medium/smtp" || "postfix-restrictive/smtp" || "opendkim"
	// that MIGHT be the preferred method.... all lines still being matched,
	// and the regex syntax is intended to minimize the confusion between matching the relevant lines via patterns but not actually capturing info that isn't used.

	// yeah while the later groups might be helpful, the subgroups that start "(smtpd?|scache|..." help more for limiting matches to the "postfix/smtp", "postfix/cleanup", portion
	// yeah so this should mainly be used just for the first group, which shoooould be present in all matches. (and all lines, at this time anyway)
	// then other methods will be used to glean the info from the line after this first piece.

	// group[3] should be "-slow" || "-medium" above, etc (NOTE: should not be explicitly relied upon bc eg "postfix/smtp" third group would be "smtp", while "postfix-fast/smtp" would be "-fast")
	// similarly to note above, "opendkim" line would be group[1] == opendkim, group[2] == opendkim
	// group[4] NOTE note just above! should be "smtp" || "smtpd" ||
	// I believe could probably access last item via -1, which should then be "smtp" || "smtpd" || "qmgr" || "discard" || though note "opendkim" might be blank here

	// second section of notes:
	// "postfix/discard" is pretty much useless/irrelevant

	// regarding regexp.FindStringSubmatch -> the length of all sub matches is 4, however, some may be "zeroed".
	// eg: [[postfix/smtp postfix  smtp]] has len 4
	// eg: [[opendekim   ]] also has len 4
	// eg: [[postfix/discard postifx  discard]] len 4
	// eg:

	logLine = regexp.MustCompile(` ?(postfix|opendkim)(/(\w+))?\[\d+\]: ((?:(warning|error|fatal|panic): )?.*)`)
	rCount  = map[string]int{
		cMatch:   0,
		cNoMatch: 0,
	}
)

func main() {
	flag.Parse()
	if *targetFile == "" {
		fmt.Println("Args expected! Nothing to do")
		return
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
		if *targetChoice == true {
			lineMatchLogic(line, logLine)
		} else if *targetNew {
			lineMatchLogic(line, newMsgLineMatch)
		} else {
			lineMatchLogic(line, msgLineMatch)
		}
	}
	totallines := rCount[cMatch] + rCount[cNoMatch]
	fmt.Println("Total finds: ", rCount[cMatch])
	fmt.Println("Total lines read: ", totallines)
}

func lineMatchLogic(l string, r *regexp.Regexp) {
	matched := r.FindStringSubmatch(l)
	if matched != nil {
		vPrinter(matched)
		vPrinter(len(matched))
		printSubgroup(*fGroup, matched)
		if *fMatchPrint == true {
			fmt.Println("Matched line ->: ", l)
			fmt.Println("Matched groups ->: ", matched)
		}
		countOne(cMatch, rCount)
	} else {
		vPrinter("Not matched ->: ", l)
		if *fNoMatchPrint == true {
			fmt.Println("No matched line ->: ", l)
		}
		countOne(cNoMatch, rCount)
	}
}

func countOne(k string, m map[string]int) {
	m[k]++
}

func printSubgroup(g int, l []string) {
	if *fGroup > 0 {
		fmt.Println(l[g])
	}
}

func vPrinter(a ...any) {
	if *targetPrint == true {
		fmt.Println(a)
	}
}
