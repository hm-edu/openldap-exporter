module github.com/hm-edu/openldap-exporter/v2

go 1.27

toolchain go1.27.4

require (
	github.com/go-ldap/ldap/v3 v3.4.15
	github.com/prometheus/client_golang v1.25.0
	github.com/prometheus/exporter-toolkit v0.21.0
	github.com/tomcz/gotools v0.21.1
	github.com/urfave/cli-altsrc/v3 v3.1.0
	github.com/urfave/cli/v3 v3.14.0
)

require (
	github.com/Azure/go-ntlmssp v0.1.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mdlayher/socket v0.8.0 // indirect
	github.com/mdlayher/vsock v1.4.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pandatix/go-abnf v0.5.5 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	github.com/prometheus/common v0.74.0 // indirect
	github.com/prometheus/procfs v0.22.0 // indirect
	go.yaml.in/yaml/v2 v2.4.5 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace gopkg.in/asn1-ber.v1 => github.com/go-asn1-ber/asn1-ber v1.5.8
