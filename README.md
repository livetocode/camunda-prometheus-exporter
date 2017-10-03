# camunda-prometheus-exporter

## Description

This exporter will call the Camunda Rest APIs in order to collect metrics on the running workflows.

It will then create the following Prometheus metrics:

- camunda_metrics_total: Camunda metrics
- camunda_process_instances_total: Number of instances of a specific Process
- camunda_process_failed_jobs_total: Number of failed jobs for a specific Process
- camunda_process_activity_instances_total: Number of instances for a specific activity
- camunda_process_activity_failed_jobs_total: Number of failed jobs for a specific activity
- camunda_history_incidents_total: NNumber of history incidents within a Camunda server
- camunda_history_process_activity_instances_total: Number of instances of a specific activity in the history
- camunda_history_process_activity_canceled_total: umber of canceled activities for a specific activity in the history
- camunda_history_process_activity_finished_total: Number of finished activities for a specific activity in the history
- camunda_history_process_activity_complete_scope_total: Number of CompleteScope activities for a specific activity in the history
- camunda_scrape_requests_total: Number of http requests made to the Camunda APIs.
- camunda_scrape_errors_total: Number of errors while accessing the Camunda APIs
- camunda_scrape_duration_seconds: Duration of a scrape in seconds

Note that each metric will have a **"name"** property containing the Room's name.

## Requirements

You need access to a Camunda server that exposes its engine-rest endpoint

## Build

To create a local docker image, execute:

```
./scripts/build-image.sh
```

## Run

Once you have the image built and your AuthTojen, you can run it in Docker locally for testing it:

```
./scripts/run-image.sh -server http://my.camunda.server:8080
```

And then you can access the metrics:

```
open http://localhost:8080/metrics
```


## Kubernetes

Use the Helm Chart for installing it.

See the [README](charts/camunda-prometheus-exporter/README.md)


