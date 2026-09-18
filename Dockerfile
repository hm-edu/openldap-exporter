FROM golang:1.27.1
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o openldap-exporter

FROM alpine:3.24.2@sha256:3cf95fe0816180395592b8373f3ec60663f076127617bbacb4eacf9667afe2e9
COPY --from=0 /app/openldap-exporter /usr/local/bin/openldap-exporter