package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricCount struct {
	Count int `json:"count"`
}

type Metric struct {
	Timestamp string `json:"timestamp"`
	Name      string `json:"name"`
	Reporter  string `json:"reporter"`
	Value     int    `json:"value"`
}

type ProcessDefinition struct {
	Id           string `json:"id"`
	Key          string `json:"key"`
	Category     string `json:"category"`
	Description  string `json:"description"`
	Name         string `json:"name"`
	Version      int    `json:"version"`
	Resource     string `json:"resource"`
	DeploymentId string `json:"deploymentId"`
	TenantId     string `json:"tenantId"`
	VersionTag   string `json:"versionTag"`
}

func (def ProcessDefinition) String() string {
	if def.Key == "" {
		return "<EmptyDefinition>"
	}
	return fmt.Sprintf("%s@%d", def.Key, def.Version)
}

type ProcessDefinitionStatisticsResult struct {
	Id         string            `json:"id"`
	Instances  int               `json:"instances"`
	FailedJobs int               `json:"failedJobs"`
	Definition ProcessDefinition `json:"definition"`
}

var (
	serverUrl string
	verbose   bool

	httpClient = http.Client{
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

	processInstancesCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "camunda_process_instances_total",
			Help: "Number of instances of a specific Process",
		},
		[]string{"id", "definitionId", "definitionKey", "definitionVersion", "deploymentId", "tenantId"},
	)

	processFailedJobsCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "camunda_process_failed_jobs_total",
			Help: "Number of failed jobs for a specific Process",
		},
		[]string{"id", "definitionId", "definitionKey", "definitionVersion", "deploymentId", "tenantId"},
	)

	processActivitiesCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "camunda_process_activities_total",
			Help: "Number of instances of a specific activity",
		},
		[]string{"activityId", "activityName", "definitionKey"},
	)

	errorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "camunda_scrape_errors_total",
			Help: "Number of errors while accessing the Camunda APIs.",
		},
		[]string{"name"},
	)
)

func fetchJson(url string, data interface{}) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, getErr := httpClient.Do(req)
	if getErr != nil {
		return getErr
	}
	if res.StatusCode != 200 {
		return errors.New(fmt.Sprintf("%s => %d", url, res.StatusCode))
	}
	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		return readErr
	}

	jsonErr := json.Unmarshal(body, data)
	if jsonErr != nil {
		return jsonErr
	}
	return nil
}

func fetchIncidents(status string) (int, error) {
	url := fmt.Sprintf("%s/engine-rest/history/incident/count?%s=true", serverUrl, status)
	metric := MetricCount{}
	err := fetchJson(url, &metric)
	if err != nil {
		log.Printf("Could not fetch count of %s incidents: %s\n", status, err)
		errorCounter.With(prometheus.Labels{"name": "incidents"}).Inc()
		return 0, err
	}
	if verbose {
		log.Printf("%d %s incidents\n", metric.Count, status)
	}
	incidentsCounter.With(prometheus.Labels{"status": status}).Set(float64(metric.Count))
	return metric.Count, nil
}

func fetchMultipleIncidents(statuses ...string) error {
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

	metrics := []Metric{}
	err := fetchJson(url, &metrics)
	if err != nil {
		return nil, err
	}
	return metrics, nil
}

func collectMetrics() ([]Metric, error) {
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
		if verbose {
			log.Printf("metric %s %s = %d\n", metric.Timestamp, metric.Name, metric.Value)
		}
	}
	return metrics, nil
}

