package collector

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	vrrpStatusInitialize = "Initialize"
	vrrpStatusBackup     = "Backup"
	vrrpStatusMaster     = "Master"
)

var (
	vrrpSubsystem = "vrrp"
	vrrpStates    = []string{vrrpStatusInitialize, vrrpStatusMaster, vrrpStatusBackup}
)

func init() {
	registerCollector(vrrpSubsystem, disabledByDefault, NewVRRPCollector)
}

type VrrpVrInfo struct {
	Vrid      uint32
	Interface string
	V6Info    VrrpInstanceInfo `json:"v6"`
	V4Info    VrrpInstanceInfo `json:"v4"`
}

type VrrpInstanceInfo struct {
	Subinterface string `json:"interface"`
	Status       string
	Statistics   VrrpInstanceStats `json:"stats"`
}

type VrrpInstanceStats struct {
	AdverTx         *uint32
	AdverRx         *uint32
	GarpTx          *uint32
	NeighborAdverTx *uint32
	Transitions     *uint32
}

type vrrpCollector struct {
	logger       *slog.Logger
	descriptions map[string]*prometheus.Desc
}

// NewVRRPCollector collects VRRP metrics, implemented as per the Collector interface.
func NewVRRPCollector(logger *slog.Logger) (Collector, error) {
	return &vrrpCollector{logger: logger, descriptions: getVRRPDesc()}, nil
}

func getVRRPDesc() map[string]*prometheus.Desc {
	labels := []string{"proto", "vrid", "interface", "subinterface"}
	stateLabels := append(labels, "state")

	return map[string]*prometheus.Desc{
		"vrrpState":       colPromDesc(vrrpSubsystem, "state", "Status of the VRRP state machine.", stateLabels),
		"adverTx":         colPromDesc(vrrpSubsystem, "advertisements_sent_total", "Advertisements sent total.", labels),
		"adverRx":         colPromDesc(vrrpSubsystem, "advertisements_received_total", "Advertisements received total.", labels),
		"garpTx":          colPromDesc(vrrpSubsystem, "gratuitous_arp_sent_total", "Gratuitous ARP sent total.", labels),
		"neighborAdverTx": colPromDesc(vrrpSubsystem, "neighbor_advertisements_sent_total", "Neighbor Advertisements sent total.", labels),
		"transitions":     colPromDesc(vrrpSubsystem, "state_transitions_total", "Number of transitions of the VRRP state machine in total.", labels),
	}
}

// Update implemented as per the Collector interface.
func (c *vrrpCollector) Update(ch chan<- prometheus.Metric) error {
	cmd := "show vrrp json"
	jsonVRRPInfo, err := executeVRRPCommand(cmd)
	if err != nil {
		return err
	}
	if err := processVRRPInfo(ch, jsonVRRPInfo, c.descriptions); err != nil {
		return cmdOutputProcessError(cmd, string(jsonVRRPInfo), err)
	}
	return nil
}

func processVRRPInfo(ch chan<- prometheus.Metric, jsonVRRPInfo []byte, desc map[string]*prometheus.Desc) error {
	var jsonList []VrrpVrInfo
	if err := json.Unmarshal(jsonVRRPInfo, &jsonList); err != nil {
		return err
	}

	for _, vrInfo := range jsonList {
		processInstance(ch, "v4", vrInfo.Vrid, vrInfo.Interface, vrInfo.V4Info, desc)
		processInstance(ch, "v6", vrInfo.Vrid, vrInfo.Interface, vrInfo.V6Info, desc)
	}

	return nil
}

func processInstance(ch chan<- prometheus.Metric, proto string, vrid uint32, iface string, instance VrrpInstanceInfo, vrrpDesc map[string]*prometheus.Desc) {
	vrrpLabels := []string{proto, strconv.FormatUint(uint64(vrid), 10), iface, instance.Subinterface}

	for _, state := range vrrpStates {
		stateLabels := append(vrrpLabels, state)

		var value float64

		if strings.EqualFold(instance.Status, state) {
			value = 1
		}

		newGauge(ch, vrrpDesc["vrrpState"], value, stateLabels...)
	}

	if instance.Statistics.AdverTx != nil {
		newCounter(ch, vrrpDesc["adverTx"], float64(*instance.Statistics.AdverTx), vrrpLabels...)
	}

	if instance.Statistics.AdverRx != nil {
		newCounter(ch, vrrpDesc["adverRx"], float64(*instance.Statistics.AdverRx), vrrpLabels...)
	}

	if instance.Statistics.GarpTx != nil {
		newCounter(ch, vrrpDesc["garpTx"], float64(*instance.Statistics.GarpTx), vrrpLabels...)
	}

	if instance.Statistics.NeighborAdverTx != nil {
		newCounter(ch, vrrpDesc["neighborAdverTx"], float64(*instance.Statistics.NeighborAdverTx), vrrpLabels...)
	}

	if instance.Statistics.Transitions != nil {
		newCounter(ch, vrrpDesc["transitions"], float64(*instance.Statistics.Transitions), vrrpLabels...)
	}
}
