package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"sync"
	"time"
)

const (
	// test expect very fast response
	google  = "google.com"
	gmail   = "gmail.com"
	yahoo   = "yahoo.com"
	comcast = "comcast.net"
	web     = "webstaurantstore.com"
	trs     = "restaurantstore.com"

	// test bad
	yandex = "yandex.com"

	// smtp servers
	defaultSmtpServer = "home.mta.backends.tech"
	//secondarySmtpServer = "prod1.mta.backends.tech"
	//backupSmtpServer    = "prod2.mta.backends.tech"
	//localhostSmtp       = "localhost"
	smtpPort = "25"
)

var (
	// flags
	fdebug bool
	fquiet bool
	fSend  bool
	// smtp servers
	smtpServers     = []string{defaultSmtpServer}
	DestNotifyEmail = "kdellinger@webstaurantstore.com"
)

func init() {
	flag.BoolVar(&fdebug, "debug", false, "Enable printing of informational lines to stdout")
	flag.BoolVar(&fdebug, "d", false, "Enable printing of informational lines to stdout")
	flag.BoolVar(&fquiet, "quiet", false, "Do not print any results")
	flag.BoolVar(&fquiet, "q", false, "Do not print any results")
	flag.BoolVar(&fSend, "send", false, "Send email")
	flag.BoolVar(&fSend, "s", false, "Send email")

	flag.Parse()
}

func main() {

	//var results []string
	var checkDomains []string
	var badDomains []string
	var wg sync.WaitGroup

	ctx, cancelFunc := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelFunc()
	if len(flag.Args()) == 2 {
		checkDomains = os.Args[1:]
	} else {
		infoLine("No names provided, using defaults")
		checkDomains = []string{google, gmail, yahoo, comcast, web, trs, yandex}
	}
	resolv := &net.Resolver{
		PreferGo:     true,
		StrictErrors: false,
	}

	for _, h := range checkDomains {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			r, err := resolv.LookupMX(ctx, h)
			if err != nil {
				infoLine("Error lookup: ", err)
				badDomains = append(badDomains, h)
			}
			for _, a := range r {
				infoLine("Received expected response from:", a.Host)
			}
		}(h)
	}

	wg.Wait()

	if fSend {
		fmt.Println("Sending email to:", DestNotifyEmail)
		tryServersNotify(DestNotifyEmail, "timetime", "This is an informational email")
	}

	if !fquiet {
		fmt.Println("Program complete")
		fmt.Println("Domains checked:", len(checkDomains))
		fmt.Println("Domains success:", len(checkDomains)-len(badDomains))
		fmt.Println("Domains failure:", len(badDomains))
		fmt.Println("Failed domains:", badDomains)
		if fSend {

		}
	}
}

func serverPort(server, port string) string {
	infoLine(server + ":" + port)
	return server + ":" + port
}

func smtpMsgBody(to []string, subject, body string) []byte {
	msg := fmt.Sprint("To: ", to, "\r\n", "Subject: ", subject, "\r\n\r\n", body, "\r\n")
	return []byte(msg)
}

func tryServersNotify(destemail, subject, body string) {
	to := []string{destemail}
	bmsgbody := smtpMsgBody(to, subject, body)
	fromAddr := getHostNameAt()
	for _, s := range smtpServers {
		usingSmtp := serverPort(s, smtpPort)
		auth := smtp.PlainAuth("", fromAddr, "", s)
		if err := smtp.SendMail(usingSmtp, auth, fromAddr, to, bmsgbody); err == nil {
			infoLine("send mail using:", usingSmtp)
			break
		} else {
			continue
		}
	}
}

func getHostNameAt() string {
	h, err := os.Hostname()
	if err != nil {
		fmt.Println("Warning: Unable to obtain hostname:", err)
		return "unknown@localhost"
	}
	return h
}

func infoLine(a ...any) {
	if fdebug && !fquiet {
		fmt.Println("Info Line: ", a)
	}
}
