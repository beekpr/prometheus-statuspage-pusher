package main

import (
	"context"
	"fmt"
	"math"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/api"
	prometheus "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

func queryPrometheus() componentStatus {
	client, err := api.NewClient(api.Config{Address: *prometheusURL})
	if err != nil {
		log.Fatalf("Couldn't create Prometheus client: %s", err)
	}
	api := prometheus.NewAPI(client)

	metrics := make(componentStatus)
	for metricID, query := range queryConfig {
		ctxlog := log.WithFields(log.Fields{
			"metric_id": metricID,
		})

		var (
			err          error
			warnings     prometheus.Warnings
			metricPoints []Status
		)

		metricPoints, warnings, err = queryInstant(api, query, ctxlog)

		for _, w := range warnings {
			ctxlog.Warnf("Prometheus query warning: %s", w)
		}

		if err != nil {
			ctxlog.Error(err)
			continue
		}

		metrics[metricID] = metricPoints
	}

	return metrics
}

func queryInstant(api prometheus.API, query string, logger *log.Entry) ([]Status, prometheus.Warnings, error) {
	now := time.Now()
	response, warnings, err := api.Query(context.Background(), query, now)

	if err != nil {
		return nil, warnings, fmt.Errorf("Couldn't query Prometheus: %w", err)
	}

	if response.Type() != model.ValVector {
		return nil, warnings, fmt.Errorf("Expected result type %s, got %s", model.ValVector, response.Type())
	}

	vec := response.(model.Vector)
	if l := vec.Len(); l != 1 {
		return nil, warnings, fmt.Errorf("Expected single time serial, got %d", l)
	}

	value := vec[0].Value
	logger.Infof("Query result: %s", value)

	status := "operational"

	if value == 1 {
		status = "operational"
	} else {
		status = "major_outage"
	}

	if math.IsNaN(float64(value)) {
		return nil, warnings, fmt.Errorf("Invalid metric value NaN")
	}

	return []Status{
		{
			Status: status,
		},
	}, warnings, nil
}
