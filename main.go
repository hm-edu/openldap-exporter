package main

import (
	"context"
	"errors"
	"fmt"
	log "log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	ldap "github.com/go-ldap/ldap/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/tomcz/gotools/errgroup"
	"github.com/tomcz/gotools/quiet"
	altsrc "github.com/urfave/cli-altsrc/v3"
	"github.com/urfave/cli-altsrc/v3/yaml"
	"github.com/urfave/cli/v3"
)

const (
	promAddr          = "prom-addr"
	ldapAddr          = "ldap-addr"
	ldapUser          = "ldap-user"
	ldapPass          = "ldap-pass"
	interval          = "interval"
	metrics           = "metrics-path"
	jsonLog           = "json-log"
	replicationObject = "replication-object"
	replicationServer = "replication-server"
	serverId          = "server-id"
)

var showStop bool

func main() {

	config := "config"
	sourcer := altsrc.NewStringPtrSourcer(&config)
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:    promAddr,
			Value:   ":9330",
			Usage:   "Bind address for Prometheus HTTP metrics server",
			Sources: cli.NewValueSourceChain(yaml.YAML(promAddr, sourcer), cli.EnvVar("PROM_ADDR")),
		},
		&cli.StringFlag{
			Name:    metrics,
			Value:   "/metrics",
			Usage:   "Path on which to expose Prometheus metrics",
			Sources: cli.NewValueSourceChain(yaml.YAML(metrics, sourcer), cli.EnvVar("METRICS_PATH")),
		},
		&cli.StringFlag{
			Name:    ldapAddr,
			Value:   "localhost:389",
			Usage:   "Address and port of OpenLDAP server",
			Sources: cli.NewValueSourceChain(yaml.YAML(ldapAddr, sourcer), cli.EnvVar("LDAP_ADDR")),
		},
		&cli.StringFlag{
			Name:    ldapUser,
			Usage:   "OpenLDAP bind username (optional)",
			Sources: cli.NewValueSourceChain(yaml.YAML(ldapUser, sourcer), cli.EnvVar("LDAP_USER")),
		},
		&cli.StringFlag{
			Name:    ldapPass,
			Usage:   "OpenLDAP bind password (optional)",
			Sources: cli.NewValueSourceChain(yaml.YAML(ldapPass, sourcer), cli.EnvVar("LDAP_PASS")),
		},
		&cli.DurationFlag{
			Name:    interval,
			Value:   30 * time.Second,
			Usage:   "Scrape interval",
			Sources: cli.NewValueSourceChain(yaml.YAML(interval, sourcer), cli.EnvVar("INTERVAL")),
		},
		&cli.BoolFlag{
			Name:    jsonLog,
			Value:   false,
			Usage:   "Output logs in JSON format",
			Sources: cli.NewValueSourceChain(yaml.YAML(jsonLog, sourcer), cli.EnvVar("JSON_LOG")),
		},
		&cli.StringFlag{
			Name:  replicationObject,
			Usage: "Object to watch replication upon",
		},
		&cli.StringSliceFlag{
			Name:  replicationServer,
			Usage: "The replication servers to watch",
		},
		&cli.IntFlag{
			Name:  serverId,
			Usage: "The id of the server to watch",
		},
		&cli.StringFlag{
			Name:        config,
			Destination: &config,
			Usage:       "Optional configuration from a `YAML_FILE`",
		},
	}
	app := &cli.Command{
		Name:            "openldap_exporter",
		Usage:           "Export OpenLDAP metrics to Prometheus",
		Version:         GetVersion(),
		HideHelpCommand: true,
		Flags:           flags,
		Action:          runMain,
	}
	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Error("service failed", "err", err)
		os.Exit(1)
	}
	if showStop {
		log.Info("service stopped")
	}
}

