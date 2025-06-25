//////////////////////////////////////////////////////////////////////////////////////
// PROGRAM HAS BEEN MODIFIED FROM ITS ORIGINAL IMPLEMENTATION
// PLEASE REVIEW README.md
// **
// Original copyright and licensing information retained,
// though the application has been modified to fit specific implementation criteria
//////////////////////////////////////////////////////////////////////////////////////

// Copyright 2017 Kumina, https://kumina.nl/
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	postfixNamespace = "Postfix"
	postNoQueue      = "NOQUEUE"
	postSmtpd        = "postfix/smtpd"
	postSmtp         = "postfix/smtp"
	postFast         = "postfix-fast/smtp"
	postSlow         = "postfix-slow/smtp"
	postMed          = "postfix-medium/smtp"
	postRes          = "postfix-restrictive/smtp"
	postQmgr         = "postfix/qmgr"
	postClean        = "postfix/cleanup"
	postDiscard      = "postfix/discard"
	postSent         = "status=sent"
	postDefer        = "status=deferred"
	postBounce       = "status=bounced"
	postOpendkim     = "opendkim"
)

var (
	postfixUpDesc = prometheus.NewDesc(
		prometheus.BuildFQName("postfix", "", "up"),
		"Whether scraping Postfix's metrics was successful.",
		[]string{"path"}, nil)

	// Added parsing patterns.
	// regex pattern was chosen to be used as a "pre-filter"; there is intentionally only one capturing group
	newMsgLineMatch = regexp.MustCompile(`(opendkim|postfix(?:-slow)?(?:-fast)?(?:-medium)?(?:-restrictive)?\/(?:smtpd?|scache|cleanup|qmgr|bounce|error|warning|fatal|panic|discard)?)`)
	msgDelaysMatch  = regexp.MustCompile(`delays=([0-9.?\/]+)\,`)
	msgIDMatch      = regexp.MustCompile(`\s([A-F0-9]{6,}):`)
	emailAddrMatch  = regexp.MustCompile(`<(.*?@?.*?)>:`)

	// Previous patterns for parsing log messages.
	logLine                             = regexp.MustCompile(` ?(postfix|opendkim)(/(\w+))?\[\d+\]: ((?:(warning|error|fatal|panic): )?.*)`)
	lmtpPipeSMTPLine                    = regexp.MustCompile(`, relay=(\S+), .*, delays=([0-9\.]+)/([0-9\.]+)/([0-9\.]+)/([0-9\.]+), `)
	qmgrInsertLine                      = regexp.MustCompile(`:.*, size=(\d+), nrcpt=(\d+) `)
	qmgrExpiredLine                     = regexp.MustCompile(`:.*, status=(expired|force-expired), returned to sender`)
	smtpStatusLine                      = regexp.MustCompile(`, status=(\w+) `)
	smtpTLSLine                         = regexp.MustCompile(`^(\S+) TLS connection established to \S+: (\S+) with cipher (\S+) \((\d+)/(\d+) bits\)`)
	smtpConnectionTimedOut              = regexp.MustCompile(`^connect\s+to\s+(.*)\[(.*)\]:(\d+):\s+(Connection timed out)$`)
	smtpdFCrDNSErrorsLine               = regexp.MustCompile(`^warning: hostname \S+ does not resolve to address `)
	smtpdProcessesSASLLine              = regexp.MustCompile(`: client=.*, sasl_method=(\S+)`)
	smtpdRejectsLine                    = regexp.MustCompile(`^NOQUEUE: reject: RCPT from \S+: ([0-9]+) `)
	smtpdLostConnectionLine             = regexp.MustCompile(`^lost connection after (\w+) from `)
	smtpdSASLAuthenticationFailuresLine = regexp.MustCompile(`^warning: \S+: SASL \S+ authentication failed: `)
	smtpdTLSLine                        = regexp.MustCompile(`^(\S+) TLS connection established from \S+: (\S+) with cipher (\S+) \((\d+)/(\d+) bits\)`)
	opendkimSignatureAdded              = regexp.MustCompile(`[\w\d]+: DKIM-Signature field added \(s=(\w+), d=(.*)\)`)
	bounceNonDeliveryLine               = regexp.MustCompile(`: sender non-delivery notification: `)
)

// CollectShowqFromReader parses the output of Postfix's 'showq' command
// and turns it into metrics.
//
// The output format of this command depends on the version of Postfix
// used. Postfix 2.x uses a textual format, identical to the output of
// the 'mailq' command. Postfix 3.x uses a binary format, where entries
// are terminated using null bytes. Auto-detect the format by scanning
// for null bytes in the first 128 bytes of output.
func CollectShowqFromReader(file io.Reader, ch chan<- prometheus.Metric) error {
	reader := bufio.NewReader(file)
	buf, err := reader.Peek(128)
	if err != nil && err != io.EOF {
		log.Printf("Could not read postfix output, %v", err)
	}
	if bytes.IndexByte(buf, 0) >= 0 {
		return CollectBinaryShowqFromReader(reader, ch)
	}
	return CollectTextualShowqFromReader(reader, ch)
}

