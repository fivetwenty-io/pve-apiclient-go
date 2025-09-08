package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/fivetwenty-io/pve-apiclient-go/internal/constants"
	"github.com/fivetwenty-io/pve-apiclient-go/pkg/client"
	pveerr "github.com/fivetwenty-io/pve-apiclient-go/pkg/errors"
)

var errSizeGiBPositive = errors.New("sizeGiB must be > 0")

// Service defines storage-related helpers.
type Service interface {
	CreateVolume(ctx context.Context, node, storage string, sizeGiB int, format string, vmid int, name string) (string, error)
	DeleteVolume(ctx context.Context, node, storage, volume string) error
	Exists(ctx context.Context, node, storage, volume string) (bool, error)
}

type service struct{ c client.Client }

// New returns a new storage service.
func New(c client.Client) Service { return &service{c: c} } //nolint:ireturn // Factory function pattern

func (s *service) CreateVolume(ctx context.Context, node, storage string, sizeGiB int, format string, vmid int, name string) (string, error) {
	if sizeGiB <= 0 {
		return "", errSizeGiBPositive
	}
	// PVE expects size in bytes
	sizeBytes := int64(sizeGiB) * constants.BytesPerGB
	params := map[string]interface{}{
		"size":     sizeBytes,
		"format":   format,
		"vmid":     vmid,
		"filename": name,
		"content":  "images",
	}

	data, err := s.c.PostCtx(ctx, fmt.Sprintf("/nodes/%s/storage/%s/content", node, storage), params)
	if err != nil {
		return "", fmt.Errorf("failed to create volume on storage %q node %q: %w", storage, node, err)
	}

	if m, ok := data.(map[string]interface{}); ok {
		if v, ok := m["volid"].(string); ok {
			return v, nil
		}
	}

	if vol, ok := data.(string); ok {
		return vol, nil
	}

	return "", nil
}
func (s *service) DeleteVolume(ctx context.Context, node, storage, volume string) error {
	_, err := s.c.DeleteCtx(ctx, fmt.Sprintf("/nodes/%s/storage/%s/content/%s", node, storage, volume), nil)
	if err == nil {
		return nil
	}

	if pveerr.IsAPIError(err) {
		var ae *pveerr.APIError
		if errors.As(err, &ae) && ae.IsNotFound() {
			return nil
		}
	}

	return fmt.Errorf("failed to delete volume %q from storage %q on node %q: %w", volume, storage, node, err)
}
func (s *service) Exists(ctx context.Context, node, storage, volume string) (bool, error) {
	_, err := s.c.GetCtx(ctx, fmt.Sprintf("/nodes/%s/storage/%s/content/%s", node, storage, volume), nil)
	if err == nil {
		return true, nil
	}

	if pveerr.IsAPIError(err) {
		var ae *pveerr.APIError
		if errors.As(err, &ae) && ae.IsNotFound() {
			return false, nil
		}
	}

	return false, fmt.Errorf("failed to check if volume %q exists on storage %q node %q: %w", volume, storage, node, err)
}
