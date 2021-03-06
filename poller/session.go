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
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/racker/rackspace-monitoring-poller/check"
	"github.com/racker/rackspace-monitoring-poller/config"
	"github.com/racker/rackspace-monitoring-poller/hostinfo"
	"github.com/racker/rackspace-monitoring-poller/protocol"
	"github.com/racker/rackspace-monitoring-poller/utils"
	"math/rand"
)

// CompletionFrame is a pointer to a request with a specified
// method used for the request
type CompletionFrame struct {
	ID     uint64
	Method string
}

const (
	sendChannelSize      = 128
	readChannelSize      = 256
	EventTypeSendError   = "SessionSendError"
	EventTypeReadError   = "SessionReadError"
	EventTypeAuthTimeout = "SessionAuthTimeout"
)

type prepDetails struct {
	reconciler             ChecksReconciler
	activePrep             *ChecksPreparation
	newestCommittedVersion int
	srcPrepMsg             *protocol.FrameMsg
	prepared               bool
	prepareToEndTimer      *time.Timer
}

// EleSession implements Session interface
// See Session for more information
type EleSession struct {
	utils.EventConsumerRegistry

	// reference to the connection
	connection Connection

	prepDetails

	cfg *config.Config

	// Used to cancel all go routines
	ctx    context.Context
	cancel context.CancelFunc

	// JSON encoders and decoder streams
	enc *json.Encoder
	dec *json.Decoder

	// sequence message ID
	seq uint64

	completionsMu sync.Mutex
	completions   map[uint64]*CompletionFrame

	logPrefix string

	sendCh    chan protocol.Frame
	readCh    chan *protocol.FrameMsg
	authTimer *time.Timer

	heartbeatsStarter    sync.Once
	heartbeatInterval    time.Duration
	heartbeatResponses   chan *protocol.HeartbeatResponse
	heartbeatMeasurement struct {
		tracking      utils.TimeLatencyTracking
		expectedSeqID uint64
		offset        int64
		latency       int64
		observations  uint64
	}
}

func NewSession(ctx context.Context, connection Connection, checksReconciler ChecksReconciler, cfg *config.Config) Session {
	session := &EleSession{
		connection: connection,
		prepDetails: prepDetails{
			reconciler: checksReconciler,
		},
		cfg:                cfg,
		dec:                json.NewDecoder(connection.GetFarendReader()),
		seq:                0, // so that handshake req gets ID 1 after incrementing
		sendCh:             make(chan protocol.Frame, sendChannelSize),
		readCh:             make(chan *protocol.FrameMsg, readChannelSize),
		heartbeatResponses: make(chan *protocol.HeartbeatResponse, 1),
		completions:        make(map[uint64]*CompletionFrame),
	}
	session.ctx, session.cancel = context.WithCancel(ctx)
	session.logPrefix = fmt.Sprintf("session: %v", connection.GetLogPrefix())
	go session.runFrameReading()
	go session.runFrameHandlingAndTimeout()
	go session.runFrameSending()
	session.Auth()
	return session
}

// Auth sends a handshake request with token, agent id, name,
// and process version
func (s *EleSession) Auth() {
	request := protocol.NewHandshakeRequest(s.cfg)
	s.Send(request)
	if s.cfg.TimeoutAuth > 0 {
		s.authTimer = time.AfterFunc(s.cfg.TimeoutAuth, func() {
			log.WithField("prefix", s.logPrefix).Warn("Closing connection due to expired auth")
			s.connection.Close()
			s.EmitEvent(utils.NewEvent(EventTypeAuthTimeout, nil))
		})
	}
}

// Send stages a frame for sending after setting the target and source.
// NOTE: If the message's ID is not initialized an ID will be allocated.
func (s *EleSession) Send(msg protocol.Frame) {
	if msg.GetId() == 0 {
		msg.SetId(&s.seq)
	}
	msg.SetTarget("endpoint")
	msg.SetSource(s.connection.GetGUID())
	s.sendCh <- msg
}

// Respond is equivalent to Send but improves readability by emphasizing this is the poller responding to a
// request from the server.
func (s *EleSession) Respond(msg protocol.Frame) {
	msg.SetTarget("endpoint")
	msg.SetSource(s.connection.GetGUID())
	s.sendCh <- msg
}

func (s *EleSession) getCompletionRequest(resp protocol.Frame) *CompletionFrame {
	s.completionsMu.Lock()
	req, ok := s.completions[resp.GetId()]
	if !ok {
		s.completionsMu.Unlock()
		return nil
	}
	delete(s.completions, resp.GetId())
	s.completionsMu.Unlock()
	return req
}