// CollectTextualShowqFromReader parses Postfix's textual showq output.
func CollectTextualShowqFromReader(file io.Reader, ch chan<- prometheus.Metric) error {

	// Histograms tracking the messages by size and age.
	sizeHistogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "postfix",
			Name:      "showq_message_size_bytes",
			Help:      "Size of messages in Postfix's message queue, in bytes",
			Buckets:   []float64{1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9},
		},
		[]string{"queue"})
	ageHistogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "postfix",
			Name:      "showq_message_age_seconds",
			Help:      "Age of messages in Postfix's message queue, in seconds",
			Buckets:   []float64{1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8},
		},
		[]string{"queue"})

	err := CollectTextualShowqFromScanner(sizeHistogram, ageHistogram, file)

	sizeHistogram.Collect(ch)
	ageHistogram.Collect(ch)
	return err
}

func CollectTextualShowqFromScanner(sizeHistogram prometheus.ObserverVec, ageHistogram prometheus.ObserverVec, file io.Reader) error {
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)
	// Initialize all queue buckets to zero.
	for _, q := range []string{"active", "hold", "other"} {
		sizeHistogram.WithLabelValues(q)
		ageHistogram.WithLabelValues(q)
	}

	location, err := time.LoadLocation("Local")
	if err != nil {
		log.Println(err)
	}

	// Regular expression for matching postqueue's output. Example:
	// "A07A81514      5156 Tue Feb 14 13:13:54  MAILER-DAEMON"
	messageLine := regexp.MustCompile(`^[0-9A-F]+([\*!]?) +(\d+) (\w{3} \w{3} +\d+ +\d+:\d{2}:\d{2}) +`)

	for scanner.Scan() {
		text := scanner.Text()
		matches := messageLine.FindStringSubmatch(text)
		if matches == nil {
			continue
		}
		queueMatch := matches[1]
		sizeMatch := matches[2]
		dateMatch := matches[3]

		// Derive the name of the message queue.
		queue := "other"
		if queueMatch == "*" {
			queue = "active"
		} else if queueMatch == "!" {
			queue = "hold"
		}

		// Parse the message size.
		size, err := strconv.ParseFloat(sizeMatch, 64)
		if err != nil {
			return err
		}

		// Parse the message date. Unfortunately, the
		// output contains no year number. Assume it
		// applies to the last year for which the
		// message date doesn't exceed time.Now().
		date, err := time.ParseInLocation("Mon Jan 2 15:04:05", dateMatch, location)
		if err != nil {
			return err
		}
		now := time.Now()
		date = date.AddDate(now.Year(), 0, 0)
		if date.After(now) {
			date = date.AddDate(-1, 0, 0)
		}

		sizeHistogram.WithLabelValues(queue).Observe(size)
		ageHistogram.WithLabelValues(queue).Observe(now.Sub(date).Seconds())
	}
	return scanner.Err()
}

// ScanNullTerminatedEntries is a splitting function for bufio.Scanner
// to split entries by null bytes.
func ScanNullTerminatedEntries(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		// Valid record found.
		return i + 1, data[0:i], nil
	} else if atEOF && len(data) != 0 {
		// Data at the end of the file without a null terminator.
		return 0, nil, errors.New("Expected null byte terminator")
	} else {
		// Request more data.
		return 0, nil, nil
	}
}

// CollectBinaryShowqFromReader parses Postfix's binary showq format.
func CollectBinaryShowqFromReader(file io.Reader, ch chan<- prometheus.Metric) error {
	scanner := bufio.NewScanner(file)
	scanner.Split(ScanNullTerminatedEntries)

	// Histograms tracking the messages by size and age.
	sizeHistogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "postfix",
			Name:      "showq_message_size_bytes",
			Help:      "Size of messages in Postfix's message queue, in bytes",
			Buckets:   []float64{1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9},
		},
		[]string{"queue"})
	ageHistogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "postfix",
			Name:      "showq_message_age_seconds",
			Help:      "Age of messages in Postfix's message queue, in seconds",
			Buckets:   []float64{1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8},
		},
		[]string{"queue"})

	// Initialize all queue buckets to zero.
	for _, q := range []string{"active", "deferred", "hold", "incoming", "maildrop"} {
		sizeHistogram.WithLabelValues(q)
		ageHistogram.WithLabelValues(q)
	}

	now := float64(time.Now().UnixNano()) / 1e9
	queue := "unknown"
	for scanner.Scan() {
		// Parse a key/value entry.
		key := scanner.Text()
		if len(key) == 0 {
			// Empty key means a record separator.
			queue = "unknown"
			continue
		}
		if !scanner.Scan() {
			return fmt.Errorf("key %q does not have a value", key)
		}
		value := scanner.Text()

		if key == "queue_name" {
			// The name of the message queue.
			queue = value
		} else if key == "size" {
			// Message size in bytes.
			size, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return err
			}
			sizeHistogram.WithLabelValues(queue).Observe(size)
		} else if key == "time" {
			// Message time as a UNIX timestamp.
			utime, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return err
			}
			ageHistogram.WithLabelValues(queue).Observe(now - utime)
		}
	}

	sizeHistogram.Collect(ch)
	ageHistogram.Collect(ch)
	return scanner.Err()
}

