# OpenLDAP Prometheus Exporter

`openldap_exporter` scrapes metrics from OpenLDAP and exposes them over HTTP for Prometheus.

It reads the OpenLDAP monitor backend, exports general slapd counters and operations, and can also expose replication status and MDB database page usage. The exporter is based on the ideas in https://github.com/jcollie/openldap_exporter, but is written in Go for simpler distribution and installation.

## Quick start

Enable the OpenLDAP monitor backend, run the exporter, and point Prometheus at `http://<exporter-host>:9330/metrics`.

Download a binary from the [latest release](https://github.com/hm-edu/openldap-exporter/releases), build it locally, or run the container image published by this repository.

```sh
openldap_exporter \
  --ldap-addr ldap://localhost:389 \
  --ldap-user "cn=monitoring,cn=Monitor" \
  --ldap-pass "secret"
```

Check that metrics are available:

```sh
curl -s http://localhost:9330/metrics
```

## OpenLDAP setup

`slapd` supports an optional LDAP monitoring interface that exposes the current state of the server. Documentation for this backend can be found in the OpenLDAP [backend guide](http://www.openldap.org/doc/admin24/backends.html#Monitor) and [administration guide](http://www.openldap.org/doc/admin24/monitoringslapd.html).

To enable the backend, add this to `slapd.conf`:

```
database monitor
rootdn "cn=monitoring,cn=Monitor"
rootpw secret
```

Technically `rootdn` and `rootpw` are optional, but authenticated monitor access is strongly recommended.

If your installation loads backends as modules, also load the monitor backend:

```
moduleload back_monitor
```

## Metrics

The exporter exposes metrics with the `openldap_` prefix. Core monitor metrics include:

| Metric | Labels | Description |
| --- | --- | --- |
| `openldap_monitor_counter_object` | `dn` | Counter-style values from `monitorCounterObject` entries, such as connections, bytes, entries, and waiters. |
| `openldap_monitor_operation` | `dn` | Completed operation counters from `cn=Operations,cn=Monitor`. |
| `openldap_monitored_object` | `dn` | Numeric values from `monitoredObject` entries, such as thread and uptime data. |
| `openldap_scrape` | `result` | Exporter scrape attempts against LDAP. |
| `openldap_bind` | `result` | LDAP bind attempts performed by the exporter. |
| `openldap_dial` | `result` | LDAP connection attempts performed by the exporter. |

Example output:

```prometheus
# HELP openldap_monitor_counter_object cn=Monitor (objectClass=monitorCounterObject) monitorCounter
# TYPE openldap_monitor_counter_object gauge
openldap_monitor_counter_object{dn="cn=Bytes,cn=Statistics,cn=Monitor"} 1.857812777e+09
openldap_monitor_counter_object{dn="cn=Current,cn=Connections,cn=Monitor"} 50
openldap_monitor_counter_object{dn="cn=Entries,cn=Statistics,cn=Monitor"} 4.226632e+06
openldap_monitor_counter_object{dn="cn=Max File Descriptors,cn=Connections,cn=Monitor"} 1024
openldap_monitor_counter_object{dn="cn=PDU,cn=Statistics,cn=Monitor"} 4.446117e+06
openldap_monitor_counter_object{dn="cn=Read,cn=Waiters,cn=Monitor"} 31
openldap_monitor_counter_object{dn="cn=Referrals,cn=Statistics,cn=Monitor"} 0
openldap_monitor_counter_object{dn="cn=Total,cn=Connections,cn=Monitor"} 65383
openldap_monitor_counter_object{dn="cn=Write,cn=Waiters,cn=Monitor"} 0
# HELP openldap_monitor_operation cn=Operations,cn=Monitor (objectClass=monitorOperation) monitorOpCompleted
# TYPE openldap_monitor_operation gauge
openldap_monitor_operation{dn="cn=Abandon,cn=Operations,cn=Monitor"} 0
openldap_monitor_operation{dn="cn=Add,cn=Operations,cn=Monitor"} 0
openldap_monitor_operation{dn="cn=Bind,cn=Operations,cn=Monitor"} 57698
openldap_monitor_operation{dn="cn=Compare,cn=Operations,cn=Monitor"} 0
openldap_monitor_operation{dn="cn=Delete,cn=Operations,cn=Monitor"} 0
openldap_monitor_operation{dn="cn=Extended,cn=Operations,cn=Monitor"} 0
openldap_monitor_operation{dn="cn=Modify,cn=Operations,cn=Monitor"} 0
openldap_monitor_operation{dn="cn=Modrdn,cn=Operations,cn=Monitor"} 0
openldap_monitor_operation{dn="cn=Search,cn=Operations,cn=Monitor"} 161789
openldap_monitor_operation{dn="cn=Unbind,cn=Operations,cn=Monitor"} 9336
# HELP openldap_monitored_object cn=Monitor (objectClass=monitoredObject) monitoredInfo
# TYPE openldap_monitored_object gauge
openldap_monitored_object{dn="cn=Active,cn=Threads,cn=Monitor"} 1
openldap_monitored_object{dn="cn=Backload,cn=Threads,cn=Monitor"} 1
openldap_monitored_object{dn="cn=Max Pending,cn=Threads,cn=Monitor"} 0
openldap_monitored_object{dn="cn=Max,cn=Threads,cn=Monitor"} 16
openldap_monitored_object{dn="cn=Open,cn=Threads,cn=Monitor"} 8
openldap_monitored_object{dn="cn=Pending,cn=Threads,cn=Monitor"} 0
openldap_monitored_object{dn="cn=Starting,cn=Threads,cn=Monitor"} 0
openldap_monitored_object{dn="cn=Uptime,cn=Time,cn=Monitor"} 1.225737e+06
# HELP openldap_scrape successful vs unsuccessful ldap scrape attempts
# TYPE openldap_scrape counter
openldap_scrape{result="ok"} 6985
...
```

## Configuration

You can configure `openldap_exporter` with command line flags, environment variables, and an optional YAML file. All sources are optional; defaults are used when no value is provided.

Configuration precedence, from highest to lowest:

1. Command line flags
2. Environment variables
3. YAML configuration file parameters
4. Default values

Common options:

| Flag | Environment | YAML key | Default | Description |
| --- | --- | --- | --- | --- |
| `--prom-addr` | `PROM_ADDR` | `prom-addr` | `:9330` | Bind address for the HTTP metrics server. |
| `--metrics-path` | `METRICS_PATH` | `metrics-path` | `/metrics` | HTTP path for Prometheus metrics. |
| `--ldap-addr` | `LDAP_ADDR` | `ldap-addr` | `ldap://localhost:389` | LDAP server URL. `ldap://` and `ldaps://` are supported. |
| `--ldap-user` | `LDAP_USER` | `ldap-user` | | Optional LDAP bind DN. |
| `--ldap-pass` | `LDAP_PASS` | `ldap-pass` | | Optional LDAP bind password. |
| `--interval` | `INTERVAL` | `interval` | `30s` | LDAP scrape interval. |
| `--json-log` | `JSON_LOG` | `json-log` | `false` | Emit JSON logs. |
| `--config` | | | | Read configuration from a YAML file. |

Replication-specific flags are documented in the replication section below.

Example with environment variables and a YAML file:

```
INTERVAL=10s /usr/sbin/openldap_exporter --prom-addr ":8080" --config /etc/slapd/exporter.yaml
```

Where `exporter.yaml` looks like this:

```yaml
---
ldap-user: "cn=monitoring,cn=Monitor"
ldap-pass: "secret"
```

Example Prometheus scrape configuration:

```yaml
scrape_configs:
  - job_name: openldap
    static_configs:
      - targets:
          - ldap-provider.example.org:9330
```

## Replication monitoring

Replication monitoring reads the `contextCSN` attribute from the replicated naming context. Enable it by setting `--replication-object` to the base DN you want to watch:

```sh
openldap_exporter \
  --ldap-addr ldap://ldap-provider.example.org:389 \
  --ldap-user "cn=monitoring,cn=Monitor" \
  --ldap-pass "secret" \
  --replication-object "dc=example,dc=org"
```

This adds `openldap_monitor_replication` samples. The `id` label is the OpenLDAP server ID from the `contextCSN` value, and the `type` label exposes the parsed CSN parts:

```prometheus
openldap_monitor_replication{id="001",type="gt"} 1768463445
openldap_monitor_replication{id="001",type="count"} 12
openldap_monitor_replication{id="001",type="mod"} 0
```

`type="gt"` is the CSN timestamp as Unix seconds. `type="count"` and `type="mod"` are the change counter and modifier fields from the same CSN.

To compare a provider with replicas, also pass the provider server ID and one or more replica LDAP URLs:

```sh
openldap_exporter \
  --ldap-addr ldap://ldap-provider.example.org:389 \
  --ldap-user "cn=monitoring,cn=Monitor" \
  --ldap-pass "secret" \
  --replication-object "dc=example,dc=org" \
  --server-id 1 \
  --replication-server ldap://ldap-replica-1.example.org:389 \
  --replication-server ldap://ldap-replica-2.example.org:389
```

For multi-master replication, run one exporter per master. Each exporter should connect to one local master with `--ldap-addr`, use that master's `--server-id`, and list the other masters with `--replication-server`. A single exporter can report whether its configured master is ahead of or behind the peers for that server ID, but it cannot provide the full cluster view for every master by itself.

This adds `openldap_replication_delta`. The value is the provider CSN timestamp minus the replica CSN timestamp, in seconds, for the configured `--server-id`. A positive value means the provider is ahead of that replica; a negative value means the replica reports a newer CSN for that server ID.

```prometheus
openldap_replication_delta{replica="ldap://ldap-replica-1.example.org:389"} 3
openldap_replication_delta{replica="ldap://ldap-replica-2.example.org:389"} 42
```

Useful PromQL examples:

```promql
# Seconds since the latest observed provider CSN per server ID.
time() - openldap_monitor_replication{type="gt"}

# Largest replica lag reported by this exporter.
max(openldap_replication_delta)

# Alert-style expression for replicas more than five minutes behind.
openldap_replication_delta > 300
```

## MDB monitoring

MDB page data is collected automatically from `cn=Databases,cn=Monitor` when the OpenLDAP monitor backend exposes `olmMDBDatabase` entries. No extra exporter flags are required beyond normal monitor access.

Example `/metrics` output:

```prometheus
openldap_pages_free{context="dc=example,dc=org"} 1024
openldap_pages_used{context="dc=example,dc=org"} 24576
openldap_pages_max{context="dc=example,dc=org"} 1048576
```

The `context` label is the database naming context. The values come from `olmMDBPagesFree`, `olmMDBPagesUsed`, and `olmMDBPagesMax`.

Useful PromQL examples:

```promql
# MDB usage percentage per naming context.
100 * openldap_pages_used / openldap_pages_max

# Remaining MDB pages before hitting the configured maximum.
openldap_pages_max - openldap_pages_used

# Free pages inside the MDB file, by naming context.
openldap_pages_free

# Alert-style expression when an MDB database is above 80% of max pages.
openldap_pages_used / openldap_pages_max > 0.8
```

## Build

Install Go 1.26 or newer, then build the exporter:

```sh
make build
```

The local binary is written to `target/openldap_exporter`. Cross-platform release artifacts are built with:

```sh
make commit
```
