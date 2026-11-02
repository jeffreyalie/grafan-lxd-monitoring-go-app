package main

import (
	"log"
	"sync"
	"time"

	"github.com/canonical/lxd/shared/api"
	"github.com/prometheus/client_golang/prometheus"
)

type lxdCollector struct {
	mu        sync.RWMutex
	instances []api.InstanceFull
	lastOK    time.Time

	up, cpu, mem, procs, lastPoll, pollErrs *prometheus.Desc
	errCount                                float64
}

func newLXDCollector(interval time.Duration) *lxdCollector {
	l := []string{"name", "type", "project", "status"}
	c := &lxdCollector{
		up:       prometheus.NewDesc("lxd_instance_up", "1 if the instance is Running", l, nil),
		cpu:      prometheus.NewDesc("lxd_instance_cpu_usage_seconds_total", "CPU time used", l, nil),
		mem:      prometheus.NewDesc("lxd_instance_memory_usage_bytes", "Memory in use", l, nil),
		procs:    prometheus.NewDesc("lxd_instance_processes", "Process count", l, nil),
		lastPoll: prometheus.NewDesc("lxd_last_successful_poll_timestamp_seconds", "Last successful LXD poll", nil, nil),
		pollErrs: prometheus.NewDesc("lxd_poll_errors_total", "Failed LXD polls", nil, nil),
	}
	go func() {
		c.poll()
		for range time.Tick(interval) {
			c.poll()
		}
	}()
	return c
}

func (c *lxdCollector) poll() {
	instances, err := fetchInstances()
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.errCount++
		log.Printf("metrics poll failed: %v", err)
		return
	}
	c.instances, c.lastOK = instances, time.Now()
}

func (c *lxdCollector) Describe(ch chan<- *prometheus.Desc) { prometheus.DescribeByCollect(c, ch) }

func (c *lxdCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, i := range c.instances {
		lv := []string{i.Name, i.Type, i.Project, i.Status}
		up := 0.0
		if i.Status == "Running" {
			up = 1
		}
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, up, lv...)
		if i.State != nil {
			ch <- prometheus.MustNewConstMetric(c.cpu, prometheus.CounterValue, float64(i.State.CPU.Usage)/1e9, lv...)
			ch <- prometheus.MustNewConstMetric(c.mem, prometheus.GaugeValue, float64(i.State.Memory.Usage), lv...)
			ch <- prometheus.MustNewConstMetric(c.procs, prometheus.GaugeValue, float64(i.State.Processes), lv...)
		}
	}
	ch <- prometheus.MustNewConstMetric(c.lastPoll, prometheus.GaugeValue, float64(c.lastOK.Unix()))
	ch <- prometheus.MustNewConstMetric(c.pollErrs, prometheus.CounterValue, c.errCount)
}