// CollectShowqFromSocket collects Postfix queue statistics from a socket.
func CollectShowqFromSocket(path string, ch chan<- prometheus.Metric) error {
	fd, err := net.Dial("unix", path)
	if err != nil {
		return err
	}
	defer fd.Close()
	return CollectShowqFromReader(fd, ch)
}

// CollectFromLogline collects metrict from a Postfix log line.
func (e *PostfixCollector) CollectFromLogLine(line string) {
	// primary metrics logic

	// "Strip off timestamp, hostname, etc."
	// better description might be:
	// 'this is how log lines are filtered for relevancy, and
	// organized into groups in order to count/parse/extract data.'
	// logMatches := logLine.FindStringSubmatch(line)
	newLogMatches := newMsgLineMatch.FindStringSubmatch(line)
	logMatches := logLine.FindStringSubmatch(line)
	// currently there is technically one capturing group, but the regexp.FindStringSubmatch has len 2, because 0 is the full match
	// for our purposes, 0 and 1 should be the same

	if newLogMatches != nil {
		newProcess := newLogMatches[1]
		switch newProcess {
		case postSmtpd:
			if strings.Contains(line, postNoQueue) {
				e.msgsNoQueue.Inc()
			} else if msgIDMatch.MatchString(line) {
				e.msgsAcceptedIn.Inc()
			} else if strings.Contains(line, "disconnect") {
				e.sSmtpdDisconnects.Inc()
			} else if strings.Contains(line, "connect") {
				e.sSmtpdConnects.Inc()
			} else {
				infoLine("SMTPD ignores: ", line)
				// there will be other lines but they are not relevant
			}

		case postSmtp, postFast, postSlow, postMed, postRes:
			if lineDelays := msgDelaysMatch.FindStringSubmatch(line); lineDelays != nil {
				splitDelays := strings.Split(lineDelays[1], "/")
				switch {
				case strings.Contains(line, postSent):
					e.msgsSent.Inc()
					// if problems with last value in these functions, remove the last value; set as "" instead of "sent_msgs"
					// they're supposed to be "labels" in prometheus speak
					addToHistogramVec(e.smtpDelays, splitDelays[0], "before_queue_manager", "")
					addToHistogramVec(e.smtpDelays, splitDelays[1], "queue_manager", "")
					addToHistogramVec(e.smtpDelays, splitDelays[2], "connection_setup", "")
					addToHistogramVec(e.smtpDelays, splitDelays[3], "transmission", "")
				case strings.Contains(line, postDefer):
					e.msgsDeferredTries.Inc()
					addToHistogramVec(e.smtpDelays, splitDelays[0], "before_queue_manager", "")
					addToHistogramVec(e.smtpDelays, splitDelays[1], "queue_manager", "")
					addToHistogramVec(e.smtpDelays, splitDelays[2], "connection_setup", "")
					addToHistogramVec(e.smtpDelays, splitDelays[3], "transmission", "")
				case strings.Contains(line, postBounce):
					e.msgsBounced.Inc()
					addToHistogramVec(e.smtpDelays, splitDelays[0], "before_queue_manager", "")
					addToHistogramVec(e.smtpDelays, splitDelays[1], "queue_manager", "")
					addToHistogramVec(e.smtpDelays, splitDelays[2], "connection_setup", "")
					addToHistogramVec(e.smtpDelays, splitDelays[3], "transmission", "")
				default:
					infoLine("INFO DEBUG: 'delays=' match, default case. Check 'status=***' line: ", line)
				}
			}

		case postClean:
			e.msgsCleanupLines.Inc()
		case postQmgr:
			e.sQmgrOperations.Inc()
		case postOpendkim:
			if opendkimMatches := opendkimSignatureAdded.FindStringSubmatch(line); opendkimMatches != nil {
				e.sOpenDKIM.WithLabelValues(opendkimMatches[1], opendkimMatches[2]).Inc()
			} else {
				infoLine("DEBUG: Expecting but did not receive OpenDKIM Match: ", line)
				// a line that might be expected here is '... no signing table match for ...'
			}
		default:
			if !strings.Contains(line, postDiscard) {
				e.msgsUnknownUnsupported.Inc()
				infoLine("*New Regex Matchers* Not Matched: ", line)
			}
		}
	}

	if logMatches != nil {
		process := logMatches[1]
		// level := logMatches[5] // was only used for collecting unknown loglines. changed to counter
		/*
			ORIGINAL REGEX NOTES
			- group 1 will only ever be "postfix" or "opendkim" (then there is default case at bottom)
			- group 2 is "/smtp" || "/discard" || "/qmgr" || "/scache" || "smtpd" etc
			- group 3 is the same as above except without the leading slash
			- group 4 is the remaining portion of the logline, starting right after the "postfix/$daemonName[###]: $HERE ..."

		*/
		remainder := logMatches[4]
		switch process {
		case "postfix":
			// Group patterns to check by Postfix service.
			subprocess := logMatches[3]
			switch subprocess {
			case "cleanup":
				// not all valid cleanup lines contain 'message-id' signifier
				if strings.Contains(remainder, ": message-id=<") {
					e.cleanupProcesses.Inc()
					// account for `replace: header Received: ` and `replace: header Message-ID`
				} else if strings.Contains(remainder, ": replace: header ") {
					e.cleanupProcesses.Inc()
					// account for `prepend: header Subject:`
				} else if strings.Contains(remainder, ": prepend: header Subject:") {
					e.cleanupProcesses.Inc()
				} else if strings.Contains(remainder, ": reject: ") {
					e.cleanupRejects.Inc()
				} else {
					e.msgsUnknownUnsupported.Inc()
					infoLine("Original Regex: case 'cleanup': ", line)
				}
			// this could be completely removed and nothing would change
			case "lmtp":
				if lmtpMatches := lmtpPipeSMTPLine.FindStringSubmatch(remainder); lmtpMatches != nil {
					addToHistogramVec(e.lmtpDelays, lmtpMatches[2], "LMTP pdelay", "before_queue_manager")
					addToHistogramVec(e.lmtpDelays, lmtpMatches[3], "LMTP adelay", "queue_manager")
					addToHistogramVec(e.lmtpDelays, lmtpMatches[4], "LMTP sdelay", "connection_setup")
					addToHistogramVec(e.lmtpDelays, lmtpMatches[5], "LMTP xdelay", "transmission")
				} else {
					e.msgsUnknownUnsupported.Inc()
					infoLine("Original Regex: case 'lmtp': ", line)
				}
			// also could be removed
			case "pipe":
				if pipeMatches := lmtpPipeSMTPLine.FindStringSubmatch(remainder); pipeMatches != nil {
					addToHistogramVec(e.pipeDelays, pipeMatches[2], "PIPE pdelay", pipeMatches[1], "before_queue_manager")
					addToHistogramVec(e.pipeDelays, pipeMatches[3], "PIPE adelay", pipeMatches[1], "queue_manager")
					addToHistogramVec(e.pipeDelays, pipeMatches[4], "PIPE sdelay", pipeMatches[1], "connection_setup")
					addToHistogramVec(e.pipeDelays, pipeMatches[5], "PIPE xdelay", pipeMatches[1], "transmission")
				} else {
					e.msgsUnknownUnsupported.Inc()
					infoLine("Original Regex: case 'pipe': ", line)
				}
			// technically could potentially be relevant to operation, maybe, though not to mail deliverability
			case "qmgr":
				if qmgrInsertMatches := qmgrInsertLine.FindStringSubmatch(remainder); qmgrInsertMatches != nil {
					addToHistogram(e.qmgrInsertsSize, qmgrInsertMatches[1], "QMGR size")
					addToHistogram(e.qmgrInsertsNrcpt, qmgrInsertMatches[2], "QMGR nrcpt")
				} else if strings.HasSuffix(remainder, ": removed") {
					e.qmgrRemoves.Inc()
				} else if qmgrExpired := qmgrExpiredLine.FindStringSubmatch(remainder); qmgrExpired != nil {
					e.qmgrExpires.Inc()
				} else {
					e.msgsUnknownUnsupported.Inc()
					infoLine("Original Regex: case 'qmgr': ", line)
				}
			case "smtp":
				if smtpMatches := lmtpPipeSMTPLine.FindStringSubmatch(remainder); smtpMatches != nil {
					addToHistogramVec(e.smtpDelays, smtpMatches[2], "before_queue_manager", "")
					addToHistogramVec(e.smtpDelays, smtpMatches[3], "queue_manager", "")
					addToHistogramVec(e.smtpDelays, smtpMatches[4], "connection_setup", "")
					addToHistogramVec(e.smtpDelays, smtpMatches[5], "transmission", "")
					if smtpStatusMatches := smtpStatusLine.FindStringSubmatch(remainder); smtpStatusMatches != nil {
						e.smtpProcesses.WithLabelValues(smtpStatusMatches[1]).Inc()
						if smtpStatusMatches[1] == "deferred" {
							e.smtpStatusDeferred.Inc()
						}
					}
				} else if smtpTLSMatches := smtpTLSLine.FindStringSubmatch(remainder); smtpTLSMatches != nil {
					e.smtpTLSConnects.WithLabelValues(smtpTLSMatches[1:]...).Inc()
				} else if smtpMatches := smtpConnectionTimedOut.FindStringSubmatch(remainder); smtpMatches != nil {
					e.smtpConnectionTimedOut.Inc()
				} else {
					e.msgsUnknownUnsupported.Inc()
					infoLine("Original Regex: case 'smtp': ", line)
				}
			case "smtpd":
				if strings.HasPrefix(remainder, "connect from ") {
					e.smtpdConnects.Inc()
				} else if strings.HasPrefix(remainder, "disconnect from ") {
					e.smtpdDisconnects.Inc()
				} else if smtpdFCrDNSErrorsLine.MatchString(remainder) {
					e.smtpdFCrDNSErrors.Inc()
				} else if smtpdLostConnectionMatches := smtpdLostConnectionLine.FindStringSubmatch(remainder); smtpdLostConnectionMatches != nil {
					e.smtpdLostConnections.WithLabelValues(smtpdLostConnectionMatches[1]).Inc()
				} else if smtpdProcessesSASLMatches := smtpdProcessesSASLLine.FindStringSubmatch(remainder); smtpdProcessesSASLMatches != nil {
					e.smtpdProcesses.WithLabelValues(smtpdProcessesSASLMatches[1]).Inc()
				} else if strings.Contains(remainder, ": client=") {
					e.smtpdProcesses.WithLabelValues("").Inc()
				} else if smtpdRejectsMatches := smtpdRejectsLine.FindStringSubmatch(remainder); smtpdRejectsMatches != nil {
					e.smtpdRejects.WithLabelValues(smtpdRejectsMatches[1]).Inc()
				} else if smtpdSASLAuthenticationFailuresLine.MatchString(remainder) {
					e.smtpdSASLAuthenticationFailures.Inc()
				} else if smtpdTLSMatches := smtpdTLSLine.FindStringSubmatch(remainder); smtpdTLSMatches != nil {
					e.smtpdTLSConnects.WithLabelValues(smtpdTLSMatches[1:]...).Inc()
				} else {
					e.msgsUnknownUnsupported.Inc()
					infoLine("Original Regex: case 'smtpd': ", line)
				}
			case "bounce":
				if bounceMatches := bounceNonDeliveryLine.FindStringSubmatch(remainder); bounceMatches != nil {
					e.bounceNonDelivery.Inc()
				} else {
					e.msgsUnknownUnsupported.Inc()
					infoLine("Original Regex: case 'bounce': ", line)
				}
			case "virtual":
				if strings.HasSuffix(remainder, ", status=sent (delivered to maildir)") {
					e.virtualDelivered.Inc()
				} else {
					e.msgsUnknownUnsupported.Inc()
					infoLine("Original Regex: case 'virtual': ", line)
				}
			default:
				if !strings.Contains(line, postDiscard) {
					e.msgsUnknownUnsupported.Inc()
					infoLine("Original Regex: default 'subprocess': ", line)
				}
			}
		case "opendkim":
			if opendkimMatches := opendkimSignatureAdded.FindStringSubmatch(remainder); opendkimMatches != nil {
				e.opendkimSignatureAdded.WithLabelValues(opendkimMatches[1], opendkimMatches[2]).Inc()
			} else {
				e.msgsUnknownUnsupported.Inc()
				infoLine("Original Regex: case 'opendkim': ", line)
			}
		default:
			// Unknown log entry format.
			e.msgsUnknownUnsupported.Inc()
			infoLine("Original Regex: case DEFAULT: ", line)
		}
		// These are being counted as "Unknown" / "Unsupported", but it is expected
		// that this number is not zero, as not all lines include relevant or notable information.
		// An example might be info regarding an untrusted SMTP connection-- SMTP servers
		// may use certificates that are not publicly available, but that's still the domain,
		// the mail still goes there, doesn't really matter that it's 'untrusted'; it's still valid TLS/SSL connection.
		// **If this number is high, (or if any additional smtp rules based on sender/recipient are added),
		// this may require additional review.
	} else {
		//e.msgsUnknownUnsupported.Inc()
		infoLine("INFO: Original Regex - Not Matched: ", line)
		// expected lines here are ANY of the 'postfix-fast/smtp', 'postfix-***/smtp' which are already being counted in new regex patterns above,
		// also the majority of the NOQUEUES, which are also being counted in new regex patterns above.
		// also the 'Untrusted TLS connection established ...' and 'statistics' lines, which are generally irrelevant
	}
	return
}

