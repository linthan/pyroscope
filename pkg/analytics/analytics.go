/*
Package analytics deals with collecting pyroscope server usage data.

By default pyroscope server sends anonymized usage data to Pyroscope team.
This helps us understand how people use Pyroscope and prioritize features accordingly.
We take privacy of our users very seriously and only collect high-level stats
such as number of apps added, types of spies used, etc.

You can disable this with a flag or an environment variable

	pyroscope server -analytics-opt-out
	...
	PYROSCOPE_ANALYTICS_OPT_OUT=true pyroscope server

*/
package analytics

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

var (
	url               = "https://analytics.pyroscope.io/api/events"
	gracePeriod       = 1 * time.Second
	snapshotFrequency = 5 * time.Second
	uploadFrequency   = 10 * time.Second
)

type StatsProvider interface {
	Stats() map[string]int
	AppsCount() int
}

func NewService(cfg *config.Server, s *storage.Storage, p StatsProvider) *Service {
	return &Service{
		cfg: cfg,
		s:   s,
		p:   p,
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxConnsPerHost: 1,
			},
			Timeout: 60 * time.Second,
		},
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

type Service struct {
	cfg        *config.Server
	s          *storage.Storage
	p          StatsProvider
	httpClient *http.Client
	uploads    int

	stop     chan struct{}
	done     chan struct{}
	baseline *storage.Analytics
}

func (s *Service) Start() {
	defer close(s.done)
	s.baseline = s.s.ReadAnalytics()
	timer := time.NewTimer(gracePeriod)
	select {
	case <-s.stop:
		return
	case <-timer.C:
	}
	s.sendReport()
	ticker := time.NewTicker(uploadFrequency)
	snapshot := time.NewTicker(snapshotFrequency)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.sendReport()
		case <-snapshot.C:
			m := s.getAnalytics()
			m = s.mergeAnalytics(s.baseline, m)
			s.s.SaveAnalytics(m)
		case <-s.stop:
			return
		}
	}
}

func (s *Service) mergeAnalytics(baseline *storage.Analytics, current *storage.Analytics) *storage.Analytics {
	retv := storage.Analytics{}
	cu := reflect.ValueOf(*current)
	bs := reflect.ValueOf(*baseline)
	ret := reflect.ValueOf(retv)
	for i := 0; i < bs.NumField(); i++ {
		field := bs.Field(i)
		fieldtype := bs.Type()
		fieldCurrent := cu.FieldByName(fieldtype.Name())
		fieldRet := ret.FieldByName(fieldtype.Name())
		t, ok := fieldtype.Field(i).Tag.Lookup("type")
		if ok && t == "counter" {
			switch fieldtype.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				fieldRet.SetInt(field.Int() + fieldCurrent.Int())
			}
		}

	}
	return &retv

}
func (s *Service) Stop() {
	s.s.SaveAnalytics(s.getAnalytics())
	close(s.stop)
	<-s.done
}

func (s *Service) getAnalytics() *storage.Analytics {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	du := s.s.DiskUsage()

	controllerStats := s.p.Stats()

	m := storage.Analytics{
		InstallID:            s.s.InstallID(),
		RunID:                uuid.New().String(),
		Version:              build.Version,
		Timestamp:            time.Now(),
		UploadIndex:          s.uploads,
		GOOS:                 runtime.GOOS,
		GOARCH:               runtime.GOARCH,
		GoVersion:            runtime.Version(),
		MemAlloc:             int(ms.Alloc),
		MemTotalAlloc:        int(ms.TotalAlloc),
		MemSys:               int(ms.Sys),
		MemNumGC:             int(ms.NumGC),
		BadgerMain:           int(du["main"]),
		BadgerTrees:          int(du["trees"]),
		BadgerDicts:          int(du["dicts"]),
		BadgerDimensions:     int(du["dimensions"]),
		BadgerSegments:       int(du["segments"]),
		ControllerIndex:      controllerStats["index"],
		ControllerComparison: controllerStats["comparison"],
		ControllerDiff:       controllerStats["diff"],
		ControllerIngest:     controllerStats["ingest"],
		ControllerRender:     controllerStats["render"],
		SpyRbspy:             controllerStats["ingest:rbspy"],
		SpyPyspy:             controllerStats["ingest:pyspy"],
		SpyGospy:             controllerStats["ingest:gospy"],
		SpyEbpfspy:           controllerStats["ingest:ebpfspy"],
		SpyPhpspy:            controllerStats["ingest:phpspy"],
		SpyDotnetspy:         controllerStats["ingest:dotnetspy"],
		SpyJavaspy:           controllerStats["ingest:javaspy"],
		AppsCount:            s.p.AppsCount(),
	}

	return &m
}

func (s *Service) sendReport() {
	logrus.Debug("sending analytics report")

	m := s.getAnalytics()
	m = s.mergeAnalytics(s.baseline, m)

	buf, err := json.Marshal(m)
	if err != nil {
		logrus.WithField("err", err).Error("Error happened when preparing JSON")
		return
	}
	resp, err := s.httpClient.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		logrus.WithField("err", err).Error("Error happened when uploading anonymized usage data")
	}
	if resp != nil {
		_, err := io.ReadAll(resp.Body)
		if err != nil {
			logrus.WithField("err", err).Error("Error happened when uploading reading server response")
			return
		}
	}

	s.uploads++
}
