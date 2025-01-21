/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver

import (
	"fmt"
	"net"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/outscale/osc-bsu-csi-driver/pkg/util"
	"google.golang.org/grpc"
	klog "k8s.io/klog/v2"
)

// Mode is the operating mode of the CSI driver.
type Mode string

const (
	// ControllerMode is the mode that only starts the controller service.
	ControllerMode Mode = "controller"
	// NodeMode is the mode that only starts the node service.
	NodeMode Mode = "node"
	// AllMode is the mode that only starts both the controller and the node service.
	AllMode Mode = "all"
)

const (
	DriverName     = "bsu.csi.outscale.com"
	TopologyKey    = "topology." + DriverName + "/zone"
	TopologyK8sKey = "topology.kubernetes.io/zone"
)

type Driver struct {
	controllerService
	nodeService

	srv     *grpc.Server
	options *DriverOptions
}

type DriverOptions struct {
	endpoint        string
	extraVolumeTags map[string]string
	mode            Mode
}

func NewDriver(options ...func(*DriverOptions)) (*Driver, error) {
	driverOptions := DriverOptions{
		endpoint: DefaultCSIEndpoint,
		mode:     AllMode,
	}
	for _, option := range options {
		option(&driverOptions)
	}

	if err := ValidateDriverOptions(&driverOptions); err != nil {
		return nil, fmt.Errorf("Invalid driver options: %w", err)
	}

	driver := Driver{
		options: &driverOptions,
	}

	switch driverOptions.mode {
	case ControllerMode:
		driver.controllerService = newControllerService(&driverOptions)
	case NodeMode:
		driver.nodeService = newNodeService()
	case AllMode:
		driver.controllerService = newControllerService(&driverOptions)
		driver.nodeService = newNodeService()
	default:
		return nil, fmt.Errorf("unknown mode: %s", driverOptions.mode)
	}

	return &driver, nil
}

func (d *Driver) checkTools() error {
	if d.mounter == nil {
		return nil
	}

	out, err := d.mounter.Command("mkfs.ext4", "-V").CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkfs.ext4 issue: %s", strings.TrimSpace(string(out)))
	}
	klog.V(3).InfoS(string(out))

	out, err = d.mounter.Command("mkfs.xfs", "-V").CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkfs.xfs issue: %s", strings.TrimSpace(string(out)))
	}
	klog.V(3).InfoS(string(out))

	out, err = d.mounter.Command("cryptsetup", "-V").CombinedOutput()
	if err != nil {
		return fmt.Errorf("cryptsetup issue: %s", strings.TrimSpace(string(out)))
	}
	klog.V(3).InfoS(string(out))

	return nil
}
func (d *Driver) Run() error {
	version := util.GetVersion().DriverVersion
	klog.V(1).InfoS(fmt.Sprintf("Driver: %v Version: %v", DriverName, version))
	if err := d.checkTools(); err != nil {
		return err
	}

	scheme, addr, err := util.ParseEndpoint(d.options.endpoint)
	if err != nil {
		return err
	}
	listener, err := net.Listen(scheme, addr)
	if err != nil {
		return err
	}

	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(LoggingInterceptor(version)),
	}
	d.srv = grpc.NewServer(opts...)

	csi.RegisterIdentityServer(d.srv, d)

	switch d.options.mode {
	case ControllerMode:
		csi.RegisterControllerServer(d.srv, d)
	case NodeMode:
		csi.RegisterNodeServer(d.srv, d)
	case AllMode:
		csi.RegisterControllerServer(d.srv, d)
		csi.RegisterNodeServer(d.srv, d)
	default:
		return fmt.Errorf("unknown mode: %s", d.options.mode)
	}

	klog.V(1).InfoS("Running in mode: " + string(d.options.mode))
	klog.V(1).InfoS("Listening for connections on: " + listener.Addr().String())
	return d.srv.Serve(listener)
}

func (d *Driver) Stop() {
	klog.V(0).InfoS("Stopping server")
	d.srv.Stop()
}

func WithEndpoint(endpoint string) func(*DriverOptions) {
	return func(o *DriverOptions) {
		o.endpoint = endpoint
	}
}

func WithExtraVolumeTags(extraVolumeTags map[string]string) func(*DriverOptions) {
	return func(o *DriverOptions) {
		o.extraVolumeTags = extraVolumeTags
	}
}

func WithMode(mode Mode) func(*DriverOptions) {
	return func(o *DriverOptions) {
		o.mode = mode
	}
}
