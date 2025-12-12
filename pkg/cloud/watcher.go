/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package cloud

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	osc "github.com/outscale/osc-sdk-go/v2"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

type result[T any] struct {
	ok  *T
	err error
}

func resultOk[T any](t *T) result[T] {
	return result[T]{ok: t}
}

func resultError[T any](err error) result[T] {
	return result[T]{err: err}
}

type watcher[T any] struct {
	ctx   context.Context // the context of the waiter
	id    string
	until func(r *T) (ok bool, err error)
	resp  chan result[T]
}

type ResourceWatcher[T any] struct {
	interval time.Duration
	refresh  func(ctx context.Context, ids []string) ([]*T, error)

	in       chan watcher[T]
	watchers map[string]watcher[T]
}

func NewResourceWatcher[T any](interval time.Duration,
	refresh func(ctx context.Context, ids []string) ([]*T, error)) *ResourceWatcher[T] {
	return &ResourceWatcher[T]{
		interval: interval,
		refresh:  refresh,

		in:       make(chan watcher[T]),
		watchers: make(map[string]watcher[T]),
	}
}

func NewSnapshotWatcher(interval time.Duration, client OscInterface) *ResourceWatcher[osc.Snapshot] {
	return NewResourceWatcher(interval, func(ctx context.Context, ids []string) ([]*osc.Snapshot, error) {
		req := osc.ReadSnapshotsRequest{
			Filters: &osc.FiltersSnapshot{
				SnapshotIds: &ids,
			},
			ResultsPerPage: ptr.To(int32(len(ids))), //nolint:gosec
		}
		resp, httpRes, err := client.ReadSnapshots(ctx, req)
		logAPICall(ctx, "ReadSnapshots", req, resp, httpRes, err)
		if err != nil {
			return nil, fmt.Errorf("read snapshots: %w", err)
		}
		rsnaps := resp.GetSnapshots()
		snaps := make([]*osc.Snapshot, len(ids))
		for i, id := range ids {
			j := slices.IndexFunc(rsnaps, func(snap osc.Snapshot) bool { return snap.GetSnapshotId() == id })
			if j >= 0 {
				snaps[i] = &rsnaps[j]
			}
		}
		return snaps, nil
	})
}

func NewVolumeWatcher(interval time.Duration, client OscInterface) *ResourceWatcher[osc.Volume] {
	return NewResourceWatcher(interval, func(ctx context.Context, ids []string) ([]*osc.Volume, error) {
		req := osc.ReadVolumesRequest{
			Filters: &osc.FiltersVolume{
				VolumeIds: &ids,
			},
			ResultsPerPage: ptr.To(int32(len(ids))), //nolint:gosec
		}
		resp, httpRes, err := client.ReadVolumes(ctx, req)
		logAPICall(ctx, "ReadVolumes", req, resp, httpRes, err)
		if err != nil {
			return nil, fmt.Errorf("read volumes: %w", err)
		}
		rvols := resp.GetVolumes()
		vols := make([]*osc.Volume, len(ids))
		for i, id := range ids {
			j := slices.IndexFunc(rvols, func(vol osc.Volume) bool { return vol.GetVolumeId() == id })
			if j >= 0 {
				vols[i] = &rvols[j]
			}
		}
		return vols, nil
	})
}

func (sw *ResourceWatcher[T]) Run(ctx context.Context) {
	logger := klog.FromContext(ctx)
	t := time.NewTicker(sw.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case in := <-sw.in:
			sw.watchers[in.id] = in
		case <-t.C:
			if len(sw.watchers) == 0 {
				continue
			}
			ids := slices.Collect(maps.Keys(sw.watchers))
			logger.V(5).Info("Watching resources", "count", len(ids))
			rsrcs, err := sw.refresh(ctx, ids)
			if err != nil {
				logger.V(3).Error(err, "unable to check statuses")
				continue
			}
			for i, rsrc := range rsrcs {
				w, found := sw.watchers[ids[i]]
				if !found { // should not occur
					continue
				}
				ok, err := w.until(rsrc)
				switch {
				case ok:
					logger.V(5).Info("Resource is ok", "id", ids[i])
					sw.response(ctx, w, resultOk(rsrc))
					delete(sw.watchers, ids[i])
					close(w.resp)
				case err != nil:
					logger.V(5).Info("Resource is in error", "id", ids[i])
					sw.response(ctx, w, resultError[T](err))
					delete(sw.watchers, ids[i])
					close(w.resp)
				default:
					logger.V(5).Info("Resource is not ready", "id", ids[i])
				}
			}
		}
	}
}

func (sw *ResourceWatcher[T]) response(ctx context.Context, w watcher[T], res result[T]) {
	select {
	case <-ctx.Done():
	case <-w.ctx.Done(): // we do not want to block if the waiter context has been cancelled.
	case w.resp <- res: // send the response, would block without the previous test...
	}
}

func (sw *ResourceWatcher[T]) WaitUntil(ctx context.Context, id string, until func(r *T) (ok bool, err error)) (r *T, err error) {
	logger := klog.FromContext(ctx)
	start := time.Now()
	defer func() {
		logger.V(4).Info("End of wait", "success", err == nil, "duration", time.Since(start))
	}()
	resp := make(chan result[T], 1)
	w := watcher[T]{ctx: ctx, id: id, until: until, resp: resp}
	// send (unless context has been cancelled)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case sw.in <- w:
	}
	// receive (unless context has been cancelled)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-w.resp:
		return res.ok, res.err
	}
}
