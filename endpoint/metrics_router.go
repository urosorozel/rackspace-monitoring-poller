//
// Copyright 2016 Rackspace
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS-IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package endpoint

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/racker/rackspace-monitoring-poller/config"
)

const (
	IncomingChanSize = 100
)

type Metric struct {
	// Name is <entity_id>.<agent_id>.<check_type>.<check_id>.<field>
	Name       string
	Value      string
	MetricType string
}

type MetricsRouter struct {
	metrics chan *Metric

	cfg config.EndpointConfig
}

func NewMetricsRouter(cfg *config.EndpointConfig) *MetricsRouter {
	mr := &MetricsRouter{
		metrics: make(chan *Metric, IncomingChanSize),
		cfg:     *cfg,
	}

	go mr.start()

	return mr
}

func BuildMetricName(entityId, agentId, checkType, checkId, field string) string {
	return fmt.Sprintf("%s.%s.%s.%s.%s", entityId, agentId, checkType, checkId, field)
}

func (mr *MetricsRouter) Route(metric Metric) {
	mr.metrics <- &metric
}

func (mr *MetricsRouter) start() {
	for {
		metric := <-mr.metrics
		metricBlob := fmt.Sprintf("%v:%v|%v", metric.Name, metric.Value, metric.MetricType)
		log.WithField("metric", metricBlob).Debug("Routing to bit bucket...for now")
		//TODO
	}
}
