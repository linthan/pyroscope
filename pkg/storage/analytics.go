package storage

import (
	"encoding/json"
	"time"

	"github.com/dgraph-io/badger/v2"
)

type Analytics struct {
	InstallID            string    `json:"install_id"`
	RunID                string    `json:"run_id"`
	Version              string    `json:"version"`
	Timestamp            time.Time `json:"timestamp"`
	UploadIndex          int       `json:"upload_index"`
	GOOS                 string    `json:"goos"`
	GOARCH               string    `json:"goarch"`
	GoVersion            string    `json:"go_version"`
	MemAlloc             int       `json:"mem_alloc"`
	MemTotalAlloc        int       `json:"mem_total_alloc"`
	MemSys               int       `json:"mem_sys"`
	MemNumGC             int       `json:"mem_num_gc"`
	BadgerMain           int       `json:"badger_main" type:"counter"`
	BadgerTrees          int       `json:"badger_trees" type:"counter"`
	BadgerDicts          int       `json:"badger_dicts" type:"counter"`
	BadgerDimensions     int       `json:"badger_dimensions" type:"counter"`
	BadgerSegments       int       `json:"badger_segments" type:"counter"`
	ControllerIndex      int       `json:"controller_index"`
	ControllerComparison int       `json:"controller_comparison"`
	ControllerDiff       int       `json:"controller_diff"`
	ControllerIngest     int       `json:"controller_ingest"`
	ControllerRender     int       `json:"controller_render"`
	SpyRbspy             int       `json:"spy_rbspy"`
	SpyPyspy             int       `json:"spy_pyspy"`
	SpyGospy             int       `json:"spy_gospy"`
	SpyEbpfspy           int       `json:"spy_ebpfspy"`
	SpyPhpspy            int       `json:"spy_phpspy"`
	SpyDotnetspy         int       `json:"spy_dotnetspy"`
	SpyJavaspy           int       `json:"spy_javaspy"`
	AppsCount            int       `json:"apps_count"`
}

func (s *Storage) SaveAnalytics(a *Analytics) {
	v, _ := json.Marshal(a)
	s.main.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry([]byte("analytics"), v)
		err := txn.SetEntry(e)
		return err
	})
}

func (s *Storage) ReadAnalytics() *Analytics {
	a := Analytics{}
	s.main.View(func(txn *badger.Txn) error {
		v, e := txn.Get([]byte("analytics"))
		v.Value(func(val []byte) error {
			e = json.Unmarshal(val, &a)
			return e
		})
		return e
	})
	return &a
}