func (s *EleSession) handleResponse(resp *protocol.FrameMsg) error {
	if req := s.getCompletionRequest(resp); req != nil {
		switch req.Method {
		case protocol.MethodHandshakeHello:
			if s.authTimer != nil {
				s.authTimer.Stop()
				s.authTimer = nil
			}
			resp := protocol.DecodeHandshakeResponse(resp)
			if resp.Error != nil {
				log.WithFields(log.Fields{
					"prefix": s.logPrefix,
					"error":  resp.Error.Message,
				}).Error("handshake error")
				return errors.New(resp.Error.Message)
			}
			// just to be sure guard against multiple handshake starting multiple heartbeat routines
			s.heartbeatInterval = time.Duration(resp.Result.HeartbeatInterval) * time.Millisecond
			s.heartbeatsStarter.Do(s.goRunHeartbeats)
			s.connection.SetAuthenticated()
		case protocol.MethodHeartbeatPost:
			resp := protocol.DecodeHeartbeatResponse(resp)
			s.heartbeatResponses <- resp
		case protocol.MethodCheckMetricsPostMulti:
		default:
			log.WithFields(log.Fields{
				"prefix": s.logPrefix,
				"method": req.Method,
			}).Error("Unexpected method")
		}
	}
	return nil
}

// GetReadDeadline adds sessions's heartbeat interval to configured read deadline
func (s *EleSession) computeReadDeadline() time.Time {
	return s.cfg.ComputeReadDeadline(s.heartbeatInterval)
}

// GetWriteDeadline adds sessions's heartbeat interval to configured write deadline
func (s *EleSession) computeWriteDeadline() time.Time {
	return s.cfg.ComputeWriteDeadline(s.heartbeatInterval)
}

func (s *EleSession) runFrameReading() {
	log.WithField("prefix", s.logPrefix).Debug("read starting")
	defer log.WithField("prefix", s.logPrefix).Debug("read exiting")

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			f := new(protocol.FrameMsg)
			s.connection.SetReadDeadline(s.computeReadDeadline())
			if err := s.dec.Decode(f); err == io.EOF {
				log.WithFields(log.Fields{
					"prefix":     s.logPrefix,
					"connection": s.connection,
				}).Debug("Far end closed connection")
				s.cancel()
				return
			} else if err != nil {
				s.exitError(err)
				s.cancel()
				return
			}
			s.readCh <- f
		}
	}
}

func (s *EleSession) runFrameHandlingAndTimeout() {
	log.WithField("prefix", s.logPrefix).Debug("frame handling starting")
	defer log.WithField("prefix", s.logPrefix).Debug("frame handling exiting")

	for {
		select {
		case <-s.ctx.Done():
			return

		case f := <-s.readCh:
			if err := s.handleFrame(f); err != nil {
				s.EmitEvent(utils.NewEvent(EventTypeReadError, &FrameMsgError{Frame: f, Error: err}))
				log.WithFields(log.Fields{
					"prefix": s.logPrefix,
					"error":  err,
				}).Debug("Error during handleFrame")
				// but loop back around since frame-level errors are not catastrophic
			}

		case <-utils.ChannelOfTimer(s.prepDetails.prepareToEndTimer):
			s.prepDetails.prepareToEndTimer = nil
			s.respondFailureToPollerPrepare(s.prepDetails.srcPrepMsg, s.activePrep,
				protocol.PrepareResultStatusFailed, "prepare-to-end timer expired")
			s.prepDetails.clear()
		}
	}
}

func (s *EleSession) handleFrame(f *protocol.FrameMsg) error {
	if log.GetLevel() >= log.DebugLevel {
		js, _ := f.Encode()
		log.WithFields(log.Fields{
			"prefix":  s.logPrefix,
			"payload": string(js),
		}).Debug("socket receive")
	}
	var err error
	switch f.GetMethod() {
	case protocol.MethodEmpty: // Responses do not have a method name
		err = s.handleResponse(f)
	case protocol.MethodHostInfoGet:
		go s.handleHostInfo(f)
	case protocol.MethodPollerPrepare:
		s.handlePollerPrepare(f)
	case protocol.MethodPollerPrepareBlock:
		s.handlePollerPrepareBlock(f)
	case protocol.MethodPollerPrepareEnd:
		s.handlePollerPrepareEnd(f)
	case protocol.MethodPollerCommit:
		s.handlePollerCommit(f)
	case protocol.MethodCheckTest:
		go s.handleCheckTest(f)
	default:
		log.WithFields(log.Fields{
			"prefix": s.logPrefix,
			"method": f.GetMethod(),
		}).Error("Need to handle method")
	}
	return err
}