func addToHistogram(h prometheus.Histogram, value, fieldName string) {
	float, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("HISTOGRAM: Couldn't convert value '%s' for %v: %v", value, fieldName, err)
	}
	h.Observe(float)
}
func addToHistogramVec(h *prometheus.HistogramVec, value, fieldName string, labels ...string) {
	float, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("HISTOGRAM VEC: Couldn't convert value '%s' for %v: %v", value, fieldName, err)
		// This is the one that's printing the errors to stdout
	}
	h.WithLabelValues(labels...).Observe(float)
}

// PostfixExporter holds the state that should be preserved by the
// Postfix Prometheus metrics exporter across scrapes.*
// * original comments retained *
type PostfixCollector struct {
	// PostfixCollector implements the prometheus.Collector interface
	// is instantied with `NewPostfixCollector`, must then be registered

	targetShowqPath string
	targetLogfile   LogSource

	// Added Metrics
	msgsUnknownUnsupported prometheus.Counter
	msgsAcceptedIn         prometheus.Counter
	msgsNoQueue            prometheus.Counter
	msgsSent               prometheus.Counter
	sSmtpdConnects         prometheus.Counter
	sSmtpdDisconnects      prometheus.Counter
	sQmgrOperations        prometheus.Counter
	sOpenDKIM              *prometheus.CounterVec
	msgsBounced            prometheus.Counter
	msgsDeferredTries      prometheus.Counter
	msgsCleanupLines       prometheus.Counter

	// Original Metrics
	// Metrics that should persist after refreshes, based on logs.
	cleanupProcesses                prometheus.Counter
	cleanupRejects                  prometheus.Counter
	cleanupNotAccepted              prometheus.Counter
	lmtpDelays                      *prometheus.HistogramVec
	pipeDelays                      *prometheus.HistogramVec
	qmgrInsertsNrcpt                prometheus.Histogram
	qmgrInsertsSize                 prometheus.Histogram
	qmgrRemoves                     prometheus.Counter
	qmgrExpires                     prometheus.Counter
	smtpDelays                      *prometheus.HistogramVec
	smtpTLSConnects                 *prometheus.CounterVec
	smtpConnectionTimedOut          prometheus.Counter
	smtpProcesses                   *prometheus.CounterVec
	smtpDeferreds                   prometheus.Counter
	smtpdConnects                   prometheus.Counter
	smtpdDisconnects                prometheus.Counter
	smtpdFCrDNSErrors               prometheus.Counter
	smtpdLostConnections            *prometheus.CounterVec
	smtpdProcesses                  *prometheus.CounterVec
	smtpdRejects                    *prometheus.CounterVec
	smtpdSASLAuthenticationFailures prometheus.Counter
	smtpdTLSConnects                *prometheus.CounterVec
	unsupportedLogEntries           *prometheus.CounterVec
	smtpStatusDeferred              prometheus.Counter
	opendkimSignatureAdded          *prometheus.CounterVec
	bounceNonDelivery               prometheus.Counter
	virtualDelivered                prometheus.Counter
}

