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

type MessageDetails struct {
	No_queue_addresses map[string]bool
	//Individual_defers  map[string]int
}

func NewMessageDetails() *MessageDetails {
	return &MessageDetails{
		No_queue_addresses: map[string]bool{},
		//Individual_defers:  map[string]int{},
	}
}

func (d *MessageDetails) addNoQueue(a string) {
	for k, _ := range d.No_queue_addresses {
		if a == k {
			return
		}
	}
	d.No_queue_addresses[a] = true
}

func (d *MessageDetails) String() string {
	v, _ := json.Marshal(d.No_queue_addresses)
	return string(v)
}

//func (d *MessageDetails) addDefer(a string) {
//	d.Individual_defers[a]++
//}

// to then be used with Publish, and will use Func for v Var on these field names

func infoLine(a ...any) {
	if *fInfo {
		fmt.Println("Info Line ->: ", a)
	}
}
