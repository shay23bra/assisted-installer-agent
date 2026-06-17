package inventory

import (
	"encoding/json"
	"fmt"

	"github.com/openshift/assisted-installer-agent/src/config"
	"github.com/openshift/assisted-service/models"
	"github.com/sirupsen/logrus"
)

// CreateInventoryOutput serializes the inventory to JSON, optionally truncating it when
// OutputMaxSize is exceeded.
//
// OutputMaxSize is a best-effort limit: adding truncation metadata (type, reasons) can push
// the payload back over the limit, including after a full truncation. Callers should treat
// the limit as a target to reduce payload size, not a hard guarantee.
func CreateInventoryOutput(inventoryConfig config.InventoryConfig, in *models.Inventory) ([]byte, error) {
	if in == nil {
		return nil, nil
	}

	ret, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal inventory: %w", err)
	}

	// If the output max size is not set or the inventory is smaller than the max size, return the inventory as is
	if inventoryConfig.OutputMaxSize <= 0 || len(ret) <= inventoryConfig.OutputMaxSize {
		return ret, nil
	}

	originalSize := len(ret)
	logrus.Warnf("Inventory size is too large (%d bytes), it will be partially truncated", originalSize)

	partial, err := TruncateInventory(inventoryConfig, in)
	if err != nil {
		return nil, fmt.Errorf("failed to truncate inventory: %w", err)
	}

	ret, err = json.Marshal(partial)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal partially truncated inventory: %w", err)
	}

	// Truncation was successful, return the inventory as is
	if len(ret) <= inventoryConfig.OutputMaxSize {
		return ret, nil
	}

	logrus.Warnf("Inventory size is still too large (%d bytes), it will be fully truncated", len(ret))

	empty := &models.Inventory{
		Truncation: &models.InventoryTruncation{
			Type: models.InventoryTruncationTypeFull,
			Reasons: []string{fmt.Sprintf(
				"Inventory size is too large (%d bytes), it contains %d disks, %d gpus, %d interfaces and %d routes",
				originalSize, len(in.Disks), len(in.Gpus), len(in.Interfaces), len(in.Routes),
			)},
		},
	}

	ret, err = json.Marshal(empty)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal fully truncated inventory: %w", err)
	}

	return ret, nil
}

func TruncateInventory(inventoryConfig config.InventoryConfig, in *models.Inventory) (*models.Inventory, error) {
	if in == nil {
		return nil, nil
	}

	clone, err := cloneInventory(in)
	if err != nil {
		return nil, fmt.Errorf("failed to clone inventory: %w", err)
	}

	removeIneligibleDisks(clone)
	removeSmallDisks(inventoryConfig, clone)

	return clone, nil
}

func removeIneligibleDisks(in *models.Inventory) {
	if in == nil {
		return
	}

	disks := make([]*models.Disk, 0)
	ineligibleDisks := 0

	for _, disk := range in.Disks {
		if !disk.InstallationEligibility.Eligible {
			ineligibleDisks++

			continue
		}

		disks = append(disks, disk)
	}

	switch ineligibleDisks {
	case 0:
		return
	case 1:
		addTruncationReason(in, "1 disk was removed because it was ineligible for installation")
	default:
		addTruncationReason(in, fmt.Sprintf("%d disks were removed because they were ineligible for installation", ineligibleDisks))
	}

	in.Disks = disks
}

// removeSmallDisks drops disks below DiskMinSize before serialization. This approximates
// assisted-service installation eligibility (which depends on host role) without the agent
// needing cluster context. Service should pass a conservative minimum for the deployment.
func removeSmallDisks(inventoryConfig config.InventoryConfig, in *models.Inventory) {
	if in == nil || inventoryConfig.DiskMinSize <= 0 {
		return
	}

	disks := make([]*models.Disk, 0)
	smallDisks := 0

	for _, disk := range in.Disks {
		if disk.SizeBytes < inventoryConfig.DiskMinSize {
			smallDisks++

			continue
		}

		disks = append(disks, disk)
	}

	switch smallDisks {
	case 0:
		return
	case 1:
		addTruncationReason(in, fmt.Sprintf("1 disk was removed because it was smaller than the minimum size (%d bytes)", inventoryConfig.DiskMinSize))
	default:
		addTruncationReason(in, fmt.Sprintf("%d disks were removed because they were smaller than the minimum size (%d bytes)", smallDisks, inventoryConfig.DiskMinSize))
	}

	in.Disks = disks
}

// cloneInventory deep-copies an inventory via JSON marshal/unmarshal so truncation does not
// mutate the original.
func cloneInventory(in *models.Inventory) (*models.Inventory, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal inventory: %w", err)
	}

	out := &models.Inventory{}
	if err := json.Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inventory: %w", err)
	}

	return out, nil
}

func addTruncationReason(in *models.Inventory, reason string) {
	if in == nil {
		return
	}

	if in.Truncation == nil {
		in.Truncation = &models.InventoryTruncation{
			Type:    models.InventoryTruncationTypePartial,
			Reasons: make([]string, 0),
		}
	}

	in.Truncation.Reasons = append(in.Truncation.Reasons, reason)
}
