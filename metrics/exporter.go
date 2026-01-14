package metrics

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cormoran/natureremo"
	"github.com/eivy/control-remo-from-pi/mqtt"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "remo"
)

// Metrics descriptions
var (
	temperature = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "temperature"),
		"The temperature of the remo device",
		[]string{"name", "id"}, nil,
	)

	humidity = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "humidity"),
		"The humidity of the remo device",
		[]string{"name", "id"}, nil,
	)

	illumination = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "illumination"),
		"The illumination of the remo device",
		[]string{"name", "id"}, nil,
	)

	motion = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "motion"),
		"The motion of the remo device",
		[]string{"name", "id"}, nil,
	)

	normalElectricEnergy = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "normal_direction_cumulative_electric_energy"),
		"The raw value for cumulative electric energy in normal direction",
		[]string{"name", "id"}, nil,
	)

	reverseElectricEnergy = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "reverse_direction_cumulative_electric_energy"),
		"The raw value for cumulative electric energy in reverse direction",
		[]string{"name", "id"}, nil,
	)

	coefficient = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "coefficient"),
		"The coefficient for cumulative electric energy",
		[]string{"name", "id"}, nil,
	)

	electricEnergyUnit = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "cumulative_electric_energy_unit_kilowatt_hour"),
		"The unit in kWh for cumulative electric energy",
		[]string{"name", "id"}, nil,
	)

	electricEnergyDigits = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "cumulative_electric_energy_effective_digits"),
		"The number of effective digits for cumulative electric energy",
		[]string{"name", "id"}, nil,
	)

	measuredInstantaneousEnergy = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "measured_instantaneous_energy_watt"),
		"The measured instantaneous energy in W",
		[]string{"name", "id"}, nil,
	)

	rateLimitLimit = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "x_rate_limit_limit"),
		"The rate limit for the remo API",
		nil, nil,
	)

	rateLimitReset = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "x_rate_limit_reset"),
		"The time in which the rate limit for the remo API will be reset",
		nil, nil,
	)

	rateLimitRemaining = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "x_rate_limit_remaining"),
		"The remaining number of request for the remo API",
		nil, nil,
	)

	httpRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "http_requests_total",
		Help:      "The total number of requests labeled by response code",
	},
		[]string{"code", "api"},
	)
)

// Exporter collects ECS clusters metrics
type Exporter struct {
	client     *natureremo.Client // Custom ECS client to get information from the clusters
	mqttClient *mqtt.Client
}

// NewExporter returns an initialized exporter
func NewExporter(config *Config, client *natureremo.Client, mqttClient *mqtt.Client) (*Exporter, error) {
	return &Exporter{
		client:     client,
		mqttClient: mqttClient,
	}, nil
}

// Describe is to describe the metrics for Prometheus
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- temperature
	ch <- humidity
	ch <- illumination
	ch <- motion
	ch <- normalElectricEnergy
	ch <- reverseElectricEnergy
	ch <- coefficient
	ch <- electricEnergyUnit
	ch <- electricEnergyDigits
	ch <- measuredInstantaneousEnergy
	ch <- rateLimitLimit
	ch <- rateLimitReset
	ch <- rateLimitRemaining
	httpRequestsTotal.Describe(ch)
}

// Collect collects data to be consumed by prometheus
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()
	devices, err := e.client.DeviceService.GetAll(ctx)
	if err != nil {
		fmt.Printf("Fetching device stats failed: %v", err)
		return
	}

	appliances, err := e.client.ApplianceService.GetAll(ctx)
	if err != nil {
		fmt.Printf("Fetching appliances stats failed: %v", err)
		return
	}

	err = e.processMetrics(devices, appliances, ch)
	if err != nil {
		fmt.Printf("Processing the metrics failed: %v", err)
		return
	}

	for _, a := range appliances {
		if a.Type == natureremo.ApplianceTypeLight {
			e.mqttClient.PublishStatus(mqtt.Status{
				ApplianceID:   a.ID,
				ApplianceName: a.Nickname,
				Type:          string(a.Type),
				PowerState:    a.Light.State.Power == "on",
				Timestamp:     time.Now(),
			})
		}
	}

}

