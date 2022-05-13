package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/common/model"
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
	prometheusURL       = flag.String("prom", "http://localhost:9090", "URL of Prometheus server")
	statusPageAPIKey    = flag.String("apikey", "", "Statuspage API key")
	statusPageID        = flag.String("pageid", "", "Statuspage page ID")
	queryConfigFile     = flag.String("config", "queries.yaml", "Query config file")
	metricInterval      = flag.Duration("interval", 30*time.Second, "Metric push interval")
	metricValueRounding = flag.Uint("rounding", 6, "Round metric values to specific decimal places")
	backfillDuration    = flag.String("backfill", "", "Backfill the data points in, for example, 5d")
	logLevel            = flag.String("log-level", "info", "Log level accepted by Logrus, for example, \"error\", \"warn\", \"info\", \"debug\", ...")

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

	if *backfillDuration != "" {
		md, err := model.ParseDuration(*backfillDuration)
		if err != nil {
			log.Fatalf("Incorrect duration format: %s", *backfillDuration)
		}
		d := time.Duration(md)
		queryAndPush(&d)
	} else {
		queryAndPush(nil)
	}

	ticker := time.NewTicker(*metricInterval)
	for {
		select {
		case <-ticker.C:
			go queryAndPush(nil)
		}
	}
}

func queryAndPush(backfill *time.Duration) {
	log.Infof("Started to query and pushing metrics")

	start := time.Now()
	metrics := queryPrometheus(backfill, *metricValueRounding)
	prometheusTimes.Observe(time.Since(start).Seconds())
	chunkedMetrics := chunkMetrics(metrics)

	for _, m := range chunkedMetrics {
		start = time.Now()
		err := pushStatuspage(m)
		statuspageTimes.Observe(time.Since(start).Seconds())
		if err != nil {
			log.Error(err)
		}
	}

	log.Infof("Finished querying and pushing metrics")
}

func pushStatuspage(metrics statuspageMetrics) error {
	jsonContents, err := json.Marshal(statuspageMetricsPayload{Data: metrics})
	if err != nil {
		return err
	}

	log.Debugf("Metrics payload pushing to Statuspage: %s", jsonContents)

	metricIDs := make([]string, 0, len(metrics))
	for id := range metrics {
		metricIDs = append(metricIDs, id)
	}

	log.Infof("Pushing metrics: %s", strings.Join(metricIDs, ", "))

	url := fmt.Sprintf("https://api.statuspage.io/v1/pages/%s/metrics/data", url.PathEscape(*statusPageID))
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonContents))
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
