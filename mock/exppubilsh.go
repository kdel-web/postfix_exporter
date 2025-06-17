package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"time"
)

const (
	postNoQueue      = "NOQUEUE"
	postSmtpd        = "postfix/smtpd"
	postSmtp         = "postfix/smtp"
	postFast         = "postfix-fast/smtp"
	postSlow         = "postfix-slow/smtp"
	postMed          = "postfix-medium/smtp"
	postRes          = "postfix-restrictive/smtp"
	postClean        = "postfix/cleanup"
	postDiscard      = "postfix/discard"
	postQmgr         = "postfix/qmgr"
	postSent         = "status=sent"
	postDefer        = "status=deferred"
	postBounce       = "status=bounced"
	postOpendkim     = "opendkim"
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
				time.Sleep(5 * time.Second)
				continue
			}
			log.Fatalf("fileRunner: Error -> %q", err)
		}
		lines <- line
	}
}


