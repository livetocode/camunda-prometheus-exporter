FROM golang:1.9

WORKDIR /go/src/livetocode/camunda-prometheus-exporter
COPY main.go .

RUN go-wrapper download github.com/prometheus/client_golang/prometheus && \
    go-wrapper install && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=0 /go/src/livetocode/camunda-prometheus-exporter/app camunda-prometheus-exporter

ENTRYPOINT ["./camunda-prometheus-exporter"]