func (e *Exporter) processMetrics(devices []*natureremo.Device, appliances []*natureremo.Appliance, ch chan<- prometheus.Metric) error {
	for _, d := range devices {
		if d.NewestEvents == nil {
			continue
		}
		if v, ok := d.NewestEvents[natureremo.SensorTypeTemperature]; ok {
			ch <- prometheus.MustNewConstMetric(temperature, prometheus.GaugeValue, v.Value, d.Name, d.ID)
		}
		if v, ok := d.NewestEvents[natureremo.SensorTypeHumidity]; ok {
			ch <- prometheus.MustNewConstMetric(humidity, prometheus.GaugeValue, v.Value, d.Name, d.ID)
		}
		if v, ok := d.NewestEvents[natureremo.SensorTypeIllumination]; ok {
			ch <- prometheus.MustNewConstMetric(illumination, prometheus.GaugeValue, v.Value, d.Name, d.ID)
		}
		if v, ok := d.NewestEvents[natureremo.SensorTypeMovement]; ok {
			ch <- prometheus.MustNewConstMetric(motion, prometheus.GaugeValue, float64(v.CreatedAt.Unix()), d.Name, d.ID)
		}
	}

	sms := getSmartMeters(appliances)
	for _, sm := range sms {
		info, err := energyInfo(sm)
		if err != nil {
			fmt.Printf("failed to get EnergyInfo: %v", err)
			continue
		}
		ch <- prometheus.MustNewConstMetric(normalElectricEnergy, prometheus.CounterValue, float64(info.NormalEnergy), sm.Device.Name, sm.Device.ID)
		ch <- prometheus.MustNewConstMetric(reverseElectricEnergy, prometheus.CounterValue, float64(info.ReverseEnergy), sm.Device.Name, sm.Device.ID)
		ch <- prometheus.MustNewConstMetric(coefficient, prometheus.GaugeValue, float64(info.Coefficient), sm.Device.Name, sm.Device.ID)
		ch <- prometheus.MustNewConstMetric(electricEnergyUnit, prometheus.GaugeValue, info.EnergyUnit, sm.Device.Name, sm.Device.ID)
		ch <- prometheus.MustNewConstMetric(electricEnergyDigits, prometheus.GaugeValue, float64(info.EffectiveDigits), sm.Device.Name, sm.Device.ID)
		ch <- prometheus.MustNewConstMetric(measuredInstantaneousEnergy, prometheus.GaugeValue, float64(info.MeasuredInstantaneous), sm.Device.Name, sm.Device.ID)
	}

	ch <- prometheus.MustNewConstMetric(rateLimitLimit, prometheus.GaugeValue, float64(e.client.LastRateLimit.Limit))
	ch <- prometheus.MustNewConstMetric(rateLimitRemaining, prometheus.GaugeValue, float64(e.client.LastRateLimit.Remaining))
	ch <- prometheus.MustNewConstMetric(rateLimitReset, prometheus.GaugeValue, float64(e.client.LastRateLimit.Reset.Unix()))

	httpRequestsTotal.Collect(ch)

	return nil
}

func getSmartMeters(apps []*natureremo.Appliance) []*natureremo.Appliance {
	smartMeters := make([]*natureremo.Appliance, 0)
	for _, app := range apps {
		if app.Type == "EL_SMART_METER" {
			smartMeters = append(smartMeters, app)
		}
	}
	return smartMeters
}

type EnergyInfo struct {
	NormalEnergy          int
	ReverseEnergy         int
	Coefficient           int
	EnergyUnit            float64
	EffectiveDigits       int
	MeasuredInstantaneous int
}

func energyInfo(sm *natureremo.Appliance) (*EnergyInfo, error) {
	if sm.SmartMeter == nil {
		return nil, fmt.Errorf("'%s' does not have smart_meter field", sm.Device.Name)
	}
	var info EnergyInfo
	var err error
	for _, p := range sm.SmartMeter.Properties {
		switch p.Epc {
		case natureremo.EPCNormalDirectionCumulativeElectricEnergy:
			info.NormalEnergy, err = strconv.Atoi(p.Value)
			if err != nil {
				return nil, err
			}
		case natureremo.EPCReverseDirectionCumulativeElectricEnergy:
			info.ReverseEnergy, err = strconv.Atoi(p.Value)
			if err != nil {
				return nil, err
			}
		case natureremo.EPCCoefficient:
			info.Coefficient, err = strconv.Atoi(p.Value)
			if err != nil {
				return nil, err
			}
		case natureremo.EPCCumulativeElectricEnergyUnit:
			unit, err := strconv.Atoi(p.Value)
			if err != nil {
				return nil, err
			}
			switch unit {
			case 0:
				info.EnergyUnit = 1
			case 1:
				info.EnergyUnit = 0.1
			case 2:
				info.EnergyUnit = 0.01
			case 3:
				info.EnergyUnit = 0.001
			case 4:
				info.EnergyUnit = 0.0001
			case 10:
				info.EnergyUnit = 10
			case 11:
				info.EnergyUnit = 100
			case 12:
				info.EnergyUnit = 1000
			case 13:
				info.EnergyUnit = 10000
			default:
				return nil, fmt.Errorf("invalid CumulativeElectricEnergyUnit value: %d", unit)
			}
		case natureremo.EPCCumulativeElectricEnergyEffectiveDigits:
			info.EffectiveDigits, err = strconv.Atoi(p.Value)
			if err != nil {
				return nil, err
			}
		case natureremo.EPCMeasuredInstantaneous:
			info.MeasuredInstantaneous, err = strconv.Atoi(p.Value)
			if err != nil {
				return nil, err
			}
		}
	}
	return &info, nil
}
