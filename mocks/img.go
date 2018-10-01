// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/google/go-containerregistry/pkg/v1 (interfaces: Image)

// Package mocks is a generated GoMock package.
package mocks

import (
	gomock "github.com/golang/mock/gomock"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	types "github.com/google/go-containerregistry/pkg/v1/types"
	reflect "reflect"
)

// MockImage is a mock of Image interface
type MockImage struct {
	ctrl     *gomock.Controller
	recorder *MockImageMockRecorder
}

// MockImageMockRecorder is the mock recorder for MockImage
type MockImageMockRecorder struct {
	mock *MockImage
}

// NewMockImage creates a new mock instance
func NewMockImage(ctrl *gomock.Controller) *MockImage {
	mock := &MockImage{ctrl: ctrl}
	mock.recorder = &MockImageMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockImage) EXPECT() *MockImageMockRecorder {
	return m.recorder
}

// BlobSet mocks base method
func (m *MockImage) BlobSet() (map[v1.Hash]struct{}, error) {
	ret := m.ctrl.Call(m, "BlobSet")
	ret0, _ := ret[0].(map[v1.Hash]struct{})
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BlobSet indicates an expected call of BlobSet
func (mr *MockImageMockRecorder) BlobSet() *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BlobSet", reflect.TypeOf((*MockImage)(nil).BlobSet))
}

// ConfigFile mocks base method
func (m *MockImage) ConfigFile() (*v1.ConfigFile, error) {
	ret := m.ctrl.Call(m, "ConfigFile")
	ret0, _ := ret[0].(*v1.ConfigFile)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ConfigFile indicates an expected call of ConfigFile
func (mr *MockImageMockRecorder) ConfigFile() *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ConfigFile", reflect.TypeOf((*MockImage)(nil).ConfigFile))
}

// ConfigName mocks base method
func (m *MockImage) ConfigName() (v1.Hash, error) {
	ret := m.ctrl.Call(m, "ConfigName")
	ret0, _ := ret[0].(v1.Hash)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ConfigName indicates an expected call of ConfigName
func (mr *MockImageMockRecorder) ConfigName() *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ConfigName", reflect.TypeOf((*MockImage)(nil).ConfigName))
}

// Digest mocks base method
func (m *MockImage) Digest() (v1.Hash, error) {
	ret := m.ctrl.Call(m, "Digest")
	ret0, _ := ret[0].(v1.Hash)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Digest indicates an expected call of Digest
func (mr *MockImageMockRecorder) Digest() *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Digest", reflect.TypeOf((*MockImage)(nil).Digest))
}

// LayerByDiffID mocks base method
func (m *MockImage) LayerByDiffID(arg0 v1.Hash) (v1.Layer, error) {
	ret := m.ctrl.Call(m, "LayerByDiffID", arg0)
	ret0, _ := ret[0].(v1.Layer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// LayerByDiffID indicates an expected call of LayerByDiffID
func (mr *MockImageMockRecorder) LayerByDiffID(arg0 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LayerByDiffID", reflect.TypeOf((*MockImage)(nil).LayerByDiffID), arg0)
}

// LayerByDigest mocks base method
func (m *MockImage) LayerByDigest(arg0 v1.Hash) (v1.Layer, error) {
	ret := m.ctrl.Call(m, "LayerByDigest", arg0)
	ret0, _ := ret[0].(v1.Layer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// LayerByDigest indicates an expected call of LayerByDigest
func (mr *MockImageMockRecorder) LayerByDigest(arg0 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LayerByDigest", reflect.TypeOf((*MockImage)(nil).LayerByDigest), arg0)
}

// Layers mocks base method
func (m *MockImage) Layers() ([]v1.Layer, error) {
	ret := m.ctrl.Call(m, "Layers")
	ret0, _ := ret[0].([]v1.Layer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Layers indicates an expected call of Layers
func (mr *MockImageMockRecorder) Layers() *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Layers", reflect.TypeOf((*MockImage)(nil).Layers))
}

// Manifest mocks base method
func (m *MockImage) Manifest() (*v1.Manifest, error) {
	ret := m.ctrl.Call(m, "Manifest")
	ret0, _ := ret[0].(*v1.Manifest)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Manifest indicates an expected call of Manifest
func (mr *MockImageMockRecorder) Manifest() *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Manifest", reflect.TypeOf((*MockImage)(nil).Manifest))
}

// MediaType mocks base method
func (m *MockImage) MediaType() (types.MediaType, error) {
	ret := m.ctrl.Call(m, "MediaType")
	ret0, _ := ret[0].(types.MediaType)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// MediaType indicates an expected call of MediaType
func (mr *MockImageMockRecorder) MediaType() *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MediaType", reflect.TypeOf((*MockImage)(nil).MediaType))
}

// RawConfigFile mocks base method
func (m *MockImage) RawConfigFile() ([]byte, error) {
	ret := m.ctrl.Call(m, "RawConfigFile")
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// RawConfigFile indicates an expected call of RawConfigFile
func (mr *MockImageMockRecorder) RawConfigFile() *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RawConfigFile", reflect.TypeOf((*MockImage)(nil).RawConfigFile))
}

// RawManifest mocks base method
func (m *MockImage) RawManifest() ([]byte, error) {
	ret := m.ctrl.Call(m, "RawManifest")
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// RawManifest indicates an expected call of RawManifest
func (mr *MockImageMockRecorder) RawManifest() *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RawManifest", reflect.TypeOf((*MockImage)(nil).RawManifest))
}
