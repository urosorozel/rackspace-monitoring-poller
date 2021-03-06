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

package poller

import (
	"context"
	"math/rand"
	"time"

	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	set "github.com/deckarep/golang-set"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/racker/rackspace-monitoring-poller/check"
)

const (
	checkPreparationBufferSize = 10
	checkLoggerDuration        = 5 * time.Minute
)

var (
	metricsSchedulerScheduled = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "poller",
			Subsystem: "scheduler",
			Name:      "scheduled_checks",
			Help:      "Conveys the number of checks currently scheduled per type",
		},
		[]string{
			metricLabelZone,
			metricLabelCheckType,
		},
	)
)

// EleScheduler implements Scheduler interface.
// See Scheduler for more information.
type EleScheduler struct {
	ctx    context.Context
	cancel context.CancelFunc

	zoneID string
	// checks maps checkId to the check definition
	checks       map[string]check.Check
	preparations chan ChecksPrepared
	resets       chan struct{}

	stream ConnectionStream

	scheduler CheckScheduler
	executor  CheckExecutor
}

func init() {
	metricsRegistry.MustRegister(metricsSchedulerScheduled)
}

// NewScheduler instantiates a new Scheduler with standard scheduling and executor behaviors.
// It sets up checks, context, and passed in zoneid
func NewScheduler(zoneID string, stream ConnectionStream) Scheduler {
	return NewCustomScheduler(zoneID, stream, nil, nil)
}

// NewCustomScheduler instantiates a new Scheduler using NewScheduler but allows for more customization.
// Nil can be passed to either checkScheduler and/or checkExecutor to enable the default behavior.
func NewCustomScheduler(zoneID string, stream ConnectionStream, checkScheduler CheckScheduler, checkExecutor CheckExecutor) Scheduler {
	s := &EleScheduler{
		checks:       make(map[string]check.Check),
		preparations: make(chan ChecksPrepared, checkPreparationBufferSize),
		resets:       make(chan struct{}, 1),
		stream:       stream,
		zoneID:       zoneID,
		scheduler:    checkScheduler,
		executor:     checkExecutor,
	}

	// by default we are our own scheduler/executor of checks
	if s.scheduler == nil {
		s.scheduler = s
	}
	if s.executor == nil {
		s.executor = s
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())

	go s.runReconciler()

	return s

}

// GetZoneID retrieves zone id
func (s *EleScheduler) GetZoneID() string {
	return s.zoneID
}

// GetContext retrieves cancelable context
func (s *EleScheduler) GetContext() (ctx context.Context, cancel context.CancelFunc) {
	return s.ctx, s.cancel
}

// Close cancels the context and closes the connection
func (s *EleScheduler) Close() {
	s.cancel()
}

func (s *EleScheduler) GetScheduledChecks() []check.Check {
	checks := make([]check.Check, 0, len(s.checks))

	for _, entry := range s.checks {
		checks = append(checks, entry)
	}

	return checks
}

func (s *EleScheduler) Reset() {
	s.resets <- struct{}{}
}

func (s *EleScheduler) reset() {
	log.WithFields(log.Fields{
		"prefix": "scheduler",
		"checks": s.GetScheduledChecks(),
	}).Info("Cancelling and de-scheduling checks due to reset")

	for checkId, check := range s.checks {
		delete(s.checks, checkId)
		s.scheduler.CancelCheck(check)
	}
}

func (s *EleScheduler) ReconcileChecks(cp ChecksPrepared) {
	s.preparations <- cp
}

func (s *EleScheduler) runReconciler() {

	logTicker := time.NewTicker(checkLoggerDuration)
	defer logTicker.Stop()

	for {
		select {
		case cp := <-s.preparations:
			s.reconcile(cp)
			s.logScheduledChecks()

		case <-s.resets:
			s.reset()

		case <-s.ctx.Done():
			return

		case <-logTicker.C:
			s.logScheduledChecks()
		}
	}
}

func (s *EleScheduler) logScheduledChecks() {
	if len(s.checks) > 0 {
		typeCounts := log.Fields{}
		for _, ch := range s.checks {
			count, _ := typeCounts[ch.GetCheckType()].(int)
			count++
			typeCounts[ch.GetCheckType()] = count
		}
		log.WithFields(typeCounts).Info("Checks scheduled to run")
	} else {
		log.Info("No checks are scheduled to run")
	}
}

func (s *EleScheduler) ValidateChecks(cp ChecksPreparing) error {
	actionableChecks := cp.GetActionableChecks()
	for _, ac := range actionableChecks {

		switch ac.Action {
		case ActionTypeRestart:
			_, exists := s.checks[ac.Id]
			if !exists {
				return errors.New(fmt.Sprintf("Reconciling was told to restart a check, but it does not exist: %v", ac.Id))
			}
		case ActionTypeContinue:
			_, exists := s.checks[ac.Id]
			if !exists {
				return errors.New(fmt.Sprintf("Reconciling was told to continue a check, but it does not exist: %v", ac.Id))
			}
		}
	}

	return nil
}

