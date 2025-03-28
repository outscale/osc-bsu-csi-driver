// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/outscale/osc-bsu-csi-driver/pkg/driver (interfaces: Mounter)

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	luks "github.com/outscale/osc-bsu-csi-driver/pkg/driver/luks"
	exec "k8s.io/utils/exec"
	mount "k8s.io/utils/mount"
)

// MockMounter is a mock of Mounter interface.
type MockMounter struct {
	ctrl     *gomock.Controller
	recorder *MockMounterMockRecorder
}

// MockMounterMockRecorder is the mock recorder for MockMounter.
type MockMounterMockRecorder struct {
	mock *MockMounter
}

// NewMockMounter creates a new mock instance.
func NewMockMounter(ctrl *gomock.Controller) *MockMounter {
	mock := &MockMounter{ctrl: ctrl}
	mock.recorder = &MockMounterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockMounter) EXPECT() *MockMounterMockRecorder {
	return m.recorder
}

// CheckLuksPassphrase mocks base method.
func (m *MockMounter) CheckLuksPassphrase(arg0, arg1 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CheckLuksPassphrase", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// CheckLuksPassphrase indicates an expected call of CheckLuksPassphrase.
func (mr *MockMounterMockRecorder) CheckLuksPassphrase(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CheckLuksPassphrase", reflect.TypeOf((*MockMounter)(nil).CheckLuksPassphrase), arg0, arg1)
}

// Command mocks base method.
func (m *MockMounter) Command(arg0 string, arg1 ...string) exec.Cmd {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0}
	for _, a := range arg1 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Command", varargs...)
	ret0, _ := ret[0].(exec.Cmd)
	return ret0
}

// Command indicates an expected call of Command.
func (mr *MockMounterMockRecorder) Command(arg0 interface{}, arg1 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0}, arg1...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Command", reflect.TypeOf((*MockMounter)(nil).Command), varargs...)
}

// CommandContext mocks base method.
func (m *MockMounter) CommandContext(arg0 context.Context, arg1 string, arg2 ...string) exec.Cmd {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "CommandContext", varargs...)
	ret0, _ := ret[0].(exec.Cmd)
	return ret0
}

// CommandContext indicates an expected call of CommandContext.
func (mr *MockMounterMockRecorder) CommandContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CommandContext", reflect.TypeOf((*MockMounter)(nil).CommandContext), varargs...)
}

