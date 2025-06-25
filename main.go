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
	// NOTE: third-library `kingpin` has been removed. In order to better support drop-in functionality,
	// the flags have been retained; this includes the inconsistency between hyphens (-) and underscores (_).
	cmdLogfile     = flag.String("postfix.logfile_path", "/var/log/postfix_exporter_input.log", "Full path to mail log file for parsing")
	cmdDebug       = flag.Bool("debug", false, "Enable printing of arbitrary debug messages to stdout for troubleshooting purposes")
	cmdListenAddr  = flag.String("web.listen-address", ":9101", "Address on which to listen for scraping")
	cmdMetricsPath = flag.String("web.telemetry-path", "/metrics", "Path on which to expose metrics")
	cmdShowqPath   = flag.String("postfix.showq_path", "/var/spool/postfix/public/showq", "Path to Postfix showq socket")
)

func init() {
	flag.Parse()
	if *cmdLogfile == "" || *cmdListenAddr == "" || *cmdMetricsPath == "" {
		log.Fatalln("Expected parameters were not provided. Quitting")
	}
}

func main() {
	infoLine("Printing of extra info lines is enabled")

	maillogFile, err := NewFileLogSource(ctx, *cmdLogfile)
	if err != nil {
		log.Fatalf("Error opening log source: %s", err)
	}
	defer maillogFile.Close()

	exporter, err := NewPostfixCollector(
		*cmdShowqPath,
		maillogFile,
		true,
	)
	if err != nil {
		log.Fatalf("Failed to create PostfixExporter: %s", err)
	}

	prometheus.MustRegister(exporter)

	http.Handle(*cmdMetricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err = w.Write([]byte(`
			<html>
			<head><title>Postfix Exporter</title></head>
			<body>
			<h1>WSS SE Postfix Exporter</h1>
			<p><a href='` + *cmdMetricsPath + `'>Metrics</a></p>
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

	log.Print("Listening on ", *cmdListenAddr)
	log.Fatal(http.ListenAndServe(*cmdListenAddr, nil))
}

func infoLine(a ...any) {
	if *cmdDebug == true {
		log.Println("Debug Line ->: ", a)
	}
}