func runMain(ctx context.Context, c *cli.Command) error {
	showStop = true

	if c.Bool(jsonLog) {
		lh := log.NewJSONHandler(os.Stderr, nil)
		log.SetDefault(log.New(lh))
	}
	log.Info("service starting")

	server := NewMetricsServer(
		c.String(promAddr),
		c.String(metrics),
	)

	scraper := &Scraper{
		Addr:              c.String(ldapAddr),
		User:              c.String(ldapUser),
		Pass:              c.String(ldapPass),
		Tick:              c.Duration(interval),
		Sync:              c.String(replicationObject),
		ServerId:          c.Int(serverId),
		ReplicatonServers: c.StringSlice(replicationServer),
	}

	ctx, cancel := context.WithCancel(context.Background())
	group := errgroup.New()
	group.Go(func() error {
		defer cancel()
		return server.Start()
	})
	group.Go(func() error {
		defer cancel()
		scraper.Start(ctx)
		return nil
	})
	group.Go(func() error {
		defer func() {
			cancel()
			server.Stop()
		}()
		signalChan := make(chan os.Signal, 1)
		signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-signalChan:
			log.Info("shutdown received")
			return nil
		case <-ctx.Done():
			return nil
		}
	})
	return group.Wait()
}

// ===============================================================
// Metrics Scraper
// ===============================================================

const (
	baseDN    = "cn=Monitor"
	opsBaseDN = "cn=Operations,cn=Monitor"

	monitorCounterObject = "monitorCounterObject"
	monitorCounter       = "monitorCounter"

	monitoredObject = "monitoredObject"
	monitoredInfo   = "monitoredInfo"

	monitorOperation   = "monitorOperation"
	monitorOpCompleted = "monitorOpCompleted"

	monitorReplicationFilter = "contextCSN"
	monitorReplication       = "monitorReplication"
	monitorReplicationDelta  = "monitorReplicationDelta"

	// Properties for MDB monitoring
	dbBaseDN          = "cn=Databases,cn=Monitor"
	monitoredDatabase = "olmMDBDatabase"
	mdbFreePages      = "olmMDBPagesFree"
	mdbUsedPages      = "olmMDBPagesUsed"
	mdbMaxPages       = "olmMDBPagesMax"
)

type query struct {
	baseDN         string
	scope          int
	searchFilter   string
	searchAttr     string
	additionalAttr []string
	metric         *prometheus.GaugeVec
	setData        func([]*ldap.Entry, *query)
}

