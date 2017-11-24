package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	prom_version "github.com/prometheus/common/version"

	"github.com/dariubs/percent"
	"github.com/namsral/flag"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

const (
	namespace = "transfodev"
)

var (
	uptime = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "uptime"),
		"Up time percentage",
		[]string{"instance"}, nil,
	)
)

// Exporter Sets up all the runtime and metrics.
type Exporter struct {
	instances             []string
	urlPrometheusAPI      string
	userPrometheusAPI     string
	passwordPrometheusAPI string
	metrics               map[string]float64
}

type PrometheusResponse struct {
	Data struct {
		Result []struct {
			Metric struct {
				Name     string `json:"__name__"`
				Instance string `json:"instance"`
				Job      string `json:"job"`
			} `json:"metric"`
			Values [][]interface{} `json:"values"`
		} `json:"result"`
		ResultType string `json:"resultType"`
	} `json:"data"`
	Status string `json:"status"`
}

// Exporter Sets up all the runtime and metrics
type UpTime struct {
	TimeSt int32
	Value  float64
}

// NewExporter creates the metrics we wish to monitor
func NewExporter(instances []string, urlPrometheusAPI string, userPrometheusAPI string, passwordPrometheusAPI string) *Exporter {

	return &Exporter{
		instances:             instances,
		urlPrometheusAPI:      urlPrometheusAPI,
		userPrometheusAPI:     userPrometheusAPI,
		passwordPrometheusAPI: passwordPrometheusAPI,
		metrics:               map[string]float64{},
	}
}

// Describe describes all the metrics ever exported by the transfodev exporter.
// It implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- uptime
}

// Collect fetches the stats from configured tranfodev instances and delivers them
// as Prometheus metrics.
// It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	log.Infof("transfodev exporter starting")

	for _, instance := range e.instances {
		var upTime = UpTime{}
		computeDailyUpTimePct(&upTime, instance, e.urlPrometheusAPI, e.userPrometheusAPI, e.passwordPrometheusAPI)

		if e.metrics == nil {
			log.Errorf("Unable to get prometheus HTTP metrics.")
			return
		}

		e.metrics["uptime"] = upTime.Value

		ch <- prometheus.MustNewConstMetric(uptime, prometheus.GaugeValue, e.metrics["uptime"], instance)
	}

	log.Infof("transfodev exporter finished")
}

/*
*  Compute upTime percentage
*
*   TODO: calculer les dates de début et fin
*         gérer les erreurs
*
 */
func computeDailyUpTimePct(uptime *UpTime, instance string, urlPrometheusAPI string, user string, password string) {

	tStart := time.Now().AddDate(0, 0, -1)
	tStart = time.Date(tStart.Year(), tStart.Month(), tStart.Day(), 0, 0, 0, 0, tStart.Location())
	var tEnd = time.Date(tStart.Year(), tStart.Month(), tStart.Day(), 23, 59, 59, 999, tStart.Location())

	req, err := http.NewRequest("GET",
		urlPrometheusAPI+"/query_range?query=probe_http_status_code&start="+
			tStart.Format(time.RFC3339)+"&end="+tEnd.Format(time.RFC3339)+"&step=1m", nil)

	if err != nil {
		// handle err
	}
	req.SetBasicAuth(user, password)
	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		// handle err
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {

		bodyBytes, err2 := ioutil.ReadAll(resp.Body)

		var response PrometheusResponse
		err3 := json.Unmarshal(bodyBytes, &response)

		if err2 != nil {
			// handle err
		}
		if err3 != nil {
			// handle err
		}

		for i := 0; i < len(response.Data.Result); i++ {
			if instance == response.Data.Result[i].Metric.Instance {
				var sumUp = 0

				valuesMap := response.Data.Result[i].Values
				var numline = 0
				for _, v := range valuesMap {
					val, err := strconv.Atoi(v[1].(string))
					numline++
					if err != nil {
						// handle err
					}

					if (val >= 200) && (val <= 308) {
						sumUp++

					}
				}
				uptime.Value = percent.PercentOf(sumUp, numline)

			}
		}
	}

}

func init() {

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	prometheus.MustRegister(prom_version.NewCollector("tranfodev_exporter"))
}

func main() {
	var (
		exporterPort          = flag.String("exporter-port", "9405", "Address to listen on for web interface and telemetry.")
		exporterLocation      = flag.String("exporter-location", "/metrics", "Path under which to expose metrics.")
		instances             = flag.String("exporter-instances", "", "URL instances of metrics to aggregate (comma separated).")
		urlPrometheusAPI      = flag.String("prometheus-api-url", "", "URL of Prometheus HTTP API")
		userPrometheusAPI     = flag.String("prometheus-api-user", "", "User to access the Prometheus HTTP API")
		passwordPrometheusAPI = flag.String("prometheus-api-password", "", "Password to access the Prometheus HTTP API")
	)
	flag.Parse()

	log.Infoln("Starting speedtest exporter", prom_version.Info())
	log.Infoln("Build context", prom_version.BuildContext())

	exporter := NewExporter(strings.Split(*instances, ","), *urlPrometheusAPI, *userPrometheusAPI, *passwordPrometheusAPI)

	log.Infoln("Register exporter")
	prometheus.MustRegister(exporter)

	http.Handle(*exporterLocation, prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>TranfoDev Exporter</title></head>
             <body>
             <h1>TranfoDev Exporter</h1>
             <p><a href='` + *exporterLocation + `'>Metrics</a></p>
             </body>
             </html>`))
	})

	log.Infoln("Listening on", *exporterPort)
	log.Fatal(http.ListenAndServe(":" + *exporterPort, nil))
}