func (s *EleSession) handleHostInfo(f *protocol.FrameMsg) {
	if hinfo := hostinfo.NewHostInfo(f.GetRawParams()); hinfo != nil {
		result, err := hinfo.Run()
		if err != nil {
			log.WithFields(log.Fields{
				"prefix": s.logPrefix,
				"error":  err,
			}).Error("Hostinfo returned error")
			return
		}
		s.Respond(hostinfo.NewHostInfoResponse(result, f))
	}
}

func (s *EleSession) handlePollerPrepare(f *protocol.FrameMsg) {
	req := protocol.DecodePollerPrepareStartRequest(f)
	reqVer := req.Params.Version

	if reqVer <= s.prepDetails.newestCommittedVersion {
		s.respondFailureToPollerPrepare(f, req, protocol.PrepareResultStatusIgnored,
			"Request contains version older than newest committed version")
		return
	}

	if s.prepDetails.activePrep.IsOlder(reqVer) {
		s.respondFailureToPollerPrepare(f, req, protocol.PrepareResultStatusIgnored,
			"Request contains version older than active preparation")
		return
	}

	if s.activePrep != nil && s.activePrep.IsNewer(reqVer) {
		s.respondFailureToPollerPrepare(s.prepDetails.srcPrepMsg, s.prepDetails.activePrep, protocol.PrepareResultStatusIgnored,
			"Request supercedes a previous preparation")

		if s.prepDetails.prepareToEndTimer != nil {
			s.prepDetails.prepareToEndTimer.Stop()
		}

		// fall through
	}

	cp, err := NewChecksPreparation(req.Params.ZoneId, reqVer, req.Params.Manifest)
	if err != nil {
		s.respondFailureToPollerPrepare(f, req, protocol.PrepareResultStatusFailed, err.Error())
		return
	}

	err = s.prepDetails.reconciler.ValidateChecks(cp)
	if err != nil {
		s.respondFailureToPollerPrepare(f, req, protocol.PrepareResultStatusFailed, err.Error())
		return
	}

	s.prepDetails.prepareToEndTimer = time.NewTimer(s.cfg.TimeoutPrepareEnd)

	// It's all good, so note it and proceed
	s.prepDetails.srcPrepMsg = f
	s.prepDetails.activePrep = cp
}

func (s *EleSession) handlePollerPrepareBlock(f *protocol.FrameMsg) {
	req := protocol.DecodePollerPrepareBlockRequest(f)
	if !s.prepDetails.activePrep.VersionApplies(req.Params.Version) {
		log.WithFields(log.Fields{
			"prefix":  s.logPrefix,
			"req":     req.String(),
			"details": s.prepDetails.String(),
		}).Warn("Ignoring prepare block with wrong version")
		return
	}

	s.prepDetails.activePrep.AddDefinitions(req.Params.Block)
	t := s.prepDetails.prepareToEndTimer
	if !t.Stop() {
		<-t.C
	}
	t.Reset(s.cfg.TimeoutPrepareEnd)
}

func (s *EleSession) handlePollerPrepareEnd(f *protocol.FrameMsg) {
	req := protocol.DecodePollerPrepareEndRequest(f)

	if req.Params.Directive == protocol.PrepareDirectiveAbort {
		s.respondFailureToPollerPrepare(s.prepDetails.srcPrepMsg, req, protocol.PrepareResultStatusAborted,
			"Aborting poller prepare per request of the server")

		s.prepDetails.clear()

		return
	}

	if req.Params.Directive != protocol.PrepareDirectivePrepare {
		s.respondFailureToPollerPrepare(s.prepDetails.srcPrepMsg, req, protocol.PrepareResultStatusFailed,
			fmt.Sprintf("Unexpected directive during poller prepare end: %v", req.Params.Directive))
		return
	}

	if s.activePrep == nil {
		s.respondFailureToPollerPrepare(f, req, protocol.PrepareResultStatusFailed,
			"No active checks preparation")
		return
	}

	if err := s.activePrep.Validate(req.Params.Version); err != nil {
		s.respondFailureToPollerPrepare(s.prepDetails.srcPrepMsg, req, protocol.PrepareResultStatusFailed,
			err.Error())
		return
	}

	s.prepDetails.prepared = true
	s.prepDetails.prepareToEndTimer.Stop()
	s.prepDetails.prepareToEndTimer = nil

	result := protocol.PollerPrepareResult{
		ZoneId:  s.prepDetails.activePrep.ZoneId,
		Version: req.Params.Version,
		Status:  protocol.PrepareResultStatusPrepared,
	}
	resp := protocol.NewPollerPrepareResponse(s.prepDetails.srcPrepMsg, result)
	if log.GetLevel() >= log.DebugLevel {
		log.WithFields(log.Fields{
			"prefix":  s.logPrefix,
			"req":     req.String(),
			"details": s.prepDetails.String(),
		}).Debug("Responding to end of poller prepare")
	}
	s.Respond(resp)
}