var (
	monitoredObjectGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: "openldap",
			Name:      "monitored_object",
			Help:      help(baseDN, objectClass(monitoredObject), monitoredInfo),
		},
		[]string{"dn"},
	)
	monitorCounterObjectGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: "openldap",
			Name:      "monitor_counter_object",
			Help:      help(baseDN, objectClass(monitorCounterObject), monitorCounter),
		},
		[]string{"dn"},
	)
	monitorOperationGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: "openldap",
			Name:      "monitor_operation",
			Help:      help(opsBaseDN, objectClass(monitorOperation), monitorOpCompleted),
		},
		[]string{"dn"},
	)
	bindCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "openldap",
			Name:      "bind",
			Help:      "successful vs unsuccessful ldap bind attempts",
		},
		[]string{"result"},
	)
	dialCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "openldap",
			Name:      "dial",
			Help:      "successful vs unsuccessful ldap dial attempts",
		},
		[]string{"result"},
	)
	scrapeCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "openldap",
			Name:      "scrape",
			Help:      "successful vs unsuccessful ldap scrape attempts",
		},
		[]string{"result"},
	)
	monitorReplicationGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: "openldap",
			Name:      "monitor_replication",
			Help:      help(baseDN, monitorReplication),
		},
		[]string{"id", "type"},
	)
	monitorReplicationDeltaGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: "openldap",
			Name:      "replication_delta",
			Help:      help(baseDN, monitorReplicationDelta),
		},
		[]string{"replica"},
	)
	pagesFreeGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: "openldap",
			Name:      "pages_free",
			Help:      help(baseDN, objectClass(monitoredDatabase), mdbFreePages),
		},
		[]string{"context"},
	)
	pagesUsedGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: "openldap",
			Name:      "pages_used",
			Help:      help(baseDN, objectClass(monitoredDatabase), mdbUsedPages),
		},
		[]string{"context"},
	)
	pagesMaxGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: "openldap",
			Name:      "pages_max",
			Help:      help(baseDN, objectClass(monitoredDatabase), mdbMaxPages),
		},
		[]string{"context"},
	)
	queries = []*query{
		{
			baseDN:       baseDN,
			scope:        ldap.ScopeWholeSubtree,
			searchFilter: objectClass(monitoredObject),
			searchAttr:   monitoredInfo,
			metric:       monitoredObjectGauge,
			setData:      setValue,
		}, {
			baseDN:       baseDN,
			scope:        ldap.ScopeWholeSubtree,
			searchFilter: objectClass(monitorCounterObject),
			searchAttr:   monitorCounter,
			metric:       monitorCounterObjectGauge,
			setData:      setValue,
		},
		{
			baseDN:       opsBaseDN,
			scope:        ldap.ScopeWholeSubtree,
			searchFilter: objectClass(monitorOperation),
			searchAttr:   monitorOpCompleted,
			metric:       monitorOperationGauge,
			setData:      setValue,
		},
		{
			baseDN:       opsBaseDN,
			scope:        ldap.ScopeWholeSubtree,
			searchFilter: objectClass(monitorOperation),
			searchAttr:   monitorOpCompleted,
			metric:       monitorOperationGauge,
			setData:      setValue,
		},
		{
			baseDN:         dbBaseDN,
			scope:          ldap.ScopeWholeSubtree,
			searchFilter:   objectClass(monitoredDatabase),
			searchAttr:     mdbFreePages,
			additionalAttr: []string{"namingContexts"},
			metric:         pagesFreeGauge,
			setData:        setValueDb,
		},
		{
			baseDN:         dbBaseDN,
			scope:          ldap.ScopeWholeSubtree,
			searchFilter:   objectClass(monitoredDatabase),
			searchAttr:     mdbUsedPages,
			additionalAttr: []string{"namingContexts"},
			metric:         pagesUsedGauge,
			setData:        setValueDb,
		},
		{
			baseDN:         dbBaseDN,
			scope:          ldap.ScopeWholeSubtree,
			searchFilter:   objectClass(monitoredDatabase),
			searchAttr:     mdbMaxPages,
			additionalAttr: []string{"namingContexts"},
			metric:         pagesMaxGauge,
			setData:        setValueDb,
		},
	}
)

func init() {
	prometheus.MustRegister(
		monitoredObjectGauge,
		monitorCounterObjectGauge,
		monitorOperationGauge,
		monitorReplicationGauge,
		monitorReplicationDeltaGauge,
		pagesFreeGauge,
		pagesUsedGauge,
		pagesMaxGauge,
		scrapeCounter,
		bindCounter,
		dialCounter,
	)
}

func help(msg ...string) string {
	return strings.Join(msg, " ")
}

func objectClass(name string) string {
	return fmt.Sprintf("(objectClass=%v)", name)
}

func setValueDb(entries []*ldap.Entry, q *query) {
	for _, entry := range entries {
		val := entry.GetAttributeValue(q.searchAttr)
		if val == "" {
			// not every entry will have this attribute
			continue
		}
		num, err := strconv.ParseFloat(val, 64)
		if err != nil {
			// some of these attributes are not numbers
			continue
		}
		q.metric.WithLabelValues(entry.GetAttributeValue("namingContexts")).Set(num)
	}
}

func setValue(entries []*ldap.Entry, q *query) {
	for _, entry := range entries {
		val := entry.GetAttributeValue(q.searchAttr)
		if val == "" {
			// not every entry will have this attribute
			continue
		}
		num, err := strconv.ParseFloat(val, 64)
		if err != nil {
			// some of these attributes are not numbers
			continue
		}
		q.metric.WithLabelValues(entry.DN).Set(num)
	}
}

