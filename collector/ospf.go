package collector

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ospfSubsystem = "ospf"
)

func init() {
	registerCollector(ospfSubsystem, enabledByDefault, NewOSPFCollector)
}

type ospfCollector struct {
	logger       log.Logger
	descriptions map[string]*prometheus.Desc
}

// NewOSPFCollector  collects OSPF metrics, implemented as per the Collector interface.
func NewOSPFCollector(logger log.Logger) (Collector, error) {
	return &ospfCollector{logger: logger, descriptions: getOSPFDesc()}, nil
}

func getOSPFDesc() map[string]*prometheus.Desc {
	labels := []string{"vrf", "iface", "area"}
	return map[string]*prometheus.Desc{
		"ospfIfaceNeigh":    colPromDesc(ospfSubsystem, "neighbors", "Number of neighbors detected.", labels),
		"ospfIfaceNeighAdj": colPromDesc(ospfSubsystem, "neighbor_adjacencies", "Number of neighbor adjacencies formed.", labels),
	}
}

// Update implemented as per the Collector interface.
func (c *ospfCollector) Update(ch chan<- prometheus.Metric) error {
	cmd := "show ip ospf vrf all interface json"
	jsonOSPFInterface, err := executeOSPFCommand(cmd)
	if err != nil {
		return err
	} else {
		if err = processOSPFInterface(ch, jsonOSPFInterface, c.descriptions); err != nil {
			return cmdOutputProcessError(cmd, string(jsonOSPFInterface), err)
		}
	}
	return nil
}

func processOSPFInterface(ch chan<- prometheus.Metric, jsonOSPFInterface []byte, ospfDesc map[string]*prometheus.Desc) error {
	// Unfortunately, the 'show ip ospf vrf all interface json' JSON  output is poorly structured. Instead
	// of all interfaces being in a list, each interface is added as a key on the same level of vrfName and
	// vrfId. As such, we have to loop through each key and apply logic to determine whether the key is an
	// interface.
	var jsonMap map[string]json.RawMessage
	if err := json.Unmarshal(jsonOSPFInterface, &jsonMap); err != nil {
		return fmt.Errorf("cannot unmarshal ospf interface json: %s", err)
	}

	for vrfName, vrfData := range jsonMap {
		var _tempvrfInstance map[string]json.RawMessage
		if err := json.Unmarshal(vrfData, &_tempvrfInstance); err != nil {
			return fmt.Errorf("cannot unmarshal VRF instance json: %s", err)
		}

		for ospfInstanceKey, ospfInstanceVal := range _tempvrfInstance {
			switch ospfInstanceKey {
			case "vrfName", "vrfId":
				// Do nothing as we do not need the value of these keys.
			case "interfaces":
				var _tempInterfaceInstance map[string]json.RawMessage
				if err := json.Unmarshal(ospfInstanceVal, &_tempInterfaceInstance); err != nil {
					return fmt.Errorf("cannot unmarshal VRF instance json: %s", err)
				}
				for interfaceKey, interfaceValue := range _tempInterfaceInstance {
					var newIface ospfIface
					if err := json.Unmarshal(interfaceValue, &newIface); err != nil {
						return fmt.Errorf("cannot unmarshal interface json: %s", err)
					}
					if !newIface.TimerPassiveIface {
						// The labels are "vrf", "newIface", "area"
						labels := []string{strings.ToLower(vrfName), interfaceKey, newIface.Area}
						ospfMetrics(ch, newIface, labels, ospfDesc)
					}
				}
			default:
				// All other keys are interfaces.
				var iface ospfIface
				if err := json.Unmarshal(ospfInstanceVal, &iface); err != nil {
					return fmt.Errorf("cannot unmarshal interface json: %s", err)
				}
				if !iface.TimerPassiveIface {
					// The labels are "vrf", "iface", "area"
					labels := []string{strings.ToLower(vrfName), ospfInstanceKey, iface.Area}
					ospfMetrics(ch, iface, labels, ospfDesc)
				}
			}
		}
	}
	return nil
}

func ospfMetrics(ch chan<- prometheus.Metric, iface ospfIface, labels []string, ospfDesc map[string]*prometheus.Desc) {
	newGauge(ch, ospfDesc["ospfIfaceNeigh"], iface.NbrCount, labels...)
	newGauge(ch, ospfDesc["ospfIfaceNeighAdj"], iface.NbrAdjacentCount, labels...)
}

type ospfIface struct {
	NbrCount          float64
	NbrAdjacentCount  float64
	Area              string
	TimerPassiveIface bool
}