// NewPostfixExporter creates a new Postfix exporter instance.
func NewPostfixCollector(showqPath string, logSrc LogSource, logUnsupportedLines bool) (*PostfixCollector, error) {
	timeBuckets := []float64{1e-3, 1e-2, 1e-1, 1.0, 10, 1 * 60, 1 * 60 * 60, 24 * 60 * 60, 2 * 24 * 60 * 60}
	return &PostfixCollector{
		//logUnsupportedLines: logUnsupportedLines,
		targetShowqPath: showqPath,
		targetLogfile:   logSrc,

		msgsUnknownUnsupported: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: postfixNamespace,
			Name:      "unknown_maillog_lines_total",
			Help:      "Count of unmatched maillog lines. Formerly known as 'unsupported line'",
		}),

		msgsAcceptedIn: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: postfixNamespace,
			Name:      "messages_accepted_in_total",
			Help:      "Messages accepted in via SMTPD",
		}),

		msgsNoQueue: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: postfixNamespace,
			Name:      "messages_not_accepted_in_total",
			Help:      "Messaged rejected by SMTPD (NO QUEUES)",
		}),

		msgsSent: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: postfixNamespace,
			Name:      "messages_sent_smtp_total",
			Help:      "Messages successfully sent",
		}),

		msgsBounced: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: postfixNamespace,
			Name:      "messages_bounced_total",
			Help:      "Messages bounced",
		}),

		msgsDeferredTries: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: postfixNamespace,
			Name:      "deferred_messages_attempts_total",
			Help:      "Total number of message send attempts for messages in deferred status",
		}),

		msgsCleanupLines: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: postfixNamespace,
			Name:      "message_cleanup_lines_total",
			Help:      "Number of cleanup operations",
		}),

		sSmtpdConnects: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: postfixNamespace,
			Name:      "smtpd_connections_total",
			Help:      "Number of SMTPD connections. (Each connection may include more than one message)",
		}),

		sSmtpdDisconnects: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: postfixNamespace,
			Name:      "smtpd_disconnects_total",
			Help:      "Number of SMTPD disconnects total",
		}),

		sQmgrOperations: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: postfixNamespace,
			Name:      "qmgr_operations_total",
			Help:      "Queue manager operations total",
		}),

		sOpenDKIM: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: postfixNamespace,
			Name:      "opendkim_signed_lines",
			Help:      "opendkim_signed_msgs_total",
		},
			[]string{"subject", "domain"},
		),

		cleanupProcesses: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "cleanup_messages_processed_total",
			Help:      "Total number of messages processed by cleanup.",
		}),
		cleanupRejects: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "cleanup_messages_rejected_total",
			Help:      "Total number of messages rejected by cleanup.",
		}),
		cleanupNotAccepted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "cleanup_messages_not_accepted_total",
			Help:      "Total number of messages not accepted by cleanup.",
		}),
		lmtpDelays: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "postfix",
				Name:      "lmtp_delivery_delay_seconds",
				Help:      "LMTP message processing time in seconds.",
				Buckets:   timeBuckets,
			},
			[]string{"stage"}),
		pipeDelays: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "postfix",
				Name:      "pipe_delivery_delay_seconds",
				Help:      "Pipe message processing time in seconds.",
				Buckets:   timeBuckets,
			},
			[]string{"relay", "stage"}),
		qmgrInsertsNrcpt: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "postfix",
			Name:      "qmgr_messages_inserted_receipients",
			Help:      "Number of receipients per message inserted into the mail queues.",
			Buckets:   []float64{1, 2, 4, 8, 16, 32, 64, 128},
		}),
		qmgrInsertsSize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "postfix",
			Name:      "qmgr_messages_inserted_size_bytes",
			Help:      "Size of messages inserted into the mail queues in bytes.",
			Buckets:   []float64{1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9},
		}),
		qmgrRemoves: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "qmgr_messages_removed_total",
			Help:      "Total number of messages removed from mail queues.",
		}),
		qmgrExpires: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "qmgr_messages_expired_total",
			Help:      "Total number of messages expired from mail queues.",
		}),
		smtpDelays: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "postfix",
				Name:      "smtp_delivery_delay_seconds",
				Help:      "SMTP message processing time in seconds.",
				Buckets:   timeBuckets,
			},
			[]string{"stage"}),
		smtpTLSConnects: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "postfix",
				Name:      "smtp_tls_connections_total",
				Help:      "Total number of outgoing TLS connections.",
			},
			[]string{"trust", "protocol", "cipher", "secret_bits", "algorithm_bits"}),
		smtpDeferreds: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "smtp_deferred_messages_total",
			Help:      "Total number of messages that have been deferred on SMTP.",
		}),
		smtpProcesses: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "postfix",
				Name:      "smtp_messages_processed_total",
				Help:      "Total number of messages that have been processed by the smtp process.",
			},
			[]string{"status"}),
		smtpConnectionTimedOut: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "smtp_connection_timed_out_total",
			Help:      "Total number of messages that have been deferred on SMTP.",
		}),
		smtpdConnects: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "smtpd_connects_total",
			Help:      "Total number of incoming connections.",
		}),
		smtpdDisconnects: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "smtpd_disconnects_total",
			Help:      "Total number of incoming disconnections.",
		}),
		smtpdFCrDNSErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "smtpd_forward_confirmed_reverse_dns_errors_total",
			Help:      "Total number of connections for which forward-confirmed DNS cannot be resolved.",
		}),
		smtpdLostConnections: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "postfix",
				Name:      "smtpd_connections_lost_total",
				Help:      "Total number of connections lost.",
			},
			[]string{"after_stage"}),
		smtpdProcesses: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "postfix",
				Name:      "smtpd_messages_processed_total",
				Help:      "Total number of messages processed.",
			},
			[]string{"sasl_method"}),
		smtpdRejects: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "postfix",
				Name:      "smtpd_messages_rejected_total",
				Help:      "Total number of NOQUEUE rejects.",
			},
			[]string{"code"}),
		smtpdSASLAuthenticationFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "smtpd_sasl_authentication_failures_total",
			Help:      "Total number of SASL authentication failures.",
		}),
		smtpdTLSConnects: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "postfix",
				Name:      "smtpd_tls_connections_total",
				Help:      "Total number of incoming TLS connections.",
			},
			[]string{"trust", "protocol", "cipher", "secret_bits", "algorithm_bits"}),
		unsupportedLogEntries: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "postfix",
				Name:      "unsupported_log_entries_total",
				Help:      "Log entries that could not be processed.",
			},
			[]string{"service", "level"}),
		smtpStatusDeferred: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "smtp_status_deferred",
			Help:      "Total number of messages deferred.",
		}),
		opendkimSignatureAdded: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "opendkim",
				Name:      "signatures_added_total",
				Help:      "Total number of messages signed",
			},
			[]string{"subject", "domain"},
		),
		bounceNonDelivery: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "bounce_non_delivery_notification_total",
			Help:      "Total number of non-delivery notifications sent by bounce.",
		}),
		virtualDelivered: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "postfix",
			Name:      "virtual_delivered_total",
			Help:      "Total number of mail delivered to a virtual mailbox. (expected: zero)",
		}),
	}, nil
}