func fetchProcessDefinitionStatistics() ([]ProcessDefinitionStatisticsResult, error) {
	url := fmt.Sprintf("%s/engine-rest/process-definition/statistics?failedJobs=true", serverUrl)
	result := []ProcessDefinitionStatisticsResult{}
	err := fetchJson(url, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func createCounterLabelsFromStats(stat ProcessDefinitionStatisticsResult) prometheus.Labels {
	return prometheus.Labels{
		"id":                stat.Id,
		"definitionId":      stat.Definition.Id,
		"definitionKey":     stat.Definition.Key,
		"definitionVersion": strconv.Itoa(stat.Definition.Version),
		"deploymentId":      stat.Definition.DeploymentId,
		"tenantId":          stat.Definition.TenantId}
}

func collectProcessDefinitionStatistics() error {
	stats, err := fetchProcessDefinitionStatistics()
	if err != nil {
		errorCounter.With(prometheus.Labels{"name": "processDefinitionStatistics"}).Inc()
		return err
	}
	if verbose {
		log.Printf("Found %d process definition stats\n", len(stats))
	}
	for _, stat := range stats {
		labels := createCounterLabelsFromStats(stat)
		processInstancesCounter.With(labels).Set(float64(stat.Instances))
		processFailedJobsCounter.With(labels).Set(float64(stat.FailedJobs))
		if verbose {
			//log.Printf("%s: %d instances / %d failedJobs\n", stat.Definition, stat.Instances, stat.FailedJobs)
		}
	}
	return nil
}

func fetchActivityInstanceCount(activityId string, activityName string, definitionKey string) (int, error) {
	url := fmt.Sprintf("%s/engine-rest/history/activity-instance/count?activityId=%s", serverUrl, activityId)
	metric := MetricCount{}
	err := fetchJson(url, &metric)
	if err != nil {
		return 0, err
	}
	return metric.Count, nil
}

func collectActivityInstanceCount(activityId string, activityName string, definitionKey string) (int, error) {
	count, err := fetchActivityInstanceCount(activityId, activityName, definitionKey)
	if err != nil {
		log.Printf("Could not fetch count of %s activities: %s\n", activityId, err)
		errorCounter.With(prometheus.Labels{"name": "activities"}).Inc()
		return 0, err
	}
	if verbose {
		log.Printf("%s (%s) = %d\n", activityName, activityId, count)
	}
	processActivitiesCounter.With(prometheus.Labels{"activityId": activityId, 
		"activityName": activityName, 
		"definitionKey": definitionKey}).Set(float64(count))
	return count, nil
}

func collectActivities() error {
	_, err := collectActivityInstanceCount("StartEvent_0l8qdec", "Service Requests Created", "ProcessOnlineServiceRequest")
	if err != nil {
		return err
	}
	_, err = collectActivityInstanceCount("EndEvent_0tgip1s", "Service Requests Closed", "ProcessOnlineServiceRequest")
	if err != nil {
		return err
	}
	_, err = collectActivityInstanceCount("ServiceTask_1010dsd", "Authenticated Service Requests", "ProcessOnlineServiceRequest")
	if err != nil {
		return err
	}
	_, err = collectActivityInstanceCount("EndEvent_154enzk", "Anonymous Service Requests", "ProcessOnlineServiceRequest")
	if err != nil {
		return err
	}
	_, err = collectActivityInstanceCount("EndEvent_1n9otub", "Anonymous Service Requests", "ProcessOnlineServiceRequest")
	if err != nil {
		return err
	}
	_, err = collectActivityInstanceCount("ServiceTask_0yuo3ru", "Subscriptions created", "ProcessOnlineServiceRequest")
	if err != nil {
		return err
	}
	_, err = collectActivityInstanceCount("SendTask_1fjvapz", "Send Email", "ProcessOnlineServiceRequest_emailSend")
	if err != nil {
		return err
	}
	_, err = collectActivityInstanceCount("EndEvent_1uexl7r", "Individuals Requested Not to be notified", "ProcessOnlineServiceRequest_emailSend")
	if err != nil {
		return err
	}
	return nil
}

func fetchForShortTimer() error {
	err := fetchMultipleIncidents("open", "deleted", "resolved")
	if err != nil {
		return err
	}
	err = collectProcessDefinitionStatistics()
	if err != nil {
		return err
	}
	err = collectActivities()
	if err != nil {
		return err
	}
	return nil
}

func fetchForLongTimer() error {
	_, errMetrics := collectMetrics()
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
	prometheus.MustRegister(processInstancesCounter)
	prometheus.MustRegister(processFailedJobsCounter)
	prometheus.MustRegister(processActivitiesCounter)
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
	if fetchForShortTimer() != nil || fetchForLongTimer() != nil {
		log.Println("Could not fetch all the stats. Exiting now!")
		os.Exit(2)
	}
	// Start ticker to collect the HipChat room stats
	log.Println("Starting the tickers")
	shortTicker := time.NewTicker(*shortInterval)
	go func() {
		for range shortTicker.C {
			fetchForShortTimer()
		}
	}()
	longTicker := time.NewTicker(*longInterval)
	go func() {
		for range longTicker.C {
			fetchForLongTimer()
		}
	}()
	defer longTicker.Stop()

	// The Handler function provides a default handler to expose metrics
	// via an HTTP server. "/metrics" is the usual endpoint for that.
	http.Handle("/metrics", promhttp.Handler())
	log.Printf("Listening on port %d\n", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
