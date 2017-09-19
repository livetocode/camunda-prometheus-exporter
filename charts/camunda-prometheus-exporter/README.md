# camunda-prometheus-exporter Helm Chart

A Prometheus Exporter that will expose Camunda metrics.

## Chart Details
This chart will do the following:

* Deploy the service
* Auto-register the service with Prometheus by leveraging the Prometheus annotations.

## Installing the Chart

To install the chart with the release name `my-release`:

```bash
$ helm install --name my-release charts/camunda-prometheus-exporter --set camunda.server=http://my.server:8080,camunda.verbose=true
```

## Configuration

The following tables lists the configurable parameters of the Jenkins chart and their default values.


| Parameter                         | Description                          | Default                                                                      |
| --------------------------------- | ------------------------------------ | ---------------------------------------------------------------------------- |
| `hipchat.server`                  | The Camunda server                   |                                                                              |
| `hipchat.shortInterval`           | The time interval between 2 Incidents scrapes  | 30s                                                                          |
| `hipchat.longInterval`            | The time interval between 2 Metrics scrapes  | 15m                                                                            |
| `hipchat.verbose`                 | Should we log the API results?       | false                                                                        |
| `image.repository`                | Image name                           | `livetocode/camunda-prometheus-exporter`                                     |
| `image.tag`                       | Image tag                            | `latest`                                                                     |
| `image.pullPolicy`                | Image pull policy                    | `Always`                                                                     |


Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`.

Alternatively, a YAML file that specifies the values for the parameters can be provided while installing the chart. For example,

```bash
$ helm install --name my-release -f values.yaml charts/camunda-prometheus-exporter
```

And for upgrading:
```bash
helm upgrade my-release charts/camunda-prometheus-exporter/ --set camunda.server=http://my.server:8080,camunda.verbose=true
```

> **Tip**: You can use the default [values.yaml](values.yaml)