func (s *EleScheduler) reconcile(cp ChecksPrepared) {
	log.WithField("cp", cp).Debug("Reconciling prepared checks")

	// remainder will be used at the end to find ones were implicitly removed and need to be canceled out
	remainder := set.NewThreadUnsafeSet()
	for id, _ := range s.checks {
		remainder.Add(id)
	}

	actionableChecks := cp.GetActionableChecks()
	for _, ac := range actionableChecks {
		remainder.Remove(ac.Id)

		switch ac.Action {
		case ActionTypeStart:
			existingCheck, exists := s.checks[ac.Id]
			if exists {
				log.WithFields(log.Fields{
					"prefix":  "scheduler",
					"checkId": ac.Id,
				}).Warn("Reconciling was told to start a check, but it already existed.")
				s.scheduler.CancelCheck(existingCheck)
			} else {
				gauge, err := metricsSchedulerScheduled.GetMetricWithLabelValues(s.zoneID, ac.CheckType)
				if err == nil {
					gauge.Inc()
				} else {
					log.WithField("err", err).Warn("Failed to get gauge")
				}
			}
			err := s.initiateCheck(*ac)
			if err != nil {
				log.WithField("details", string(*ac.RawDetails)).Warn("Unable to initiate check")
			}

		case ActionTypeRestart:
			existingCheck, exists := s.checks[ac.Id]
			if exists {
				s.scheduler.CancelCheck(existingCheck)
			} else {
				log.WithField("checkId", ac.Id).Warn("Reconciling was told to restart a check, but it does not exist.")
			}
			err := s.initiateCheck(*ac)
			if err != nil {
				log.WithField("details", string(*ac.RawDetails)).Warn("Unable to initiate check")
			}

		case ActionTypeContinue:
			_, exists := s.checks[ac.Id]
			if !exists {
				log.WithField("checkId", ac.Id).Warn("Reconciling was told to continue a check, but it does not exist.")
			}
		}
	}

	for checkIdToRemove := range remainder.Iter() {
		log.WithField("checkId", checkIdToRemove).Info("Removing check implicitly due to absence in check preparation")

		checkIdToRemoveStr := checkIdToRemove.(string)
		checkToRemove := s.checks[checkIdToRemoveStr]
		delete(s.checks, checkIdToRemoveStr)
		s.scheduler.CancelCheck(checkToRemove)

		gauge, err := metricsSchedulerScheduled.GetMetricWithLabelValues(s.zoneID, checkToRemove.GetCheckType())
		if err == nil {
			gauge.Dec()
		} else {
			log.WithField("err", err).Warn("Failed to get gauge")
		}
	}

	if log.GetLevel() >= log.DebugLevel {
		log.WithFields(log.Fields{
			"prefix": "scheduler",
			"checks": s.checks,
		}).Debug("Reconciled and scheduled")
	}
}

func (s *EleScheduler) initiateCheck(ac ActionableCheck) error {
	newCheck, err := check.NewCheckParsed(s.ctx, ac.CheckIn)
	if err != nil {
		return err
	}
	s.checks[newCheck.GetID()] = newCheck
	s.scheduler.Schedule(newCheck)

	return nil
}

// Schedule is the default implementation of CheckScheduler that kicks off a go routine to run a check's timer.
func (s *EleScheduler) Schedule(ch check.Check) {
	if !ch.IsDisabled() {
		go s.runCheckTimerLoop(ch)
	}
}

// CancelCheck is a default implementation that calls check.Cancel
func (s *EleScheduler) CancelCheck(ch check.Check) {
	ch.Cancel()
}

func (s *EleScheduler) runCheckTimerLoop(ch check.Check) {
	// Spread the checks out over 30 seconds
	jitter := rand.Intn(CheckSpreadInMilliseconds) + 1

	log.WithFields(log.Fields{
		"id":         ch.GetID(),
		"type":       ch.GetCheckType(),
		"entity":     ch.GetEntityID(),
		"period":     ch.GetPeriod(),
		"jitterMs":   jitter,
		"waitPeriod": ch.GetWaitPeriod(),
	}).Info("Starting check")

	time.Sleep(time.Duration(jitter) * time.Millisecond)
	for {
		select {
		case <-time.After(ch.GetWaitPeriod()):
			s.executor.Execute(ch)

		case <-ch.Done(): // session cancellation is propagated since check context is child of session context
			log.WithField("check", ch.GetID()).Info("Check or session has been cancelled")
			return
		}
	}
}

// Execute perform the default CheckExecutor behavior by running the check and sending its results via SendMetrics.
func (s *EleScheduler) Execute(ch check.Check) {
	log.WithFields(log.Fields{
		"id":     ch.GetID(),
		"type":   ch.GetCheckType(),
		"period": ch.GetPeriod(),
	}).Debug("Running check")

	crs, err := ch.Run()
	if err != nil {
		log.Errorf("Error running check: %v", err)
	} else {
		s.SendMetrics(crs)
	}

}

// SendMetrics sends metrics passed in crs parameter via the stream
func (s *EleScheduler) SendMetrics(crs *check.ResultSet) {
	s.stream.SendMetrics(crs)
}