// Describe the Prometheus metrics that are going to be exported.
func (e *PostfixCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- postfixUpDesc

	if e.targetLogfile == nil {
		return
	}
	// 'Describe' is a method of 'Collector' interface that takes a pointer value type channel
	// 'Desc' is a method of 'Metric' interface that returns pointer value '*Desc'

	// added:
	ch <- e.msgsUnknownUnsupported.Desc()
	ch <- e.msgsAcceptedIn.Desc()
	ch <- e.msgsNoQueue.Desc()
	ch <- e.msgsSent.Desc()
	ch <- e.sSmtpdConnects.Desc()
	ch <- e.sSmtpdDisconnects.Desc()
	ch <- e.sQmgrOperations.Desc()
	ch <- e.msgsBounced.Desc()
	ch <- e.msgsDeferredTries.Desc()
	ch <- e.msgsCleanupLines.Desc()
	e.sOpenDKIM.Describe(ch)

	// previous:
	ch <- e.cleanupProcesses.Desc()
	ch <- e.cleanupRejects.Desc()
	ch <- e.cleanupNotAccepted.Desc()
	e.lmtpDelays.Describe(ch)
	e.pipeDelays.Describe(ch)
	ch <- e.qmgrInsertsNrcpt.Desc()
	ch <- e.qmgrInsertsSize.Desc()
	ch <- e.qmgrRemoves.Desc()
	ch <- e.qmgrExpires.Desc()
	e.smtpDelays.Describe(ch)
	e.smtpTLSConnects.Describe(ch)
	ch <- e.smtpDeferreds.Desc()
	e.smtpProcesses.Describe(ch)
	ch <- e.smtpdConnects.Desc()
	ch <- e.smtpdDisconnects.Desc()
	ch <- e.smtpdFCrDNSErrors.Desc()
	e.smtpdLostConnections.Describe(ch)
	e.smtpdProcesses.Describe(ch)
	e.smtpdRejects.Describe(ch)
	ch <- e.smtpdSASLAuthenticationFailures.Desc()
	e.smtpdTLSConnects.Describe(ch)
	ch <- e.smtpStatusDeferred.Desc()
	e.unsupportedLogEntries.Describe(ch)
	e.smtpConnectionTimedOut.Describe(ch)
	e.opendkimSignatureAdded.Describe(ch)
	ch <- e.bounceNonDelivery.Desc()
	ch <- e.virtualDelivered.Desc()
}

