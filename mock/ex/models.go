package main

// This file contains models/util for the exppublish.go program, for which
// that source file contains primary logic.
// The constructors (new*) should be used to instantiate the structs.

import (
	"encoding/json"
	"expvar"
	"fmt"
)

const (
	postNoQueue = "NOQUEUE"
	postSmtpd   = "postfix/smtpd"
	postSmtp    = "postfix/smtp"
	postFast    = "postfix-fast/smtp"
	postSlow    = "postfix-slow/smtp"
	postMed     = "postfix-medium/smtp"
	postRes     = "postfix-restrictive/smtp"
	postSent    = "status=sent"
	postDefer   = "status=deferred"
	postBounce  = "status=bounced"
)

type MessageCounter struct {
	Accepted_in    *expvar.Int
	Sent           *expvar.Int
	No_queue       *expvar.Int
	Bounced        *expvar.Int
	Deferred_tries *expvar.Int
}

func NewMessageCounter() *MessageCounter {
	return &MessageCounter{
		// NewInt calls Publish as part of its creation
		Accepted_in:    expvar.NewInt("Accepted in"),
		Sent:           expvar.NewInt("Sent"),
		No_queue:       expvar.NewInt("No queue"),
		Bounced:        expvar.NewInt("Bounced"),
		Deferred_tries: expvar.NewInt("Deferred tries"),
	}
}

type NoQueueAddrs struct {
	NoQueues map[string]bool
}

type IndividDefers struct {
	Individual_Defers map[string]int
}

func NewNoQueueAddrs() *NoQueueAddrs {
	m := make(map[string]bool, 1)
	return &NoQueueAddrs{
		NoQueues: m,
	}
}

func NewIndividDefers() *IndividDefers {
	m := make(map[string]int, 1)
	return &IndividDefers{
		Individual_Defers: m,
	}
}

func (d *NoQueueAddrs) addNoQueue(a string) {
	d.NoQueues[a] = true
}

func (d *IndividDefers) addDefer(a string) {
	d.Individual_Defers[a]++
}

func (d *NoQueueAddrs) String() string {
	v, _ := json.Marshal(d.NoQueues)
	return string(v)
}

func (d *IndividDefers) String() string {
	v, _ := json.Marshal(d.Individual_Defers)
	return string(v)
}

func infoLine(a ...any) {
	if *fInfo {
		fmt.Println("Info Line ->: ", a)
	}
}
