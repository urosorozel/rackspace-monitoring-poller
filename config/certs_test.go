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

package config_test

import (
	"github.com/racker/rackspace-monitoring-poller/config"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestLoadProductionCAs(t *testing.T) {
	assert := assert.New(t)

	pool := config.LoadProductionCAs()
	assert.NotNil(pool)

	subjects := pool.Subjects()
	assert.Len(subjects, 1)
}

func TestLoadStagingCAs(t *testing.T) {
	assert := assert.New(t)

	pool := config.LoadStagingCAs()
	assert.NotNil(pool)

	subjects := pool.Subjects()
	assert.Len(subjects, 1)
}

func TestLoadDevelopmentCAs_normal(t *testing.T) {
	assert := assert.New(t)

	pool := config.LoadDevelopmentCAs("testdata/ca.pem")
	assert.NotNil(pool)

	subjects := pool.Subjects()
	assert.Len(subjects, 1)
}

func TestLoadDevelopmentCAs_empty(t *testing.T) {
	assert := assert.New(t)

	pool := config.LoadDevelopmentCAs("testdata/empty.pem")
	assert.Nil(pool)
}

func TestLoadDevelopmentCAs_bogus(t *testing.T) {
	assert := assert.New(t)

	pool := config.LoadDevelopmentCAs("testdata/bogus.pem")
	assert.Nil(pool)
}

func TestLoadDevelopmentCAs_missing(t *testing.T) {
	assert := assert.New(t)

	pool := config.LoadDevelopmentCAs("testdata/does_not_exist.pem")
	assert.Nil(pool)
}

func TestLoadRootCAs_tls_defaults(t *testing.T) {
	assert := assert.New(t)

	certPool := config.LoadRootCAs(false, false)
	if assert.NotNil(certPool) {
		assert.Len(certPool.Subjects(), 1)
	}
}

func TestLoadRootCAs_tls_insecure(t *testing.T) {
	assert := assert.New(t)

	certPool := config.LoadRootCAs(true, false)
	assert.Nil(certPool)
}

func TestLoadRootCAs_tls_staging(t *testing.T) {
	assert := assert.New(t)

	certPool := config.LoadRootCAs(false, true)
	if assert.NotNil(certPool) {
		assert.Len(certPool.Subjects(), 1)
	}
}

func TestLoadRootCAs_tls_development(t *testing.T) {
	assert := assert.New(t)

	os.Setenv(config.EnvDevCA, "testdata/ca.pem")
	certPool := config.LoadRootCAs(false, false)
	if assert.NotNil(certPool) {
		assert.Len(certPool.Subjects(), 1)
	}
}
