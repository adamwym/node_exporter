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
	"runtime"
	"strconv"

	"github.com/mindprince/gonvml"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

const (
	gpuCollectorSubsystem = "gpu"
	labels                = []string{"minor_number", "uuid", "name"}
)

type gpuCollector struct {
	gpuNumDevices  *prometheus.Desc
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

var defaultGpuCollector *gpuCollector = nil

func init() {
	registerCollector(gpuCollectorSubsystem, defaultDisabled, NewgpuCollector)
}

// NewgpuCollector returns a new Collector exposing kernel/system statistics.
func NewgpuCollector() (Collector, error) {
	if defaultGpuCollector != nil {
		return defaultGpuCollector, nil
	}

	if err := gonvml.Initialize(); err != nil {
		log.Fatalf("Couldn't initialize gonvml: %v. Make sure NVML is in the shared library search path.", err)
	}
	defaultGpuCollector = &gpuCollector{
		gpuNumDevices: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, gpuCollectorSubsystem, "num_devices"),
			"Number of GPU devices",
			[]string{},
			nil,
		),
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
	}

	// defer gonvml.Shutdown()
	runtime.SetFinalizer(defaultGpuCollector, func(obj *gpuCollector) { gonvml.Shutdown() })
}

// Update implements Collector and exposes gpu related metrics with nvml library
func (c *gpuCollector) Update(ch chan<- prometheus.Metric) error {
	numDevices, err := gonvml.DeviceCount()
	if err != nil {
		log.Printf("DeviceCount() error: %v", err)
		numDevices = 0
	}

	ch <- prometheus.MustNewConstMetric(c.gpuNumDevices, prometheus.CounterValue, float64(numDevices), nil)

	for i := 0; i < int(numDevices); i++ {
		dev, err := gonvml.DeviceHandleByIndex(uint(i))
		if err != nil {
			log.Printf("DeviceHandleByIndex(%d) error: %v", i, err)
			continue
		}

		minorNumber, err := dev.MinorNumber()
		if err != nil {
			log.Printf("MinorNumber() error: %v", err)
			continue
		}
		minor := strconv.Itoa(int(minorNumber))

		uuid, err := dev.UUID()
		if err != nil {
			log.Printf("UUID() error: %v", err)
			continue
		}

		name, err := dev.Name()
		if err != nil {
			log.Printf("Name() error: %v", err)
			continue
		}

		labelsValue := []string{minor, uuid, name}
		totalMemory, usedMemory, err := dev.MemoryInfo()
		if err != nil {
			log.Printf("MemoryInfo() error: %v", err)
		} else {
			ch <- prometheus.MustNewConstMetric(c.gpuUsedMemory, prometheus.CounterValue, float64(usedMemory), labelsValue...)
			ch <- prometheus.MustNewConstMetric(c.gpuTotalMemory, prometheus.CounterValue, float64(totalMemory), labelsValue...)
		}

		dutyCycle, _, err := dev.UtilizationRates()
		if err != nil {
			log.Printf("UtilizationRates() error: %v", err)
		} else {
			ch <- prometheus.MustNewConstMetric(c.gpuDutyCycle, prometheus.CounterValue, float64(dutyCycle), labelsValue...)
		}

		powerUsage, err := dev.PowerUsage()
		if err != nil {
			log.Printf("PowerUsage() error: %v", err)
		} else {
			ch <- prometheus.MustNewConstMetric(c.gpuPowerUsage, prometheus.CounterValue, float64(powerUsage), labelsValue...)
		}

		temperature, err := dev.Temperature()
		if err != nil {
			log.Printf("Temperature() error: %v", err)
		} else {
			ch <- prometheus.MustNewConstMetric(c.gpuTemperature, prometheus.CounterValue, float64(temperature), labelsValue...)
		}

		fanSpeed, err := dev.FanSpeed()
		if err != nil {
			log.Printf("FanSpeed() error: %v", err)
		} else {
			ch <- prometheus.MustNewConstMetric(c.gpuFanSpeed, prometheus.CounterValue, float64(fanSpeed), labelsValue...)
		}
	}
}
