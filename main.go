package main

import (
	"strings"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"net/http"
	"os"
	"strconv"
	"time"
	"path"

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
	Suspended    bool   `json:"suspended"`
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

type HistoryProcessDefinitionActivityResult struct {
	ActivityId    string `json:"id"`
	Instances     int    `json:"instances"`
	Canceled      int    `json:"canceled"`
	Finished      int    `json:"finished"`
	CompleteScope int    `json:"completeScope"`
}

type ProcessDefinitionStatisticsActivityResult struct {
	ActivityId string `json:"id"`
	Instances  int    `json:"instances"`
	FailedJobs int    `json:"failedJobs"`
	//Incidents
}

var (
	serverUrl          string
	restPrefix         string
	verbose            bool
	shouldFetchRuntime bool
	shouldFetchHistory bool
	shouldFetchMetrics bool
	user               string
	password           string

	httpClient = http.Client{
		Timeout: time.Second * 5,
	}

	historyIncidentsCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "camunda_history_incidents_total",
			Help: "Number of history incidents within a Camunda server",
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

	processActivityInstancesCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "camunda_process_activity_instances_total",
			Help: "Number of instances for a specific activity",
		},
		[]string{"activityId", "definitionKey", "definitionId", "definitionVersion"},
	)
	
	processActivityFailedJobsCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "camunda_process_activity_failed_jobs_total",
			Help: "Number of failed jobs for a specific activity",
		},
		[]string{"activityId", "definitionKey", "definitionId", "definitionVersion"},
	)
	
	historyProcessActivityInstancesCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "camunda_history_process_activity_instances_total",
			Help: "Number of instances of a specific activity in the history",
		},
		[]string{"activityId", "definitionKey", "definitionId", "definitionVersion"},
	)

	historyProcessActivityCanceledCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "camunda_history_process_activity_canceled_total",
			Help: "Number of canceled activities for a specific activity in the history",
		},
		[]string{"activityId", "definitionKey", "definitionId", "definitionVersion"},
	)

	historyProcessActivityFinishedCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "camunda_history_process_activity_finished_total",
			Help: "Number of finished activities for a specific activity in the history",
		},
		[]string{"activityId", "definitionKey", "definitionId", "definitionVersion"},
	)

	historyProcessActivityCompleteScopeCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "camunda_history_process_activity_complete_scope_total",
			Help: "Number of CompleteScope activities for a specific activity in the history",
		},
		[]string{"activityId", "definitionKey", "definitionId", "definitionVersion"},
	)

	requestCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "camunda_scrape_requests_total",
			Help: "Number of http requests made to the Camunda APIs.",
		},
		[]string{"httpStatusCode"},
	)
	errorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "camunda_scrape_errors_total",
			Help: "Number of errors while accessing the Camunda APIs.",
		},
		[]string{"name"},
	)
	scrapeDurationCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "camunda_scrape_duration_seconds",
			Help: "Duration of a scrape in seconds",
		},
		[]string{"name"},
	)
)

