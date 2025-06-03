package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ctx = context.Background()
	// NOTE: flags could be adjusted for better 'drop-in' replacement
	targetLogfile = flag.String("logfile", "/var/log/maillog", "Full path to mail log file for parsing")

	targetDebug = flag.Bool("debug", false, "Enable printing of arbitrary debug messages to stdout for troubleshooting purposes")
	//targetLogSnooze = flag.String("sleep-time", "5", "Seconds to sleep after hitting EOF on mail log file")
	targetListenAddr  = flag.String("listen-address", ":9003", "Address on which to listen for scraping")
	targetMetricsPath = flag.String("metrics-path", "/metrics", "Path on which to expose metrics")
	targetShowqPath   = flag.String("showq-path", "/var/spool/postfix/public/showq", "Path to Postfix showq socket")
)

func init() {
	flag.Parse()
	if *targetLogfile == "" || *targetListenAddr == "" || *targetMetricsPath == "" || *targetShowqPath == "" {
		log.Fatalln("Expected parameters were not provided. Quitting")
	}
}

func main() {
	infoLine("Printing of extra info lines is enabled")

	//logSrc, err := NewLogSourceFromFactories(ctx)
	//if err != nil {
	//	log.Fatalf("Error opening log source: %s", err)
	//}
	//defer logSrc.Close()

	exporter, err := NewPostfixExporter(
		*targetShowqPath,
		*targetLogfile,
		//*logUnsupportedLines,
	)
	if err != nil {
		log.Fatalf("Failed to create PostfixExporter: %s", err)
	}
	prometheus.MustRegister(exporter)

	http.Handle(*targetMetricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err = w.Write([]byte(`
			<html>
			<head><title>Postfix Exporter</title></head>
			<body>
			<h1>WSS SE Postfix Exporter</h1>
			<p><a href='` + *targetMetricsPath + `'>Metrics</a></p>
			</body>
			</html>`))
		if err != nil {
			log.Fatalln("Error in HTTP Handler func! ->: ", err)
		}
	})
	ctx, cancelFunc := context.WithCancel(ctx)
	infoLine("Initialize context")
	defer cancelFunc()

	go exporter.StartMetricCollection(ctx)
	infoLine("Started goroutine for metric collection")

	log.Print("Listening on ", *targetListenAddr)
	log.Fatal(http.ListenAndServe(*targetListenAddr, nil))
}

func infoLine(a ...any) {
	if *targetDebug == true {
		log.Println("Debug Line ->: ")
	}
}
