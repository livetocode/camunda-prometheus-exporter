FROM alpine:latest  
RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY ./build/app camunda-prometheus-exporter

ENTRYPOINT ["./camunda-prometheus-exporter"]  
