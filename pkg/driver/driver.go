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
	"context"
	"fmt"
	"net"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/outscale/osc-bsu-csi-driver/cmd/options"
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

func (m Mode) HasController() bool {
	return m == ControllerMode || m == AllMode
}

func (m Mode) HasNode() bool {
	return m == NodeMode || m == AllMode
}

const (
	DriverName     = "bsu.csi.outscale.com"
	TopologyKey    = "topology." + DriverName + "/zone"
	TopologyK8sKey = "topology.kubernetes.io/zone"
)

type Driver struct {
	controllerService
	nodeService

	cancel func()

	srv     *grpc.Server
	options *DriverOptions

	csi.UnimplementedIdentityServer
}

type DriverOptions struct {
	mode Mode

	// Controller options
	endpoint          string
	extraVolumeTags   map[string]string
	extraSnapshotTags map[string]string
	cloudOptions      options.CloudOptions

	// Node options
	luksOpenFlags []string

	// overriden services
	mounter Mounter
}

func NewDriver(ctx context.Context, opts ...func(*DriverOptions)) (*Driver, error) {
	driverOptions := DriverOptions{
		mode:     AllMode,
		endpoint: options.DefaultCSIEndpoint,
	}
	for _, option := range opts {
		option(&driverOptions)
	}

	if err := ValidateDriverOptions(&driverOptions); err != nil {
		return nil, fmt.Errorf("invalid driver options: %w", err)
	}

	driver := Driver{
		options: &driverOptions,
	}

	var err error
	// no need to test for invalid modes, as ValidateDriverOptions has already done it.
	if driverOptions.mode.HasController() {
		driver.controllerService = newControllerService(ctx, &driverOptions)
	}
	if driverOptions.mode.HasNode() {
		driver.nodeService, err = newNodeService(ctx, &driverOptions)
	}

	return &driver, err
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
func (d *Driver) Run(ctx context.Context) error {
	version := util.GetVersion().DriverVersion
	klog.V(3).InfoS(fmt.Sprintf("Driver: %v Version: %v", DriverName, version))
	klog.V(1).InfoS("Running in mode: " + string(d.options.mode))

	scheme, addr, err := util.ParseEndpoint(d.options.endpoint)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, scheme, addr)
	if err != nil {
		return err
	}
	klog.V(3).InfoS("Listening for connections on: " + listener.Addr().String())

	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(LoggingInterceptor(version)),
	}
	d.srv = grpc.NewServer(opts...)

	csi.RegisterIdentityServer(d.srv, d)

	if d.options.mode.HasController() {
		csi.RegisterControllerServer(d.srv, d)
		if err := d.controllerService.Start(ctx); err != nil { //nolint:staticcheck
			return err
		}
	}
	if d.options.mode.HasNode() {
		csi.RegisterNodeServer(d.srv, d)
		if err := d.checkTools(); err != nil {
			return err
		}
	}

	return d.srv.Serve(listener)
}

func (d *Driver) Stop() {
	klog.V(0).InfoS("Stopping server")
	d.srv.Stop()
	d.cancel()
}

func WithEndpoint(endpoint string) func(*DriverOptions) {
	return func(o *DriverOptions) {
		o.endpoint = endpoint
	}
}

func WithExtraVolumeTags(tags map[string]string) func(*DriverOptions) {
	return func(o *DriverOptions) {
		o.extraVolumeTags = tags
	}
}

func WithExtraSnapshotTags(tags map[string]string) func(*DriverOptions) {
	return func(o *DriverOptions) {
		o.extraSnapshotTags = tags
	}
}

func WithLuksOpenFlags(flags []string) func(*DriverOptions) {
	return func(o *DriverOptions) {
		o.luksOpenFlags = flags
	}
}

func WithMode(mode Mode) func(*DriverOptions) {
	return func(o *DriverOptions) {
		o.mode = mode
	}
}

func WithCloudOptions(opts options.CloudOptions) func(*DriverOptions) {
	return func(o *DriverOptions) {
		o.cloudOptions = opts
	}
}

func WithMounter(mounter Mounter) func(*DriverOptions) {
	return func(o *DriverOptions) {
		o.mounter = mounter
	}
}
