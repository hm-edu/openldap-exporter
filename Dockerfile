FROM golang:1.27.3@sha256:6f1acc898d064a4e9d5773ee574a9af6a9b786306939ac14d7fd806bf62046db
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o openldap-exporter

FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
COPY --from=0 /app/openldap-exporter /usr/local/bin/openldap-exporter