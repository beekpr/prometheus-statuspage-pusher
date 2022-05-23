package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

var prometheusTimes = promauto.NewHistogram(prometheus.HistogramOpts{
	Name: "statuspage_pusher_prometheus_requests",
	Help: "The response times of prometheus requests",
})

var statuspageTimes = promauto.NewHistogram(prometheus.HistogramOpts{
	Name: "statuspage_pusher_statuspage_requests",
	Help: "The response times of statuspage.io requests",
})

var (
	prometheusURL    = flag.String("prom", "http://localhost:9090", "URL of Prometheus server")
	statusPageAPIKey = flag.String("apikey", "", "Statuspage API key")
	statusPageID     = flag.String("pageid", "", "Statuspage page ID")
	queryConfigFile  = flag.String("config", "queries.yaml", "Query config file")
	metricInterval   = flag.Duration("interval", 300*time.Second, "Metric push interval")
	logLevel         = flag.String("log-level", "info", "Log level accepted by Logrus, for example, \"error\", \"warn\", \"info\", \"debug\", ...")

	httpClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	queryConfig map[string]string
)

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
	_, _ = fmt.Fprintf(w, "OK")
}

func main() {
	flag.Parse()
	if lvl, err := log.ParseLevel(*logLevel); err != nil {
		log.Fatal(err)
	} else {
		log.SetLevel(lvl)
	}

	qcd, err := ioutil.ReadFile(*queryConfigFile)
	if err != nil {
		log.Fatalf("Couldn't read config file: %s", err)
	}
	if err := yaml.Unmarshal(qcd, &queryConfig); err != nil {
		log.Fatalf("Couldn't parse config file: %s", err)
	}

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/healthz", healthz)
	go http.ListenAndServe(":8080", nil)

	ticker := time.NewTicker(*metricInterval)
	for {
		select {
		case <-ticker.C:
			go queryAndPush()
		}
	}
}

func queryAndPush() {
	log.Infof("Started to query and pushing metrics")

	start := time.Now()
	metrics := queryPrometheus()

	prometheusTimes.Observe(time.Since(start).Seconds())

	for id, val := range metrics {
		pushStatuspage(id, val[0])
	}

	log.Infof("Finished querying and pushing metrics")
}

func pushStatuspage(id string, status Status) error {
	jsonContents, err := json.Marshal(Component{Status: status})
	if err != nil {
		return err
	}

	log.Debugf("Metrics payload pushing to Statuspage: %s", jsonContents)

	log.Infof("Pushing metric: %s", id)
	url := fmt.Sprintf("https://api.statuspage.io/v1/pages/%s/components/%s", url.PathEscape(*statusPageID), url.PathEscape(id))
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonContents))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "OAuth "+*statusPageAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respStr, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("HTTP status %d, Empty API response", resp.StatusCode)
		}
		return fmt.Errorf("HTTP status %d, API error: %s", resp.StatusCode, string(respStr))
	}

	return nil
}
