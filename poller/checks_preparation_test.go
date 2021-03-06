//
// Copyright 2017 Rackspace
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

package poller_test

import (
	"encoding/json"
	"github.com/racker/rackspace-monitoring-poller/poller"
	"github.com/racker/rackspace-monitoring-poller/protocol"
	"github.com/racker/rackspace-monitoring-poller/protocol/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"testing"
)

func TestNewCheckPreparation_VersionMatch(t *testing.T) {
	cp, err := poller.NewChecksPreparation("zn1", 1, []protocol.PollerPrepareManifest{})

	assert.NoError(t, err)
	assert.NotNil(t, cp)
	assert.True(t, cp.VersionApplies(1))
}

func TestNewCheckPreparation_VersionMismatch(t *testing.T) {
	cp, err := poller.NewChecksPreparation("zn1", 1, []protocol.PollerPrepareManifest{})

	assert.NoError(t, err)
	assert.NotNil(t, cp)
	assert.False(t, cp.VersionApplies(3))
}

func TestCheckPreparation_AddDefinitions_Normal(t *testing.T) {
	manifest := []protocol.PollerPrepareManifest{
		{
			Action:   protocol.PrepareActionStart,
			ZoneId:   "zn1",
			EntityId: "en1",
			Id:       "ch1",
		},
		{
			Action:   protocol.PrepareActionRestart,
			ZoneId:   "zn1",
			EntityId: "en2",
			Id:       "ch2",
		},
		{
			Action:   protocol.PrepareActionContinue,
			ZoneId:   "zn1",
			EntityId: "en2",
			Id:       "ch3",
		},
	}

	cp, err := poller.NewChecksPreparation("zn1", 1, manifest)
	require.NoError(t, err)

	block1 := loadTestDataChecks(t,
		checkLoadInfo{name: "tcp_check", id: "ch2", entityId: "en2", zonedId: "zn1"},
	)

	block2 := loadTestDataChecks(t,
		checkLoadInfo{name: "tcp_check", id: "ch1", entityId: "en1", zonedId: "zn1"},
	)

	cp.AddDefinitions(block1)
	cp.AddDefinitions(block2)

	err = cp.Validate(1)
	assert.NoError(t, err)
}

func TestCheckPreparation_AddDefinitions_Fails(t *testing.T) {

	tests := []struct {
		name     string
		manifest []protocol.PollerPrepareManifest
		block    []*check.CheckIn
		validate int
	}{
		{
			name: "missingOne",
			manifest: []protocol.PollerPrepareManifest{
				{
					Action:   protocol.PrepareActionStart,
					ZoneId:   "zn1",
					EntityId: "en1",
					Id:       "ch1",
				},
				{
					Action:   protocol.PrepareActionRestart,
					ZoneId:   "zn1",
					EntityId: "en2",
					Id:       "ch2",
				},
				{
					Action:   protocol.PrepareActionContinue,
					ZoneId:   "zn1",
					EntityId: "en2",
					Id:       "ch3",
				},
			},
			block: loadTestDataChecks(t,
				checkLoadInfo{name: "tcp_check", id: "ch2", entityId: "en2", zonedId: "zn1"},
			),
			validate: 1,
		},
		{
			name: "notDeclared",
			manifest: []protocol.PollerPrepareManifest{
				{
					Action:   protocol.PrepareActionContinue,
					ZoneId:   "zn1",
					EntityId: "en2",
					Id:       "ch3",
				},
			},
			block: loadTestDataChecks(t,
				checkLoadInfo{name: "tcp_check", id: "ch2", entityId: "en2", zonedId: "zn1"},
			),
			validate: 1,
		},
		{
			name: "wrongVersion",
			manifest: []protocol.PollerPrepareManifest{
				{
					Action:   protocol.PrepareActionRestart,
					ZoneId:   "zn1",
					EntityId: "en2",
					Id:       "ch2",
				},
			},
			block: loadTestDataChecks(t,
				checkLoadInfo{name: "tcp_check", id: "ch2", entityId: "en2", zonedId: "zn1"},
			),
			validate: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp, _ := poller.NewChecksPreparation("zn1", 1, tt.manifest)

			cp.AddDefinitions(tt.block)

			err := cp.Validate(tt.validate)
			assert.Error(t, err)

		})
	}

}

func TestNewCheckPreparation_UnknownActionStr(t *testing.T) {
	manifest := []protocol.PollerPrepareManifest{
		{
			Action:   "BOGUS ACTION",
			ZoneId:   "zn1",
			EntityId: "en2",
			Id:       "ch2",
		},
	}

	_, err := poller.NewChecksPreparation("zn1", 1, manifest)

	assert.Error(t, err)
}

func TestChecksPreparation_IsNewer_ThanNil(t *testing.T) {
	var cp *poller.ChecksPreparation

	assert.True(t, cp.IsNewer(1))
}

func TestChecksPreparation_IsNewer_Value(t *testing.T) {

	tests := []struct {
		name      string
		preparing int
		checking  int
		expect    bool
	}{
		{
			name:      "good",
			preparing: 1,
			checking:  2,
			expect:    true,
		},
		{
			name:      "same",
			preparing: 1,
			checking:  1,
			expect:    false,
		},
		{
			name:      "older",
			preparing: 50,
			checking:  1,
			expect:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp, err := poller.NewChecksPreparation("zn1", tt.preparing, []protocol.PollerPrepareManifest{})
			assert.NoError(t, err)

			assert.Equal(t, tt.expect, cp.IsNewer(tt.checking))
		})
	}

}

type checkLoadInfo struct {
	name     string
	id       string
	entityId string
	zonedId  string

	checkType string
	action    string
}

func loadTestDataChecks(t *testing.T, info ...checkLoadInfo) (checks []*check.CheckIn) {
	checks = make([]*check.CheckIn, 0, len(info))

	for _, entry := range info {
		bytes, err := ioutil.ReadFile("testdata/" + entry.name + ".json")
		require.NoError(t, err)

		var ch check.CheckIn
		err = json.Unmarshal(bytes, &ch)
		require.NoError(t, err)
		ch.Id = entry.id
		ch.EntityId = entry.entityId
		ch.ZoneId = entry.zonedId

		assert.NotEmpty(t, ch.CheckType)
		checks = append(checks, &ch)
	}

	return
}

func loadChecksPreparation(t *testing.T, info ...checkLoadInfo) *poller.ChecksPreparation {
	manifest := make([]protocol.PollerPrepareManifest, 0, len(info))
	for _, entry := range info {
		manifest = append(manifest, protocol.PollerPrepareManifest{
			ZoneId:    entry.zonedId,
			Action:    entry.action,
			Id:        entry.id,
			EntityId:  entry.entityId,
			CheckType: entry.checkType,
		})
	}

	cp, err := poller.NewChecksPreparation("zn1", 1, manifest)
	require.NoError(t, err)

	block := loadTestDataChecks(t, info...)
	cp.AddDefinitions(block)

	return cp
}