func fetchJson(anUrl string, data interface{}) error {
	if !strings.HasPrefix(anUrl, "http") {
		u, err := url.Parse(serverUrl)
		if err != nil {
			return err
		}
		u2, err2 := url.Parse(anUrl)
		if err2 != nil {
			return err2
		}
		u.Path = path.Join(restPrefix, u2.Path)
		u.RawQuery = u2.RawQuery
		anUrl = u.String()
	}
	req, err := http.NewRequest(http.MethodGet, anUrl, nil)
	req.SetBasicAuth(user, password)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, getErr := httpClient.Do(req)
	if getErr != nil {
		return getErr
	}
	//TODO: should we allow 404?
	if verbose {
		log.Printf("%s -> %d\n", anUrl, res.StatusCode)
	}
	requestCounter.With(prometheus.Labels{"httpStatusCode": strconv.Itoa(res.StatusCode)}).Inc()
	if res.StatusCode != 200 {
		return errors.New(fmt.Sprintf("%s => %d", anUrl, res.StatusCode))
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

func fetchHistoryIncidents(status string) (int, error) {
	// https://docs.camunda.org/manual/7.6/reference/rest/history/incident/get-incident-query-count/
	url := fmt.Sprintf("/history/incident/count?%s=true", status)
	metric := MetricCount{}
	err := fetchJson(url, &metric)
	if err != nil {
		log.Printf("Could not fetch count of %s incidents: %s\n", status, err)
		return 0, err
	}
	if verbose {
		log.Printf("%d %s incidents\n", metric.Count, status)
	}
	return metric.Count, nil
}

func collectHistoryIncidents(statuses ...string) error {
	var hasErrors = false
	for _, status := range statuses {
		incidentCount, err := fetchHistoryIncidents(status)
		if err != nil {
			hasErrors = true
			errorCounter.With(prometheus.Labels{"name": "incidents"}).Inc()
		} else {
			historyIncidentsCounter.With(prometheus.Labels{"status": status}).Set(float64(incidentCount))
		}
	}
	if hasErrors {
		return errors.New("Could not process all statuses")
	}
	return nil
}

func fetchMetrics(maxResults int, startDate string) ([]Metric, error) {
	// https://docs.camunda.org/manual/7.6/reference/rest/metrics/get-metrics-interval/
	url := fmt.Sprintf("/metrics?maxResults=%d", maxResults)
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
	// https://docs.camunda.org/manual/7.7/reference/rest/process-definition/get-statistics/
	url := "/process-definition/statistics?failedJobs=true"
	result := []ProcessDefinitionStatisticsResult{}
	err := fetchJson(url, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func fetchProcessDefinitionActivities(processDefinitionId string) ([]ProcessDefinitionStatisticsActivityResult, error) {
	// https://docs.camunda.org/manual/7.6/reference/rest/process-definition/get-activity-statistics/
	url := fmt.Sprintf("/process-definition/%s/statistics?failedJobs=true", processDefinitionId)
	result := []ProcessDefinitionStatisticsActivityResult{}
	err := fetchJson(url, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func fetchHistoryProcessDefinitionActivities(processDefinitionId string) ([]HistoryProcessDefinitionActivityResult, error) {
	// https://docs.camunda.org/manual/7.6/reference/rest/history/process-definition/get-historic-activity-statistics/
	url := fmt.Sprintf("/history/process-definition/%s/statistics?canceled=true&finished=true&completeScope=true", processDefinitionId)
	result := []HistoryProcessDefinitionActivityResult{}
	err := fetchJson(url, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func collectProcessDefinitionActivities(processDefinition ProcessDefinition) error {
	stats, err := fetchProcessDefinitionActivities(processDefinition.Id)
	if err != nil {
		errorCounter.With(prometheus.Labels{"name": "fetchProcessDefinitionActivities"}).Inc()
		return err
	}
	if verbose {
		log.Printf("Found %d activities for process definition %s\n", len(stats), processDefinition.Id)
	}
	for _, stat := range stats {
		labels := prometheus.Labels{
			"activityId":        stat.ActivityId,
			"definitionId":      processDefinition.Id,
			"definitionKey":     processDefinition.Key,
			"definitionVersion": strconv.Itoa(processDefinition.Version)}
	
		processActivityInstancesCounter.With(labels).Set(float64(stat.Instances))
		processActivityFailedJobsCounter.With(labels).Set(float64(stat.FailedJobs))
		if verbose {
			//log.Printf("%s: %d instances / %d failedJobs\n", stat.Definition, stat.Instances, stat.FailedJobs)
		}
	}

	// Same as previously but in the History
	if shouldFetchHistory {
		historyStats, historyErr := fetchHistoryProcessDefinitionActivities(processDefinition.Id)
		if historyErr != nil {
			errorCounter.With(prometheus.Labels{"name": "fetchHistoryProcessDefinitionActivities"}).Inc()
			return err
		}
		if verbose {
			log.Printf("History: Found %d activities for process definition %s\n", len(historyStats), processDefinition.Id)
		}
		for _, historyStat := range historyStats {
			labels := prometheus.Labels{
				"activityId":        historyStat.ActivityId,
				"definitionId":      processDefinition.Id,
				"definitionKey":     processDefinition.Key,
				"definitionVersion": strconv.Itoa(processDefinition.Version)}
		
			historyProcessActivityInstancesCounter.With(labels).Set(float64(historyStat.Instances))
			historyProcessActivityCanceledCounter.With(labels).Set(float64(historyStat.Canceled))
			historyProcessActivityFinishedCounter.With(labels).Set(float64(historyStat.Finished))
			historyProcessActivityCompleteScopeCounter.With(labels).Set(float64(historyStat.CompleteScope))
			if verbose {
				//log.Printf("%s: %d instances / %d failedJobs\n", stat.Definition, stat.Instances, stat.FailedJobs)
			}
		}	
	}

	return nil
}

func collectProcessDefinitionStatistics() error {
	var stats []ProcessDefinitionStatisticsResult
	err := measureTime("fetchProcessDefinitionStatistics", func () error {
		var err error
		stats, err = fetchProcessDefinitionStatistics()
		return err
	})
	if err != nil {
		errorCounter.With(prometheus.Labels{"name": "fetchProcessDefinitionStatistics"}).Inc()
		return err
	}
	if verbose {
		log.Printf("Found %d process definition stats\n", len(stats))
	}
	for _, stat := range stats {
		if stat.Definition.Suspended {
			if verbose {
				log.Printf("Skip process definition %s because it is suspended\n", stat.Definition.Id)
			}
			continue
		}
		/* Note sure if we can simply skip stats without instances or failed jobs.
		if stat.Instances + stat.FailedJobs == 0 {
			if verbose {
				log.Printf("Skip process definition %s because there are no instances nor failed jobs\n", stat.Definition.Id)
			}
			continue
		}*/
		labels := prometheus.Labels{
			"id":                stat.Id,
			"definitionId":      stat.Definition.Id,
			"definitionKey":     stat.Definition.Key,
			"definitionVersion": strconv.Itoa(stat.Definition.Version),
			"deploymentId":      stat.Definition.DeploymentId,
			"tenantId":          stat.Definition.TenantId}
		processInstancesCounter.With(labels).Set(float64(stat.Instances))
		processFailedJobsCounter.With(labels).Set(float64(stat.FailedJobs))
		if verbose {
			//log.Printf("%s: %d instances / %d failedJobs\n", stat.Definition, stat.Instances, stat.FailedJobs)
		}
		err = measureTime("collectProcessDefinitionActivities", func () error {
			return collectProcessDefinitionActivities(stat.Definition)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func fetchForShortTimer() error {
	return measureTime("fetchForShortTimer", func () error {
		if shouldFetchHistory {
			err := measureTime("collectHistoryIncidents", func () error {
				return collectHistoryIncidents("open", "deleted", "resolved")
			})
			if err != nil {
				return err
			}
		}
		if shouldFetchRuntime {
			err := measureTime("collectProcessDefinitionStatistics", func () error {
				return collectProcessDefinitionStatistics()
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func fetchForLongTimer() error {
	if shouldFetchMetrics {
		return measureTime("fetchForLongTimer", func () error {
			_, errMetrics := collectMetrics()
			if errMetrics != nil {
				return errMetrics
			}
			return nil
		})
	}
	return nil
}

func measureTime(name string, action func () error) error {
	if(verbose) {
		log.Printf("------> Measuring %s", name)
	}
	start := time.Now()
	err := action()
	elapsed := time.Since(start)
	scrapeDurationCounter.With(prometheus.Labels{"name": name}).Set(elapsed.Seconds())
	if(verbose) {
		log.Printf("======> Elapsed time for %s scrape: %s", name, elapsed)
	}
	return err
}

func init() {
	// Metrics have to be registered to be exposed:
	prometheus.MustRegister(historyIncidentsCounter)
	prometheus.MustRegister(historyProcessActivityCanceledCounter)
	prometheus.MustRegister(historyProcessActivityCompleteScopeCounter)
	prometheus.MustRegister(historyProcessActivityFinishedCounter)
	prometheus.MustRegister(historyProcessActivityInstancesCounter)
	prometheus.MustRegister(metricsCounter)
	prometheus.MustRegister(errorCounter)
	prometheus.MustRegister(scrapeDurationCounter)
	prometheus.MustRegister(requestCounter)
	prometheus.MustRegister(processInstancesCounter)
	prometheus.MustRegister(processFailedJobsCounter)
	prometheus.MustRegister(processActivityInstancesCounter)
	prometheus.MustRegister(processActivityFailedJobsCounter)
}

func main() {
	flag.StringVar(&serverUrl, "server", "", "The Camunda server URI")
	flag.StringVar(&restPrefix, "restPrefix", "rest", "The REST prefix used to access the Camunda API")
	port := flag.Int("port", 8080, "The http port the server will listen on")
	shortInterval := flag.Duration("shortInterval", time.Second*30, "The interval between 2 incidents scrapes")
	longInterval := flag.Duration("longInterval", time.Minute*15, "The interval between 2 metrics scrapes")
	flag.BoolVar(&verbose, "verbose", false, "Should we log the metrics?")
	flag.BoolVar(&shouldFetchRuntime, "fetch-runtime", false, "Should we fetch runtime data?")
	flag.BoolVar(&shouldFetchHistory, "fetch-history", false, "Should we fetch history data?")
	flag.BoolVar(&shouldFetchMetrics, "fetch-metrics", false, "Should we fetch metrics?")
	flag.StringVar(&user, "user", "", "The Camunda API user")
	flag.StringVar(&password, "password", "", "The Camunda API password")

	flag.Parse()

	// Validate flags
	if serverUrl == "" {
		fmt.Println("You must specify the Camunda server URI!")
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
			log.Println("Short timer event")
			fetchForShortTimer()
		}
	}()
	if shouldFetchMetrics {
		longTicker := time.NewTicker(*longInterval)
		go func() {
			for range longTicker.C {
				log.Println("Long timer event")
				fetchForLongTimer()
			}
		}()
		defer longTicker.Stop()
	}
	
	// The Handler function provides a default handler to expose metrics
	// via an HTTP server. "/metrics" is the usual endpoint for that.
	http.Handle("/metrics", promhttp.Handler())
	log.Printf("Listening on port %d\n", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
