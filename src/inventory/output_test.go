package inventory

import (
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift/assisted-installer-agent/src/config"
	"github.com/openshift/assisted-service/models"
)

func diskWithEligibility(name string, eligible bool) *models.Disk {
	disk := createFakeModelDisk(0)
	disk.Name = name
	disk.InstallationEligibility = models.DiskInstallationEligibility{Eligible: eligible}
	return disk
}

func diskWithSize(name string, sizeBytes int64) *models.Disk {
	disk := diskWithEligibility(name, true)
	disk.SizeBytes = sizeBytes
	return disk
}

func defaultInventoryConfig() config.InventoryConfig {
	return config.InventoryConfig{}
}

func inventoryWithDisks(disks ...*models.Disk) *models.Inventory {
	return &models.Inventory{
		Hostname: "test-host",
		Disks:    disks,
	}
}

func unmarshalInventory(data []byte) *models.Inventory {
	var inv models.Inventory
	Expect(json.Unmarshal(data, &inv)).To(Succeed())
	return &inv
}

var _ = Describe("CreateInventoryOutput", func() {
	It("returns nil for nil inventory", func() {
		out, err := CreateInventoryOutput(config.InventoryConfig{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeNil())
	})

	It("returns the full inventory when output max size is disabled (-1)", func() {
		in := inventoryWithDisks(diskWithEligibility("sda", true))
		cfg := config.InventoryConfig{OutputMaxSize: -1}

		out, err := CreateInventoryOutput(cfg, in)
		Expect(err).NotTo(HaveOccurred())

		result := unmarshalInventory(out)
		Expect(result.Hostname).To(Equal("test-host"))
		Expect(result.Disks).To(HaveLen(1))
		Expect(result.Truncation).To(BeNil())
	})

	It("returns the full inventory when output max size is disabled (0)", func() {
		in := inventoryWithDisks(diskWithEligibility("sda", true))
		cfg := config.InventoryConfig{}

		out, err := CreateInventoryOutput(cfg, in)
		Expect(err).NotTo(HaveOccurred())

		result := unmarshalInventory(out)
		Expect(result.Hostname).To(Equal("test-host"))
		Expect(result.Disks).To(HaveLen(1))
		Expect(result.Truncation).To(BeNil())
	})

	It("returns the full inventory when it fits within the max size", func() {
		in := inventoryWithDisks(diskWithEligibility("sda", true))
		full, err := json.Marshal(in)
		Expect(err).NotTo(HaveOccurred())

		out, err := CreateInventoryOutput(config.InventoryConfig{OutputMaxSize: len(full)}, in)
		Expect(err).NotTo(HaveOccurred())

		result := unmarshalInventory(out)
		Expect(result.Disks).To(HaveLen(1))
		Expect(result.Truncation).To(BeNil())
	})

	It("partially truncates by removing small disks", func() {
		const minSize = 100 * 1024 * 1024 * 1024
		in := inventoryWithDisks(
			diskWithSize("big", minSize),
			diskWithSize("small1", 512),
			diskWithSize("small2", 1024),
		)

		withAll, err := json.Marshal(in)
		Expect(err).NotTo(HaveOccurred())

		cfg := config.InventoryConfig{
			OutputMaxSize: len(withAll) - 1,
			DiskMinSize:   minSize,
		}
		out, err := CreateInventoryOutput(cfg, in)
		Expect(err).NotTo(HaveOccurred())

		result := unmarshalInventory(out)
		Expect(result.Disks).To(HaveLen(1))
		Expect(result.Disks[0].Name).To(Equal("big"))
		Expect(result.Truncation.Reasons).To(ContainElement(
			"2 disks were removed because they were smaller than the minimum size (107374182400 bytes)",
		))
	})

	It("partially truncates by removing ineligible disks", func() {
		in := inventoryWithDisks(
			diskWithEligibility("eligible", true),
			diskWithEligibility("ineligible", false),
		)

		withBoth, err := json.Marshal(in)
		Expect(err).NotTo(HaveOccurred())

		cfg := config.InventoryConfig{OutputMaxSize: len(withBoth) - 1}
		out, err := CreateInventoryOutput(cfg, in)
		Expect(err).NotTo(HaveOccurred())

		result := unmarshalInventory(out)
		Expect(result.Disks).To(HaveLen(1))
		Expect(result.Disks[0].Name).To(Equal("eligible"))
		Expect(result.Truncation).NotTo(BeNil())
		Expect(result.Truncation.Type).To(Equal(models.InventoryTruncationTypePartial))
		Expect(result.Truncation.Reasons).To(ContainElement("1 disk was removed because it was ineligible for installation"))
	})

	It("does not mutate the original inventory when truncating", func() {
		in := inventoryWithDisks(
			diskWithEligibility("eligible", true),
			diskWithEligibility("ineligible", false),
		)

		_, err := CreateInventoryOutput(config.InventoryConfig{OutputMaxSize: 1}, in)
		Expect(err).NotTo(HaveOccurred())
		Expect(in.Disks).To(HaveLen(2))
		Expect(in.Truncation).To(BeNil())
	})

	It("fully truncates when partial truncation is not enough", func() {
		in := inventoryWithDisks(diskWithEligibility("sda", true))

		cfg := config.InventoryConfig{OutputMaxSize: 1}
		out, err := CreateInventoryOutput(cfg, in)
		Expect(err).NotTo(HaveOccurred())

		result := unmarshalInventory(out)
		Expect(result.Disks).To(BeNil())
		Expect(result.Hostname).To(BeEmpty())
		Expect(result.Truncation).NotTo(BeNil())
		Expect(result.Truncation.Type).To(Equal(models.InventoryTruncationTypeFull))
		Expect(result.Truncation.Reasons).To(HaveLen(1))
		Expect(result.Truncation.Reasons[0]).To(ContainSubstring("Inventory size is too large"))
		Expect(result.Truncation.Reasons[0]).To(ContainSubstring("disks"))
	})
})

var _ = Describe("TruncateInventory", func() {
	It("returns nil for nil inventory", func() {
		out, err := TruncateInventory(defaultInventoryConfig(), nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeNil())
	})

	It("returns a clone without modifying the original when all disks are eligible", func() {
		in := inventoryWithDisks(diskWithEligibility("sda", true))

		out, err := TruncateInventory(defaultInventoryConfig(), in)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Disks).To(HaveLen(1))
		Expect(out.Truncation).To(BeNil())
		Expect(in.Disks).To(HaveLen(1))
		Expect(in.Truncation).To(BeNil())
	})

	It("removes ineligible disks on the clone and records the reason", func() {
		in := inventoryWithDisks(
			diskWithEligibility("sda", true),
			diskWithEligibility("sdb", false),
			diskWithEligibility("sdc", false),
		)

		out, err := TruncateInventory(defaultInventoryConfig(), in)
		Expect(err).NotTo(HaveOccurred())

		Expect(out.Disks).To(HaveLen(1))
		Expect(out.Disks[0].Name).To(Equal("sda"))
		Expect(out.Truncation.Reasons).To(ContainElement("2 disks were removed because they were ineligible for installation"))
		Expect(in.Disks).To(HaveLen(3))
	})

	It("treats disks without explicit eligibility as ineligible", func() {
		in := inventoryWithDisks(createFakeModelDisk(0))
		in.Disks[0].InstallationEligibility = models.DiskInstallationEligibility{}

		out, err := TruncateInventory(defaultInventoryConfig(), in)
		Expect(err).NotTo(HaveOccurred())

		Expect(out.Disks).To(BeEmpty())
		Expect(out.Truncation.Reasons).To(ContainElement("1 disk was removed because it was ineligible for installation"))
		Expect(in.Disks).To(HaveLen(1))
	})

	It("removes small disks when disk-min-size is set", func() {
		const minSize = 100 * 1024 * 1024 * 1024 // 100 GiB
		in := inventoryWithDisks(
			diskWithSize("big", minSize),
			diskWithSize("small", minSize-1),
		)

		cfg := config.InventoryConfig{DiskMinSize: minSize}
		out, err := TruncateInventory(cfg, in)
		Expect(err).NotTo(HaveOccurred())

		Expect(out.Disks).To(HaveLen(1))
		Expect(out.Disks[0].Name).To(Equal("big"))
		Expect(out.Truncation.Reasons).To(ContainElement(
			"1 disk was removed because it was smaller than the minimum size (107374182400 bytes)",
		))
		Expect(in.Disks).To(HaveLen(2))
	})

	It("keeps a disk whose size equals disk-min-size", func() {
		const minSize int64 = 1000
		in := inventoryWithDisks(diskWithSize("exact", minSize))

		cfg := config.InventoryConfig{DiskMinSize: minSize}
		out, err := TruncateInventory(cfg, in)
		Expect(err).NotTo(HaveOccurred())

		Expect(out.Disks).To(HaveLen(1))
		Expect(out.Truncation).To(BeNil())
	})

	It("does not filter by size when disk-min-size is disabled", func() {
		in := inventoryWithDisks(diskWithSize("tiny", 1))

		out, err := TruncateInventory(defaultInventoryConfig(), in)
		Expect(err).NotTo(HaveOccurred())

		Expect(out.Disks).To(HaveLen(1))
		Expect(out.Truncation).To(BeNil())
	})

	It("sets partial truncation metadata and removes ineligible disks", func() {
		in := inventoryWithDisks(
			diskWithEligibility("sda", true),
			diskWithEligibility("sdb", false),
		)

		out, err := TruncateInventory(defaultInventoryConfig(), in)
		Expect(err).NotTo(HaveOccurred())

		Expect(out.Truncation).NotTo(BeNil())
		Expect(out.Truncation.Type).To(Equal(models.InventoryTruncationTypePartial))
		Expect(out.Disks).To(HaveLen(1))
		Expect(out.Disks[0].Name).To(Equal("sda"))
		Expect(out.Truncation.Reasons).To(ContainElement("1 disk was removed because it was ineligible for installation"))
	})

	It("does not set truncation metadata when no disks are removed", func() {
		in := inventoryWithDisks(diskWithEligibility("sda", true))

		out, err := TruncateInventory(defaultInventoryConfig(), in)
		Expect(err).NotTo(HaveOccurred())

		Expect(out.Truncation).To(BeNil())
		Expect(out.Disks).To(HaveLen(1))
	})
})

var _ = Describe("cloneInventory", func() {
	It("returns an independent copy", func() {
		in := inventoryWithDisks(diskWithEligibility("sda", true))

		clone, err := cloneInventory(in)
		Expect(err).NotTo(HaveOccurred())

		clone.Hostname = "mutated"
		Expect(in.Hostname).To(Equal("test-host"))
	})
})
