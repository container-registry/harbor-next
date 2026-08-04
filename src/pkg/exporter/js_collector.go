// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package exporter

import (
	"github.com/gomodule/redigo/redis"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/orm"
)

// JobServiceCollectorName ...
const JobServiceCollectorName = "JobServiceCollector"

var (
	jobServiceTaskQueueSize = typedDesc{
		desc:      newDescWithLabels("", "task_queue_size", "Total number of tasks", "type"),
		valueType: prometheus.GaugeValue,
	}
	jobServiceTaskQueueLatency = typedDesc{
		desc:      newDescWithLabels("", "task_queue_latency", "how long ago the next job to be processed was enqueued", "type"),
		valueType: prometheus.GaugeValue,
	}
	jobServiceConcurrency = typedDesc{
		desc:      newDescWithLabels("", "task_concurrency", "Total number of concurrency on a pool", "type", "pool"),
		valueType: prometheus.GaugeValue,
	}
	jobServiceScheduledJobTotal = typedDesc{
		desc:      newDesc("", "task_scheduled_total", "total number of scheduled job"),
		valueType: prometheus.GaugeValue,
	}
)

// NewJobServiceCollector ...
func NewJobServiceCollector(backend Backend) *JobServiceCollector {
	return &JobServiceCollector{Namespace: namespace, backend: backend}
}

// JobServiceCollector ...
type JobServiceCollector struct {
	Namespace string
	backend   Backend
}

// Describe implements prometheus.Collector
func (hc *JobServiceCollector) Describe(c chan<- *prometheus.Desc) {
	for _, jd := range hc.getDescribeInfo() {
		c <- jd
	}
}

// Collect implements prometheus.Collector
func (hc *JobServiceCollector) Collect(c chan<- prometheus.Metric) {
	for _, m := range hc.getJobserviceInfo() {
		c <- m
	}
}

// GetName returns the name of the job service collector
func (hc *JobServiceCollector) GetName() string {
	return JobServiceCollectorName
}

func (hc *JobServiceCollector) getDescribeInfo() []*prometheus.Desc {
	return []*prometheus.Desc{
		jobServiceTaskQueueSize.Desc(),
		jobServiceTaskQueueLatency.Desc(),
		jobServiceConcurrency.Desc(),
		jobServiceScheduledJobTotal.Desc(),
	}
}

func (hc *JobServiceCollector) getJobserviceInfo() []prometheus.Metric {
	if CacheEnabled() {
		value, ok := CacheGet(JobServiceCollectorName)
		if ok {
			return value.([]prometheus.Metric)
		}
	}

	js, err := hc.backend.JobService(orm.Context())
	if err != nil {
		// In core the job service redis config is fetched from job service over
		// HTTP, which may not be up yet. Skip this scrape rather than fail it.
		log.Errorf("error when resolving job service backend: %v", err)
		return []prometheus.Metric{}
	}

	// Get concurrency info via raw redis client
	result := getConccurrentInfo(js)

	// get queue info
	qs, err := js.Client.Queues()
	checkErr(err, "error when get work task queues info")
	for _, q := range qs {
		result = append(result, jobServiceTaskQueueSize.MustNewConstMetric(float64(q.Count), q.JobName))
		result = append(result, jobServiceTaskQueueLatency.MustNewConstMetric(float64(q.Latency), q.JobName))
	}

	// get scheduled job info
	_, total, err := js.Client.ScheduledJobs(0)
	checkErr(err, "error when get scheduled job number")
	result = append(result, jobServiceScheduledJobTotal.MustNewConstMetric(float64(total)))

	if CacheEnabled() {
		CachePut(JobServiceCollectorName, result)
	}
	return result
}

func getConccurrentInfo(js *JobServiceBackend) []prometheus.Metric {
	rdsConn := js.Pool.Get()
	defer rdsConn.Close()
	result := []prometheus.Metric{}
	knownJobvalues, err := redis.Values(rdsConn.Do("SMEMBERS", redisKeyKnownJobs(js.Namespace)))
	checkErr(err, "err when get known jobs")
	for _, v := range knownJobvalues {
		job := string(v.([]byte))
		lockInfovalues, err := redis.Values(rdsConn.Do("HGETALL", redisKeyJobsLockInfo(js.Namespace, job)))
		checkErr(err, "err when get job lock info")
		for i := 0; i < len(lockInfovalues); i += 2 {
			key, _ := redis.String(lockInfovalues[i], nil)
			value, _ := redis.Float64(lockInfovalues[i+1], nil)
			result = append(result, jobServiceConcurrency.MustNewConstMetric(value, job, key))
		}
	}
	return result
}
