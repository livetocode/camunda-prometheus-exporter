package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricCount struct {  
    Count int `json:"count"`
}

type Metric struct {
    Timestamp 	string `json:"timestamp"`
    Name 		string `json:"name"`
    Reporter 	string `json:"reporter"`
    Value 		int `json:"value"`
}

var (
	serverUrl string
	verbose bool
	
	httpClient = http.Client {
		Timeout: time.Second * 5, 
	}
	
	incidentsCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "camunda_incidents_total",
			Help: "Number of incidents within a Camunda server",
		},
		[]string{"status"},
	)

	metricsCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "camunda_metrics_total",
			Help: "Camunda metrics",
		},
		[]string{"name"},
	)

	errorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "camunda_scrape_errors_total",
			Help: "Number of errors while accessing the Camunda APIs.",
		},
		[]string{"name"},
	)
)

func fetchCount(api string) (int, error) {
	url := serverUrl + api
	req, err := http.NewRequest(http.MethodGet, url, nil)
    if err != nil {
		return 0, err
    }
	res, getErr := httpClient.Do(req)
    if getErr != nil {
        return 0, getErr
    }

    body, readErr := ioutil.ReadAll(res.Body)
    if readErr != nil {
        return 0, readErr
    }

	metric := MetricCount {}
    jsonErr := json.Unmarshal(body, &metric)
    if jsonErr != nil {
        return 0, jsonErr
	}

	return metric.Count, nil
}

func fetchIncidents(status string) (int, error) {
	count, err := fetchCount(fmt.Sprintf("/engine-rest/history/incident/count?%s=true", status))
	if err != nil {
		log.Printf("Could not fetch count of %s incidents: %s\n", status, err)
		errorCounter.With(prometheus.Labels{"name": "incidents"}).Inc()
		return 0, err
	}
	if verbose {
		log.Printf("%d %s incidents\n", count, status)
	}
	incidentsCounter.With(prometheus.Labels{"status": status}).Set(float64(count))
	return count, nil
}

func fetchMultipleIncidents(statuses []string) error {
	var hasErrors = false
	for _, status := range statuses {
		if _, err := fetchIncidents(status); err != nil {
			hasErrors = true
		}
	}
	if hasErrors {
		return errors.New("Could not process all statuses")
	}
	return nil
}

func fetchMetrics(maxResults int, startDate string) ([]Metric, error) {
	url := fmt.Sprintf("%s/engine-rest/metrics?maxResults=%d", serverUrl, maxResults)
	if startDate != "" {
		url += fmt.Sprintf("&startDate=%s", startDate)
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
    if err != nil {
		return nil, err
    }
	res, getErr := httpClient.Do(req)
    if getErr != nil {
        return nil, getErr
    }

    body, readErr := ioutil.ReadAll(res.Body)
    if readErr != nil {
        return nil, readErr
    }

	metrics := []Metric {}
    jsonErr := json.Unmarshal(body, &metrics)
    if jsonErr != nil {
        return nil, jsonErr
	}
	return metrics, nil
}

func fetchLatestMetrics() ([]Metric, error) {
	// first, get only one metric in order to grab the timestamp
	metrics, err := fetchMetrics(1, "")
	if err != nil {
		errorCounter.With(prometheus.Labels{"name": "metrics"}).Inc()
		return nil, err
	}
	if len(metrics) == 0 {
		return metrics, nil
	}
	// then get the metrics with the most recent timestamp
	metrics, err = fetchMetrics(100, metrics[0].Timestamp)
	if err != nil {
		errorCounter.With(prometheus.Labels{"name": "metrics"}).Inc()
		return nil, err
	}
	if verbose {
		log.Printf("%d metrics\n", len(metrics))
	}
	for _, metric := range metrics {
		metricsCounter.With(prometheus.Labels{"name": metric.Name}).Set(float64(metric.Value))
		log.Printf("metric %s %s = %d\n", metric.Timestamp, metric.Name, metric.Value)
	}
	return metrics, nil
}

func fetchForShort() error {
	statuses := []string {"open", "closed", "resolved"}
	err := fetchMultipleIncidents(statuses)
	if err != nil {
		return err
	}
	return nil	
}

func fetchForLong() error {
	_, errMetrics := fetchLatestMetrics()
	if errMetrics != nil {
		return errMetrics
	}
	return nil	
}

func init() {
	// Metrics have to be registered to be exposed:
	prometheus.MustRegister(incidentsCounter)
	prometheus.MustRegister(metricsCounter)
	prometheus.MustRegister(errorCounter)
}

func main() {
	flag.StringVar(&serverUrl, "server", "", "The Camunda server")
	port := flag.Int("port", 8080, "The http port the server will listen on")
	shortInterval := flag.Duration("shortInterval", time.Second*30, "The interval between 2 incidents scrapes")
	longInterval := flag.Duration("longInterval", time.Minute*15, "The interval between 2 metrics scrapes")
	flag.BoolVar(&verbose, "verbose", false, "Should we log the metrics?")

	flag.Parse()

	// Validate flags
	if serverUrl == "" {
		fmt.Println("You must specify the Camunda server!")
		fmt.Println()
		flag.Usage()
		os.Exit(1)
	}
	// Get initial stats
	log.Println("Fetching initial metrics")
	if fetchForShort() != nil || fetchForLong() != nil {
		log.Println("Could not fetch all the stats. Exiting now!")
		os.Exit(2)
	}
	// Start ticker to collect the HipChat room stats
	log.Println("Starting the tickers")
	shortTicker := time.NewTicker(*shortInterval)
	go func() {
		for range shortTicker.C {
			fetchForShort()
		}
	}()
	longTicker := time.NewTicker(*longInterval)
	go func() {
		for range longTicker.C {
			fetchForLong()
		}
	}()
	defer longTicker.Stop()

	// The Handler function provides a default handler to expose metrics
	// via an HTTP server. "/metrics" is the usual endpoint for that.
	http.Handle("/metrics", promhttp.Handler())
	log.Printf("Listening on port %d\n", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