type Scraper struct {
	Addr              string
	User              string
	Pass              string
	Tick              time.Duration
	log               *log.Logger
	Sync              string
	ServerId          int
	ReplicatonServers []string
}

func (s *Scraper) Start(ctx context.Context) {
	s.log = log.With("component", "scraper")
	s.addReplicationQueries()
	s.log.Info("starting monitor loop", "addr", s.Addr)
	ticker := time.NewTicker(s.Tick)
	defer ticker.Stop()
	s.scrape()
	for {
		select {
		case <-ticker.C:
			s.scrape()
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scraper) addReplicationQueries() {

	if s.Sync != "" {
		queries = append(queries,
			&query{
				baseDN:       s.Sync,
				scope:        ldap.ScopeBaseObject,
				searchFilter: "(contextCSN=*)",
				searchAttr:   monitorReplicationFilter,
				metric:       monitorReplicationGauge,
				setData:      s.setReplicationValue,
			},
		)
	}

}

func (s *Scraper) setReplicationValue(entries []*ldap.Entry, q *query) {

	var replica []ReplicaStatus
	if len(s.ReplicatonServers) != 0 {
		replicaResult := s.scrapeReplication()
		replica = replicaResult
	}
	for _, entry := range entries {
		values := entry.GetAttributeValues(q.searchAttr)
		for _, val := range values {
			if val == "" {
				// not every entry will have this attribute
				continue
			}
			ll := s.log.With(
				"filter", q.searchFilter,
				"attr", q.searchAttr,
				"value", val,
			)
			valueBuffer := strings.Split(val, "#")
			gt, err := time.Parse("20060102150405.999999Z", valueBuffer[0])
			if err != nil {
				ll.Warn("unexpected gt value", "err", err)
				continue
			}
			count, err := strconv.ParseFloat(valueBuffer[1], 64)
			if err != nil {
				ll.Warn("unexpected count value", "err", err)
				continue
			}
			sid := valueBuffer[2]

			sidNo, err := strconv.Atoi(valueBuffer[2])
			if err != nil {
				ll.Warn("unexpected sid value", "err", err)
				continue
			}

			mod, err := strconv.ParseFloat(valueBuffer[3], 64)
			if err != nil {
				ll.Warn("unexpected mod value", "err", err)
				continue
			}
			q.metric.WithLabelValues(sid, "gt").Set(float64(gt.Unix()))
			q.metric.WithLabelValues(sid, "count").Set(count)
			q.metric.WithLabelValues(sid, "mod").Set(mod)
			if len(replica) != 0 && s.ServerId == sidNo {
				for _, rep := range replica {
					delta := gt.Sub(rep.Time).Seconds()
					monitorReplicationDeltaGauge.WithLabelValues(rep.Server).Set(delta)
				}
			}
		}
	}
}

func (s *Scraper) scrape() {
	conn, err := ldap.DialURL(s.Addr)
	if err != nil {
		s.log.Error("dial failed")
		dialCounter.WithLabelValues("fail").Inc()
		return
	}
	dialCounter.WithLabelValues("ok").Inc()
	defer func(connection *ldap.Conn) {
		_ = connection.Close()
	}(conn)

	if s.User != "" && s.Pass != "" {
		err = conn.Bind(s.User, s.Pass)
		if err != nil {
			s.log.Error("bind failed", "err", err)
			bindCounter.WithLabelValues("fail").Inc()
			return
		}
		bindCounter.WithLabelValues("ok").Inc()
	}

	scrapeRes := "ok"
	for _, q := range queries {
		if err = scrapeQuery(conn, q); err != nil {
			s.log.Warn("query failed", "filter", q.searchFilter, "err", err)
			scrapeRes = "fail"
		}
	}
	scrapeCounter.WithLabelValues(scrapeRes).Inc()
}

type ReplicaStatus struct {
	Time   time.Time
	Server string
}

func (s *Scraper) scrapeReplication() []ReplicaStatus {

	var replicaStatus []ReplicaStatus
	for _, server := range s.ReplicatonServers {
		replica, err := ldap.DialURL(server)
		if err != nil {
			s.log.Error("dial failed")
			dialCounter.WithLabelValues("fail").Inc()
			continue
		}
		defer func(connection *ldap.Conn) {
			err = connection.Close()
			if err != nil {
				s.log.Error("close failed", "err", err)
			}
		}(replica)
		if s.User != "" && s.Pass != "" {
			err = replica.Bind(s.User, s.Pass)
			if err != nil {
				s.log.Error("bind failed", "err", err)
				bindCounter.WithLabelValues("fail").Inc()
				continue
			}
			bindCounter.WithLabelValues("ok").Inc()
		}

		req := ldap.NewSearchRequest(
			s.Sync, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 0, 0, false,
			"(contextCSN=*)", []string{monitorReplicationFilter}, nil,
		)
		sr, err := replica.Search(req)
		if err != nil {
			s.log.Error("query failed", "err", err)
			err = replica.Close()
			if err != nil {
				s.log.Error("close failed", "err", err)
			}
			continue
		}
		for _, entry := range sr.Entries {
			values := entry.GetAttributeValues(monitorReplicationFilter)
			for _, val := range values {
				if val == "" {
					// not every entry will have this attribute
					continue
				}
				ll := s.log.With(
					"filter", monitorReplicationFilter,
					"attr", monitorReplicationFilter,
					"value", val,
				)
				valueBuffer := strings.Split(val, "#")
				gt, err := time.Parse("20060102150405.999999Z", valueBuffer[0])
				if err != nil {
					ll.Warn("unexpected gt value", "err", err)
					continue
				}
				sid, err := strconv.Atoi(valueBuffer[2])
				if err != nil {
					ll.Warn("unexpected sid value", "err", err)
					continue
				}
				if sid == s.ServerId {
					replicaStatus = append(replicaStatus, ReplicaStatus{Time: gt, Server: server})
					break
				}
			}
		}
		err = replica.Close()
		if err != nil {
			s.log.Error("close failed", "err", err)
		}
	}
	return replicaStatus
}

func scrapeQuery(conn *ldap.Conn, q *query) error {
	req := ldap.NewSearchRequest(
		q.baseDN, q.scope, ldap.NeverDerefAliases, 0, 0, false,
		q.searchFilter, append([]string{q.searchAttr}, q.additionalAttr...), nil,
	)
	sr, err := conn.Search(req)
	if err != nil {
		return err
	}
	q.setData(sr.Entries, q)
	return nil
}

// ===============================================================
// Metrics server
// ===============================================================

var commit string
var tag string

func GetVersion() string {
	return fmt.Sprintf("%s (%s)", tag, commit)
}

type Server struct {
	server *http.Server
	logger *log.Logger
}

func NewMetricsServer(bindAddr, metricsPath string) *Server {
	mux := http.NewServeMux()
	mux.Handle(metricsPath, promhttp.Handler())
	mux.HandleFunc("/version", showVersion)
	return &Server{
		server: &http.Server{Addr: bindAddr, Handler: mux},
		logger: log.With("component", "server"),
	}
}

func showVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintln(w, GetVersion())
}

func (s *Server) Start() error {
	s.logger.Info("starting http listener", "addr", s.server.Addr)
	cfg := ""
	err := web.ListenAndServe(s.server, &web.FlagConfig{
		WebListenAddresses: &[]string{s.server.Addr},
		WebConfigFile:      &cfg,
	}, s.logger)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Stop() {
	quiet.CloseWithTimeout(s.server.Shutdown, 100*time.Millisecond)
}
