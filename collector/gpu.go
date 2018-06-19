// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build !nogpu

package collector

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

type gpuCollector struct {
	gpuUsedMemory  *prometheus.Desc
	gpuTotalMemory *prometheus.Desc
	gpuDutyCycle   *prometheus.Desc
	gpuPowerUsage  *prometheus.Desc
	gpuTemperature *prometheus.Desc
	gpuFanSpeed    *prometheus.Desc

	gpuTimePercent *prometheus.Desc // percentage of time during kernels are executing on the GPU.
	gpuClockHz     *prometheus.Desc // GPU graphics clock in Hz
	gpuMemClockHz  *prometheus.Desc // GPU memory clock in Hz
	// GPU clock throttle reason.
	// The descriptions of the values can be seen in NvmlClocksThrottleReasons section in NVML API Reference.
	gpuThrottleReason *prometheus.Desc
	// GPU performance state (C.uint). 0 to 15. 0 for max performance, 15 for min performance. 32 for unknown.
	// The descriptions of the values can be seen in nvmlPstates_t in Device Enums section in NVML API Reference.
	gpuPerfState *prometheus.Desc
}

const (
	nvidiaSmiCmd          = "nvidia-smi"
	gpuCollectorSubsystem = "gpu"
)

var (
	labels = []string{"minor_number", "uuid", "name"}
)

func init() {
	registerCollector(gpuCollectorSubsystem, defaultDisabled, NewgpuCollector)
}

// NewgpuCollector returns a new Collector exposing kernel/system statistics.
func NewgpuCollector() (Collector, error) {
	return &gpuCollector{
		gpuUsedMemory: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, gpuCollectorSubsystem, "memory_used_bytes"),
			"Memory used by the GPU device in bytes",
			labels,
			nil,
		),
		gpuTotalMemory: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, gpuCollectorSubsystem, "memory_total_bytes"),
			"Total memory of the GPU device in bytes",
			labels,
			nil,
		),
		gpuDutyCycle: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, gpuCollectorSubsystem, "duty_cycle"),
			"Percent of time over the past sample period during which one or more kernels were executing on the GPU device",
			labels,
			nil,
		),
		gpuPowerUsage: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, gpuCollectorSubsystem, "power_usage_milliwatts"),
			"Power usage of the GPU device in milliwatts",
			labels,
			nil,
		),
		gpuTemperature: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, gpuCollectorSubsystem, "temperature_celsius"),
			"Temperature of the GPU device in celsius",
			labels,
			nil,
		),
		gpuFanSpeed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, gpuCollectorSubsystem, "fanspeed_percent"),
			"Fanspeed of the GPU device as a percent of its maximum",
			labels,
			nil,
		),
	}, nil
}

func convert2map(data []string) map[string]int {
	retv := map[string]int{}
	for index, value := range data {
		retv[value] = index
	}
	return retv
}
func mustFind(valueMap map[string]int, key string) int {
	value, ok := valueMap[key]
	if !ok {
		panic(fmt.Sprintf("key:%s not found", key))
	}
	return value
}

var valueList = []string{
	"name", "index", "uuid", "fan.speed", "temperature.gpu", "clocks.gr", "clocks.sm", "clocks.mem", "power.draw", "utilization.gpu", "utilization.memory", "memory.total", "memory.free", "memory.used",
}

var valueMap = convert2map(valueList)

// Update implements Collector and exposes gpu related metrics with nvml library
func (c *gpuCollector) Update(ch chan<- prometheus.Metric) error {
	out, err := exec.Command(
		nvidiaSmiCmd,
		"--query-gpu="+strings.Join(valueList, ","),
		"--format=csv,noheader,nounits").Output()

	if err != nil {
		return err
	}

	csvReader := csv.NewReader(bytes.NewReader(out))
	csvReader.TrimLeadingSpace = true
	records, err := csvReader.ReadAll()

	if err != nil {
		return err
	}

	for _, row := range records {
		name := row[mustFind(valueMap, "name")]
		minor := row[mustFind(valueMap, "index")]
		uuid := row[mustFind(valueMap, "uuid")]
		labelsValue := []string{minor, uuid, name}

		usedMemory, err := strconv.ParseFloat(row[mustFind(valueMap, "memory.used")], 64)
		if err != nil {
			log.Debugf("PowerUsage() error: %v", err)
		} else {
			ch <- prometheus.MustNewConstMetric(c.gpuUsedMemory, prometheus.CounterValue, float64(usedMemory*1024*1024), labelsValue...)
		}
		totalMemory, err := strconv.ParseFloat(row[mustFind(valueMap, "memory.total")], 64)
		if err != nil {
			log.Debugf("PowerUsage() error: %v", err)
		} else {
			ch <- prometheus.MustNewConstMetric(c.gpuTotalMemory, prometheus.CounterValue, float64(totalMemory*1024*1024), labelsValue...)
		}

		dutyCycle, err := strconv.ParseFloat(row[mustFind(valueMap, "utilization.gpu")], 64)
		if err != nil {
			log.Debugf("UtilizationRates() error: %v", err)
		} else {
			ch <- prometheus.MustNewConstMetric(c.gpuDutyCycle, prometheus.CounterValue, float64(dutyCycle), labelsValue...)
		}

		powerUsage, err := strconv.ParseFloat(row[mustFind(valueMap, "power.draw")], 64)
		if err != nil {
			log.Debugf("PowerUsage() error: %v", err)
		} else {
			ch <- prometheus.MustNewConstMetric(c.gpuPowerUsage, prometheus.CounterValue, float64(powerUsage), labelsValue...)
		}

		temperature, err := strconv.ParseFloat(row[mustFind(valueMap, "temperature.gpu")], 64)
		if err != nil {
			log.Debugf("Temperature() error: %v", err)
		} else {
			ch <- prometheus.MustNewConstMetric(c.gpuTemperature, prometheus.CounterValue, float64(temperature), labelsValue...)
		}

		fanSpeed, err := strconv.ParseFloat(row[mustFind(valueMap, "fan.speed")], 64)
		if err != nil {
			log.Debugf("FanSpeed() error: %v", err)
		} else {
			ch <- prometheus.MustNewConstMetric(c.gpuFanSpeed, prometheus.CounterValue, float64(fanSpeed), labelsValue...)
		}
	}
	return nil
}
