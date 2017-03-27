// Automatically generated by MockGen. DO NOT EDIT!
// Source: utils/events.go

package utils

import (
	gomock "github.com/golang/mock/gomock"
)

// Mock of Event interface
type MockEvent struct {
	ctrl     *gomock.Controller
	recorder *_MockEventRecorder
}

// Recorder for MockEvent (not exported)
type _MockEventRecorder struct {
	mock *MockEvent
}

func NewMockEvent(ctrl *gomock.Controller) *MockEvent {
	mock := &MockEvent{ctrl: ctrl}
	mock.recorder = &_MockEventRecorder{mock}
	return mock
}

func (_m *MockEvent) EXPECT() *_MockEventRecorder {
	return _m.recorder
}

func (_m *MockEvent) Type() string {
	ret := _m.ctrl.Call(_m, "Type")
	ret0, _ := ret[0].(string)
	return ret0
}

func (_mr *_MockEventRecorder) Type() *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "Type")
}

func (_m *MockEvent) Target() interface{} {
	ret := _m.ctrl.Call(_m, "Target")
	ret0, _ := ret[0].(interface{})
	return ret0
}

func (_mr *_MockEventRecorder) Target() *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "Target")
}

// Mock of EventConsumer interface
type MockEventConsumer struct {
	ctrl     *gomock.Controller
	recorder *_MockEventConsumerRecorder
}

// Recorder for MockEventConsumer (not exported)
type _MockEventConsumerRecorder struct {
	mock *MockEventConsumer
}

func NewMockEventConsumer(ctrl *gomock.Controller) *MockEventConsumer {
	mock := &MockEventConsumer{ctrl: ctrl}
	mock.recorder = &_MockEventConsumerRecorder{mock}
	return mock
}

func (_m *MockEventConsumer) EXPECT() *_MockEventConsumerRecorder {
	return _m.recorder
}

func (_m *MockEventConsumer) HandleEvent(evt Event) error {
	ret := _m.ctrl.Call(_m, "HandleEvent", evt)
	ret0, _ := ret[0].(error)
	return ret0
}

func (_mr *_MockEventConsumerRecorder) HandleEvent(arg0 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "HandleEvent", arg0)
}

// Mock of EventSource interface
type MockEventSource struct {
	ctrl     *gomock.Controller
	recorder *_MockEventSourceRecorder
}

// Recorder for MockEventSource (not exported)
type _MockEventSourceRecorder struct {
	mock *MockEventSource
}

func NewMockEventSource(ctrl *gomock.Controller) *MockEventSource {
	mock := &MockEventSource{ctrl: ctrl}
	mock.recorder = &_MockEventSourceRecorder{mock}
	return mock
}

func (_m *MockEventSource) EXPECT() *_MockEventSourceRecorder {
	return _m.recorder
}

func (_m *MockEventSource) RegisterEventConsumer(consumer EventConsumer) {
	_m.ctrl.Call(_m, "RegisterEventConsumer", consumer)
}

func (_mr *_MockEventSourceRecorder) RegisterEventConsumer(arg0 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "RegisterEventConsumer", arg0)
}

func (_m *MockEventSource) DeregisterEventConsumer(consumer EventConsumer) {
	_m.ctrl.Call(_m, "DeregisterEventConsumer", consumer)
}

func (_mr *_MockEventSourceRecorder) DeregisterEventConsumer(arg0 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "DeregisterEventConsumer", arg0)
}