func (s *EleSession) handlePollerCommit(f *protocol.FrameMsg) {
	req := protocol.DecodePollerCommitRequest(f)
	if !s.prepDetails.activePrep.VersionApplies(req.Params.Version) {
		details := "Poller commit request specified non-applicable version"
		log.WithFields(log.Fields{"prefix": s.logPrefix, "req": req}).Warn(details)
		s.respondCommitResult(f, req, protocol.PrepareResultStatusIgnored, details)
		return
	}
	s.respondCommitResult(f, req, protocol.PrepareResultStatusCommitted, "")
	s.prepDetails.commit()
}

// handleCheckTest runs within a go routine
func (s *EleSession) handleCheckTest(f *protocol.FrameMsg) {
	req, err := protocol.DecodePollerCheckTestRequest(f)
	if err != nil {
		log.WithFields(log.Fields{
			"prefix": s.logPrefix,
			"err":    err,
			"frame":  f,
		}).Warn("Unable to interpret the given check for testing")
		resp := protocol.NewErrorResponse(f, 1, "Unable to interpret the given check for testing")
		s.Respond(resp)
		return
	}

	newCheck, err := check.NewCheckParsed(s.ctx, *req.Params.CheckParams)
	if err != nil {
		log.WithFields(log.Fields{
			"prefix": s.logPrefix,
			"err":    err,
			"frame":  f,
		}).Warn("Unable to interpret the given check for testing")
		resp := protocol.NewErrorResponse(f, 1, "Unable to interpret the given check for testing")
		s.Respond(resp)
		return
	}

	// Ensure the check has an ID or create a temp one if it's a "test new check"
	if newCheck.GetID() == "" {
		newCheck.SetID(fmt.Sprintf("tch%06d", rand.Intn(999999)))
	}
	crs, err := newCheck.Run()

	if err != nil {
		log.WithFields(log.Fields{
			"prefix": s.logPrefix,
			"err":    err,
			"check":  newCheck,
		}).Warn("Running check")
		resp := protocol.NewErrorResponse(f, 2, err.Error())
		s.Respond(resp)
		return
	}

	var content protocol.MetricsPostContent
	crs.PopulateMetricsPostContent(0, &content)

	resp := protocol.NewPollerCheckTestResponse(f, &content)
	s.Respond(resp)

}

func (s *EleSession) respondCommitResult(f *protocol.FrameMsg, req *protocol.PollerCommitRequest,
	status string, details string) {
	result := protocol.PollerCommitResult{
		ZoneId:  req.Params.ZoneId,
		Version: req.Params.Version,
		Status:  status,
		Details: details,
	}

	resp := protocol.NewPollerPrepareCommitResponse(f, result)
	s.Respond(resp)
}

func (s *EleSession) respondFailureToPollerPrepare(f *protocol.FrameMsg, req protocol.PollerPrepareRequest, status string, details string) {
	if log.GetLevel() >= log.DebugLevel {
		log.WithFields(log.Fields{
			"prefix": s.logPrefix,
			"req":    req,
			"prep":   s.prepDetails.String(),
		}).Warn(details)
	}
	result := protocol.PollerPrepareResult{
		ZoneId:  req.GetPreparationZoneId(),
		Version: req.GetPreparationVersion(),
		Status:  status,
		Details: details,
	}
	resp := protocol.NewPollerPrepareResponse(f, result)
	s.Respond(resp)
}

// runHeartbeats is driven by a timer on heartbeatInterval
func (s *EleSession) runHeartbeats() {
	log.WithField("prefix", s.logPrefix).Debug("heartbeat starting")
	defer log.WithField("prefix", s.logPrefix).Debug("heartbeat exiting")
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(s.heartbeatInterval):
			req := protocol.NewHeartbeatRequest()
			req.SetId(&s.seq)
			s.prepareHeartbeatMeasurement(req)
			log.WithFields(log.Fields{
				"prefix": s.logPrefix,
				"req":    req,
			}).Debug("Sending heartbeat")
			s.Send(req)

		case resp := <-s.heartbeatResponses:
			s.updateHeartbeatMeasurement(resp)
		}
	}
}

