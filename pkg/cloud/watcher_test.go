package cloud_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/outscale/osc-bsu-csi-driver/pkg/cloud"
	"github.com/outscale/osc-bsu-csi-driver/pkg/cloud/mocks"
	osc "github.com/outscale/osc-sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"k8s.io/utils/ptr"
)

func TestResourceWatcher_Volumes(t *testing.T) {
	t.Run("When concurrent calls are made, the right status is returned to the right volume", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
		mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Cond(func(req osc.ReadVolumesRequest) bool {
			return len(*req.Filters.VolumeIds) == 4
		})).Return(osc.ReadVolumesResponse{Volumes: &[]osc.Volume{
			{VolumeId: ptr.To("id-error"), State: ptr.To("error")},
			{VolumeId: ptr.To("id-ready"), State: ptr.To("ready")},
			{VolumeId: ptr.To("id-attached"), State: ptr.To("attached")},
			{VolumeId: ptr.To("id-detached"), State: ptr.To("detached")},
		}}, nil, nil).MinTimes(1)

		rw := cloud.NewVolumeWatcher(time.Second, mockOscInterface)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		go rw.Run(ctx)

		wg := sync.WaitGroup{}
		for _, state := range []string{"ready", "detached", "attached", "error"} {
			wg.Go(func() {
				time.Sleep(time.Duration(rand.IntN(100)) * time.Millisecond)
				v, err := rw.WaitUntil(ctx, "id-"+state, func(v *osc.Volume) (bool, error) {
					if *v.State != state {
						return false, errors.New("invalid state")
					}
					return true, nil
				})
				require.NoError(t, err)
				assert.Equal(t, "id-"+state, v.GetVolumeId())
				assert.Equal(t, state, v.GetState())
			})
		}
		wg.Wait()
	})
	t.Run("Errors are returned", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
		mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: &[]osc.Volume{
			{VolumeId: ptr.To("id-error"), State: ptr.To("error")},
		}}, nil, nil).MinTimes(1)

		rw := cloud.NewVolumeWatcher(time.Second, mockOscInterface)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		go rw.Run(ctx)

		_, err := rw.WaitUntil(ctx, "id-error", func(v *osc.Volume) (bool, error) {
			return false, errors.New("error")
		})
		require.Error(t, err)
	})
}

func TestResourceWatcher_Snapshots(t *testing.T) {
	t.Run("When concurrent calls are made, the right status is returned to the right snapshot", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Cond(func(req osc.ReadSnapshotsRequest) bool {
			return len(*req.Filters.SnapshotIds) == 4
		})).Return(osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{
			{SnapshotId: ptr.To("id-error"), State: ptr.To("error")},
			{SnapshotId: ptr.To("id-ready"), State: ptr.To("ready")},
			{SnapshotId: ptr.To("id-attached"), State: ptr.To("attached")},
			{SnapshotId: ptr.To("id-detached"), State: ptr.To("detached")},
		}}, nil, nil).MinTimes(1)

		rw := cloud.NewSnapshotWatcher(time.Second, mockOscInterface)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		go rw.Run(ctx)

		wg := sync.WaitGroup{}
		for _, state := range []string{"ready", "detached", "attached", "error"} {
			wg.Go(func() {
				time.Sleep(time.Duration(rand.IntN(100)) * time.Millisecond)
				s, err := rw.WaitUntil(ctx, "id-"+state, func(v *osc.Snapshot) (bool, error) {
					if *v.State != state {
						return false, errors.New("invalid state")
					}
					return true, nil
				})
				require.NoError(t, err)
				assert.Equal(t, "id-"+state, s.GetSnapshotId())
				assert.Equal(t, state, s.GetState())
			})
		}
		wg.Wait()
	})
	t.Run("Errors are returned", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{
			{SnapshotId: ptr.To("id-error"), State: ptr.To("error")},
		}}, nil, nil).MinTimes(1)

		rw := cloud.NewSnapshotWatcher(time.Second, mockOscInterface)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		go rw.Run(ctx)

		_, err := rw.WaitUntil(ctx, "id-error", func(v *osc.Snapshot) (bool, error) {
			return false, errors.New("error")
		})
		require.Error(t, err)
	})
}
