package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	//_ "net/http/pprof"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ctx          = context.Background()
	fAddress     = flag.String("listen", ":9003", "Address on which to listen")
	fMetricsPath = flag.String("path", "/metrics", "Path on which to provide metrics")
)

func main() {
	flag.Parse()
	if *fAddress == "" || *fMetricsPath == "" {
		log.Fatalln("Required parameters not provided")
	}

	theBuckets := []float64{0, 1, 2, 4, 8, 16}

	whyIsHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "why",
		Name:      "what_is_this_count",
		Help:      "Testing",
		Buckets:   theBuckets,
	})

	prometheus.MustRegister(whyIsHistogram)
	http.Handle(*fMetricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`
			<html>
			<head><title>Postfix Exporter</title></head>
			<body>
			<h1>WSS SE Postfix Exporter</h1>
			<p><a href='` + *fMetricsPath + `'>Metrics</a></p>
			</body>
			</html>`))
		if err != nil {
			log.Fatalln("Error in HTTP Handler func! ->: ", err)
		}
	})

	c := 1.0
	go func() {
		for _ = range 20 {
			time.Sleep(2 * time.Second)
			log.Println("Add number: ", c)
			whyIsHistogram.Observe(c)
			c += 5.2
		}
	}()

	log.Fatal(http.ListenAndServe(*fAddress, nil))
}
