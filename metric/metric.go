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

// Metric
package metric

import (
	"errors"
)

const (
	MetricString = iota
	MetricBool   = iota
	MetricNumber = iota
	MetricFloat  = iota
)

type Metric struct {
	Type       int         `json:"-"` // does not export to json
	TypeString string      `json:"type"`
	Dimension  string      `json:"dimension"`
	Name       string      `json:"name"`
	Unit       string      `json:"unit"`
	Value      interface{} `json:"value"`
}

func UnitToString(unit int) string {
	switch unit {
	case MetricString:
		return "string"
	case MetricBool:
		return "bool"
	case MetricNumber:
		return "int64"
	case MetricFloat:
		return "double"
	}
	return "undefined"
}

func NewMetric(name, metricDimension string, internalMetricType int, value interface{}, unit string) *Metric {
	if len(metricDimension) == 0 {
		metricDimension = "none"
	}
	metric := &Metric{
		Type:       internalMetricType,
		TypeString: UnitToString(internalMetricType),
		Dimension:  metricDimension,
		Name:       name,
		Value:      value,
		Unit:       unit,
	}
	return metric
}

func (m *Metric) ToUint64() (uint64, error) {
	if m.Type != MetricNumber {
		return 0, errors.New("Invalid coercion to Uint64")
	}
	value, ok := m.Value.(uint64)
	if !ok {
		return 0, errors.New("Invalid coercion to Uint64")
	}
	return value, nil
}

func (m *Metric) ToFloat64() (float64, error) {
	if m.Type != MetricFloat {
		return 0, errors.New("Invalid coercion to float64")
	}
	value, ok := m.Value.(float64)
	if !ok {
		return 0, errors.New("Invalid coercion to float64")
	}
	return value, nil
}