// ExistsPath mocks base method.
func (m *MockMounter) ExistsPath(arg0 string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ExistsPath", arg0)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ExistsPath indicates an expected call of ExistsPath.
func (mr *MockMounterMockRecorder) ExistsPath(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ExistsPath", reflect.TypeOf((*MockMounter)(nil).ExistsPath), arg0)
}

// FormatAndMount mocks base method.
func (m *MockMounter) FormatAndMount(arg0, arg1, arg2 string, arg3 []string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FormatAndMount", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// FormatAndMount indicates an expected call of FormatAndMount.
func (mr *MockMounterMockRecorder) FormatAndMount(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FormatAndMount", reflect.TypeOf((*MockMounter)(nil).FormatAndMount), arg0, arg1, arg2, arg3)
}

// GetDeviceName mocks base method.
func (m *MockMounter) GetDeviceName(arg0 string) (string, int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetDeviceName", arg0)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(int)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetDeviceName indicates an expected call of GetDeviceName.
func (mr *MockMounterMockRecorder) GetDeviceName(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetDeviceName", reflect.TypeOf((*MockMounter)(nil).GetDeviceName), arg0)
}

// GetDiskFormat mocks base method.
func (m *MockMounter) GetDiskFormat(arg0 string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetDiskFormat", arg0)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetDiskFormat indicates an expected call of GetDiskFormat.
func (mr *MockMounterMockRecorder) GetDiskFormat(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetDiskFormat", reflect.TypeOf((*MockMounter)(nil).GetDiskFormat), arg0)
}

// GetMountRefs mocks base method.
func (m *MockMounter) GetMountRefs(arg0 string) ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMountRefs", arg0)
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetMountRefs indicates an expected call of GetMountRefs.
func (mr *MockMounterMockRecorder) GetMountRefs(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMountRefs", reflect.TypeOf((*MockMounter)(nil).GetMountRefs), arg0)
}

// IsCorruptedMnt mocks base method.
func (m *MockMounter) IsCorruptedMnt(arg0 error) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsCorruptedMnt", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// IsCorruptedMnt indicates an expected call of IsCorruptedMnt.
func (mr *MockMounterMockRecorder) IsCorruptedMnt(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsCorruptedMnt", reflect.TypeOf((*MockMounter)(nil).IsCorruptedMnt), arg0)
}

// IsLikelyNotMountPoint mocks base method.
func (m *MockMounter) IsLikelyNotMountPoint(arg0 string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsLikelyNotMountPoint", arg0)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// IsLikelyNotMountPoint indicates an expected call of IsLikelyNotMountPoint.
func (mr *MockMounterMockRecorder) IsLikelyNotMountPoint(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsLikelyNotMountPoint", reflect.TypeOf((*MockMounter)(nil).IsLikelyNotMountPoint), arg0)
}

// IsLuks mocks base method.
func (m *MockMounter) IsLuks(arg0 string) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsLuks", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// IsLuks indicates an expected call of IsLuks.
func (mr *MockMounterMockRecorder) IsLuks(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsLuks", reflect.TypeOf((*MockMounter)(nil).IsLuks), arg0)
}

// IsLuksMapping mocks base method.
func (m *MockMounter) IsLuksMapping(arg0 string) (bool, string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsLuksMapping", arg0)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(string)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// IsLuksMapping indicates an expected call of IsLuksMapping.
func (mr *MockMounterMockRecorder) IsLuksMapping(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsLuksMapping", reflect.TypeOf((*MockMounter)(nil).IsLuksMapping), arg0)
}

// List mocks base method.
func (m *MockMounter) List() ([]mount.MountPoint, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "List")
	ret0, _ := ret[0].([]mount.MountPoint)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// List indicates an expected call of List.
func (mr *MockMounterMockRecorder) List() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "List", reflect.TypeOf((*MockMounter)(nil).List))
}

// LookPath mocks base method.
func (m *MockMounter) LookPath(arg0 string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LookPath", arg0)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// LookPath indicates an expected call of LookPath.
func (mr *MockMounterMockRecorder) LookPath(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LookPath", reflect.TypeOf((*MockMounter)(nil).LookPath), arg0)
}

// LuksClose mocks base method.
func (m *MockMounter) LuksClose(arg0 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LuksClose", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// LuksClose indicates an expected call of LuksClose.
func (mr *MockMounterMockRecorder) LuksClose(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LuksClose", reflect.TypeOf((*MockMounter)(nil).LuksClose), arg0)
}

// LuksFormat mocks base method.
func (m *MockMounter) LuksFormat(arg0, arg1 string, arg2 luks.LuksContext) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LuksFormat", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// LuksFormat indicates an expected call of LuksFormat.
func (mr *MockMounterMockRecorder) LuksFormat(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LuksFormat", reflect.TypeOf((*MockMounter)(nil).LuksFormat), arg0, arg1, arg2)
}

// LuksOpen mocks base method.
func (m *MockMounter) LuksOpen(arg0, arg1, arg2 string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LuksOpen", arg0, arg1, arg2)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// LuksOpen indicates an expected call of LuksOpen.
func (mr *MockMounterMockRecorder) LuksOpen(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LuksOpen", reflect.TypeOf((*MockMounter)(nil).LuksOpen), arg0, arg1, arg2)
}

// LuksResize mocks base method.
func (m *MockMounter) LuksResize(arg0, arg1 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LuksResize", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// LuksResize indicates an expected call of LuksResize.
func (mr *MockMounterMockRecorder) LuksResize(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LuksResize", reflect.TypeOf((*MockMounter)(nil).LuksResize), arg0, arg1)
}

// MakeDir mocks base method.
func (m *MockMounter) MakeDir(arg0 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MakeDir", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// MakeDir indicates an expected call of MakeDir.
func (mr *MockMounterMockRecorder) MakeDir(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MakeDir", reflect.TypeOf((*MockMounter)(nil).MakeDir), arg0)
}

// MakeFile mocks base method.
func (m *MockMounter) MakeFile(arg0 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MakeFile", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// MakeFile indicates an expected call of MakeFile.
func (mr *MockMounterMockRecorder) MakeFile(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MakeFile", reflect.TypeOf((*MockMounter)(nil).MakeFile), arg0)
}

// Mount mocks base method.
func (m *MockMounter) Mount(arg0, arg1, arg2 string, arg3 []string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Mount", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// Mount indicates an expected call of Mount.
func (mr *MockMounterMockRecorder) Mount(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Mount", reflect.TypeOf((*MockMounter)(nil).Mount), arg0, arg1, arg2, arg3)
}

// MountSensitive mocks base method.
func (m *MockMounter) MountSensitive(arg0, arg1, arg2 string, arg3, arg4 []string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MountSensitive", arg0, arg1, arg2, arg3, arg4)
	ret0, _ := ret[0].(error)
	return ret0
}

// MountSensitive indicates an expected call of MountSensitive.
func (mr *MockMounterMockRecorder) MountSensitive(arg0, arg1, arg2, arg3, arg4 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MountSensitive", reflect.TypeOf((*MockMounter)(nil).MountSensitive), arg0, arg1, arg2, arg3, arg4)
}

// Unmount mocks base method.
func (m *MockMounter) Unmount(arg0 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Unmount", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Unmount indicates an expected call of Unmount.
func (mr *MockMounterMockRecorder) Unmount(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Unmount", reflect.TypeOf((*MockMounter)(nil).Unmount), arg0)
}
