# camunda-prometheus-exporter

## Description

This exporter will call the Camunda Rest APIs in order to collect metrics on the running workflows.

It will then create the following Prometheus metrics:

- camunda_incidents_total: Number of incidents within a Camunda server
- camunda_scrape_errors_total: Number of errors while accessing the Camunda APIs

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


