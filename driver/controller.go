/*
Copyright 2020 Vultr Authors.
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
	"strconv"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	_   = iota
	kiB = 1 << (10 * iota)
	miB
	giB
	tiB
)

const (
	minVolumeSizeInBytes      int64 = 1 * giB
	maxVolumeSizeInBytes      int64 = 10 * tiB
	defaultVolumeSizeInBytes  int64 = 10 * giB
	volumeStatusCheckRetries        = 10
	volumeStatusCheckInterval       = 1
)

var (
	supportedVolCapabilities = &csi.VolumeCapability_AccessMode{
		Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	}
)

var _ csi.ControllerServer = &VultrControllerServer{}

type VultrControllerServer struct {
	Driver *VultrDriver
}

func NewVultrControllerServer(driver *VultrDriver) *VultrControllerServer {
	return &VultrControllerServer{Driver: driver}
}

// CreateVolume provisions a new volume on behalf of the user
func (c *VultrControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	volName := req.Name

	if volName == "" {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Name is missing")
	}

	if req.VolumeCapabilities == nil || len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Volume Capabilities is missing")
	}

	// Validate
	if !isValidCapability(req.VolumeCapabilities) {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume Volume capability is not compatible: %v", req)
	}

	// check that the volume doesnt already exist
	curVolume, err := c.Driver.client.BlockStorage.Get(context.TODO(), volName)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if curVolume != nil {
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      curVolume.BlockStorageID,
				CapacityBytes: int64(curVolume.SizeGB) * giB,
			},
		}, nil
	}

	// if applicable, create volume
	region, err := strconv.Atoi(c.Driver.region)
	if err != nil {
		return nil, status.Error(codes.Aborted, "region code must be an int")
	}
	size, err := getStorageBytes(req.CapacityRange)
	if err != nil {
		return nil, status.Errorf(codes.OutOfRange, "invalid volume capacity range: %v", err)
	}

	volume, err := c.Driver.client.BlockStorage.Create(ctx, region, int(size), volName)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Check to see if volume is in active state
	volReady := false

	for i := 0; i < volumeStatusCheckRetries; i++ {
		time.Sleep(volumeStatusCheckInterval * time.Second)
		bs, err := c.Driver.client.BlockStorage.Get(ctx, volume.BlockStorageID)

		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		if bs.Status == "active" {
			volReady = true
			break
		}
	}

	if !volReady {
		return nil, status.Errorf(codes.Internal, "volume is not active after %v seconds", volumeStatusCheckRetries)
	}

	res := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volume.BlockStorageID,
			CapacityBytes: size,
			AccessibleTopology: []*csi.Topology{
				{
					Segments: map[string]string{
						"region": c.Driver.region,
					},
				},
			},
		},
	}

	return res, nil
}

func (c *VultrControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume VolumeID is missing")
	}

	err := c.Driver.client.BlockStorage.Delete(ctx, req.VolumeId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "cannot delete volume, %v", err.Error())
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (c *VultrControllerServer) ControllerPublishVolume(context.Context, *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	panic("implement me")
}

func (c *VultrControllerServer) ControllerUnpublishVolume(context.Context, *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	panic("implement me")
}

func (c *VultrControllerServer) ValidateVolumeCapabilities(context.Context, *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	panic("implement me")
}

func (c *VultrControllerServer) ListVolumes(context.Context, *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	panic("implement me")
}

func (c *VultrControllerServer) GetCapacity(context.Context, *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	panic("implement me")
}

func (c *VultrControllerServer) ControllerGetCapabilities(context.Context, *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	panic("implement me")
}

func (c *VultrControllerServer) CreateSnapshot(context.Context, *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	panic("implement me")
}

func (c *VultrControllerServer) DeleteSnapshot(context.Context, *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	panic("implement me")
}

func (c *VultrControllerServer) ListSnapshots(context.Context, *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	panic("implement me")
}

func (c *VultrControllerServer) ControllerExpandVolume(context.Context, *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	panic("implement me")
}

func isValidCapability(caps []*csi.VolumeCapability) bool {
	for _, capacity := range caps {
		if capacity == nil {
			return false
		}

		accessMode := capacity.GetAccessMode()
		if accessMode == nil {
			return false
		}

		if accessMode.GetMode() != supportedVolCapabilities.GetMode() {
			return false
		}

		accessType := capacity.GetAccessType()
		switch accessType.(type) {
		case *csi.VolumeCapability_Block:
		case *csi.VolumeCapability_Mount:
		default:
			return false
		}
	}
	return true
}

// getStorageBytes returns storage size in bytes
func getStorageBytes(capRange *csi.CapacityRange) (int64, error) {
	if capRange == nil {
		return defaultVolumeSizeInBytes, nil
	}

	capacity := capRange.GetRequiredBytes()
	return capacity, nil
}