func (s *EleSession) goRunHeartbeats() {
	go s.runHeartbeats()
}

func (s *EleSession) prepareHeartbeatMeasurement(req *protocol.HeartbeatRequest) {
	s.heartbeatMeasurement.expectedSeqID = req.Id
	s.heartbeatMeasurement.tracking.PollerSendTimestamp = req.Params.Timestamp
}

func (s *EleSession) updateHeartbeatMeasurement(resp *protocol.HeartbeatResponse) {
	if s.heartbeatMeasurement.expectedSeqID != resp.Id {
		log.WithFields(log.Fields{
			"prefix":   s.logPrefix,
			"expected": s.heartbeatMeasurement.expectedSeqID,
			"received": resp.Id,
		}).Warn("Received out of sequence heartbeat response. Unable to compute latency from it.")
		return
	}

	s.heartbeatMeasurement.tracking.PollerRecvTimestamp = utils.NowTimestampMillis()
	s.heartbeatMeasurement.tracking.ServerRecvTimestamp = resp.Result.Timestamp
	s.heartbeatMeasurement.tracking.ServerRespTimestamp = resp.Result.Timestamp

	offset, latency, err := s.heartbeatMeasurement.tracking.ComputeSkew()
	if err != nil {
		log.WithFields(log.Fields{
			"prefix": s.logPrefix,
			"error":  err,
		}).Warn("Failed to compute skew")
		return
	}
	s.heartbeatMeasurement.observations++
	s.heartbeatMeasurement.offset = offset
	s.heartbeatMeasurement.latency = latency
	log.WithFields(log.Fields{
		"prefix": s.logPrefix,
		"offset": offset,
		"delay":  latency,
	}).Debug("Computed poller-server latencies")
}

func (s *EleSession) GetClockOffset() int64 {
	return s.heartbeatMeasurement.offset
}

func (s *EleSession) GetLatency() int64 {
	return s.heartbeatMeasurement.latency
}

func (s *EleSession) HasLatencyMeasurements() bool {
	return s.heartbeatMeasurement.observations > 0
}

func (s *EleSession) addCompletion(frame protocol.Frame) {
	cFrame := &CompletionFrame{ID: frame.GetId(), Method: frame.GetMethod()}
	s.completionsMu.Lock()
	defer s.completionsMu.Unlock()
	s.completions[cFrame.ID] = cFrame
}

// runs it it's own go routine and consumes from sendCh
func (s *EleSession) runFrameSending() {
	log.WithField("prefix", s.logPrefix).Debug("send starting")
	defer log.WithField("prefix", s.logPrefix).Debug("send exiting")
	defer s.cancel()
	for {
		select {
		case <-s.ctx.Done():
			return
		case frame := <-s.sendCh:
			s.addCompletion(frame)
			data, err := frame.Encode()
			if err != nil {
				s.exitError(err)
				return
			}
			if log.GetLevel() >= log.DebugLevel {
				log.WithFields(log.Fields{
					"prefix": s.logPrefix,
					"data":   string(data),
				}).Debug("socket send")
			}
			s.connection.SetWriteDeadline(s.computeWriteDeadline())
			_, err = s.connection.GetFarendWriter().Write(data)
			if err != nil {
				s.exitError(err)
				return
			}
			s.connection.SetWriteDeadline(s.computeWriteDeadline())
			_, err = s.connection.GetFarendWriter().Write([]byte{'\r', '\n'})
			if err != nil {
				s.exitError(err)
				return
			}
		}
	}
}

func (s *EleSession) exitError(err error) {
	log.WithFields(log.Fields{
		"prefix": s.logPrefix,
		"error":  err,
	}).Warn("Session exiting with error")
	s.EmitEvent(utils.NewEvent(EventTypeSendError, err))
	s.Close()
}

// Close shuts down session's context and closes session
func (s *EleSession) Close() {
	s.cancel()
}

// Done returns the context-done channel scoped to this session
func (s *EleSession) Done() <-chan struct{} {
	return s.ctx.Done()
}

func (cp *prepDetails) String() string {
	json, _ := json.Marshal(cp)
	return string(json)
}

func (cp *prepDetails) clear() {
	cp.activePrep = nil
	cp.srcPrepMsg = nil
	cp.prepared = false
	if cp.prepareToEndTimer != nil {
		cp.prepareToEndTimer.Stop()
		cp.prepareToEndTimer = nil
	}
}

func (cp *prepDetails) commit() {
	cp.reconciler.ReconcileChecks(cp.activePrep)
	cp.clear()
}