func (e *PostfixCollector) StartMetricCollection(ctx context.Context) {

	if e.targetLogfile == nil {
		return
	}

	gaugeVec := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "postfix",
			Subsystem: "",
			Name:      "up",
			Help:      "Whether scraping Postfix's metrics was successful.",
		},
		[]string{"path"})
	gauge := gaugeVec.WithLabelValues(e.targetLogfile.Path())
	defer gauge.Set(0)

	for {
		line, err := e.targetLogfile.Read(ctx)
		if err != nil {
			if err != io.EOF {
				log.Printf("Couldn't read journal: %v", err)
			}
			return
		}
		e.CollectFromLogLine(line)
		gauge.Set(1)
	}
}

// Collect metrics from Postfix's showq socket and its log file.
func (e *PostfixCollector) Collect(ch chan<- prometheus.Metric) {

	err := CollectShowqFromSocket(e.targetShowqPath, ch)
	if err == nil {
		ch <- prometheus.MustNewConstMetric(
			postfixUpDesc,
			prometheus.GaugeValue,
			1.0,
			e.targetShowqPath)
	} else {
		log.Printf("Failed to scrape showq socket: %s", err)
		ch <- prometheus.MustNewConstMetric(
			postfixUpDesc,
			prometheus.GaugeValue,
			0.0,
			e.targetShowqPath)
	}

	if e.targetLogfile == nil {
		return
	}
	// added:
	ch <- e.msgsUnknownUnsupported
	ch <- e.msgsAcceptedIn
	ch <- e.msgsNoQueue
	ch <- e.msgsSent
	ch <- e.sSmtpdConnects
	ch <- e.sSmtpdDisconnects
	ch <- e.sQmgrOperations
	ch <- e.msgsBounced
	ch <- e.msgsDeferredTries
	ch <- e.msgsCleanupLines
	e.sOpenDKIM.Collect(ch)
	//ch <- e.individualDefers

	// previous:
	ch <- e.cleanupProcesses
	ch <- e.cleanupRejects
	ch <- e.cleanupNotAccepted
	e.lmtpDelays.Collect(ch)
	e.pipeDelays.Collect(ch)
	ch <- e.qmgrInsertsNrcpt
	ch <- e.qmgrInsertsSize
	ch <- e.qmgrRemoves
	ch <- e.qmgrExpires
	e.smtpDelays.Collect(ch)
	e.smtpTLSConnects.Collect(ch)
	ch <- e.smtpDeferreds
	e.smtpProcesses.Collect(ch)
	ch <- e.smtpdConnects
	ch <- e.smtpdDisconnects
	ch <- e.smtpdFCrDNSErrors
	e.smtpdLostConnections.Collect(ch)
	e.smtpdProcesses.Collect(ch)
	e.smtpdRejects.Collect(ch)
	ch <- e.smtpdSASLAuthenticationFailures
	e.smtpdTLSConnects.Collect(ch)
	ch <- e.smtpStatusDeferred
	e.unsupportedLogEntries.Collect(ch)
	ch <- e.smtpConnectionTimedOut
	e.opendkimSignatureAdded.Collect(ch)
	ch <- e.bounceNonDelivery
	ch <- e.virtualDelivered
}
