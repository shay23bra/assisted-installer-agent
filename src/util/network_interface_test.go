package util

import (
	"errors"
	"io/fs"
	"net"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

type fakeFileInfo struct {
	name string
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() fs.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return true }
func (f fakeFileInfo) Sys() any           { return nil }

var _ = Describe("InfiniBand speed detection", func() {
	var (
		deps *MockIDependencies
		ni   *NetworkInterface
	)

	BeforeEach(func() {
		deps = &MockIDependencies{}
		deps.On("GetGhwChrootRoot").Return("/host").Maybe()
		ni = &NetworkInterface{
			netInterface: net.Interface{Name: "ib0"},
			dependencies: deps,
		}
	})

	AfterEach(func() {
		deps.AssertExpectations(GinkgoT())
	})

	It("falls back to InfiniBand sysfs when speed is -1", func() {
		deps.On("ReadFile", "/sys/class/net/ib0/speed").Return([]byte("-1\n"), nil)
		deps.On("LinkByName", "ib0").Return(&netlink.IPoIB{
			LinkAttrs: netlink.LinkAttrs{
				Name:      "ib0",
				EncapType: "infiniband",
			},
		}, nil)
		deps.On("ReadDir", "/sys/class/net/ib0/device/infiniband").Return(
			[]fs.FileInfo{fakeFileInfo{name: "mlx5_0"}}, nil,
		)
		deps.On("ReadFile", "/sys/class/net/ib0/dev_port").Return([]byte("0\n"), nil)
		deps.On("ReadFile", "/sys/class/infiniband/mlx5_0/ports/1/rate").Return([]byte("400 Gb/sec (4X NDR)\n"), nil)

		Expect(ni.SpeedMbps()).To(Equal(int64(400000)))
	})

	It("uses correct port number from dev_port", func() {
		deps.On("ReadFile", "/sys/class/net/ib0/speed").Return([]byte("-1\n"), nil)
		deps.On("LinkByName", "ib0").Return(&netlink.IPoIB{
			LinkAttrs: netlink.LinkAttrs{
				Name:      "ib0",
				EncapType: "infiniband",
			},
		}, nil)
		deps.On("ReadDir", "/sys/class/net/ib0/device/infiniband").Return(
			[]fs.FileInfo{fakeFileInfo{name: "mlx5_0"}}, nil,
		)
		deps.On("ReadFile", "/sys/class/net/ib0/dev_port").Return([]byte("1\n"), nil)
		deps.On("ReadFile", "/sys/class/infiniband/mlx5_0/ports/2/rate").Return([]byte("200 Gb/sec (4X NDR)\n"), nil)

		Expect(ni.SpeedMbps()).To(Equal(int64(200000)))
	})

	It("defaults to port 1 when dev_port is unreadable", func() {
		deps.On("ReadFile", "/sys/class/net/ib0/speed").Return([]byte("-1\n"), nil)
		deps.On("LinkByName", "ib0").Return(&netlink.IPoIB{
			LinkAttrs: netlink.LinkAttrs{
				Name:      "ib0",
				EncapType: "infiniband",
			},
		}, nil)
		deps.On("ReadDir", "/sys/class/net/ib0/device/infiniband").Return(
			[]fs.FileInfo{fakeFileInfo{name: "mlx5_0"}}, nil,
		)
		deps.On("ReadFile", "/sys/class/net/ib0/dev_port").Return(nil, errors.New("no such file"))
		deps.On("ReadFile", "/sys/class/infiniband/mlx5_0/ports/1/rate").Return([]byte("400 Gb/sec (4X NDR)\n"), nil)

		Expect(ni.SpeedMbps()).To(Equal(int64(400000)))
	})

	It("does not fall back for non-InfiniBand interfaces", func() {
		ni.netInterface.Name = "bond0"
		deps.On("ReadFile", "/sys/class/net/bond0/speed").Return([]byte("-1\n"), nil)
		deps.On("LinkByName", "bond0").Return(&netlink.Dummy{
			LinkAttrs: netlink.LinkAttrs{
				Name:      "bond0",
				EncapType: "ether",
			},
		}, nil)

		Expect(ni.SpeedMbps()).To(Equal(int64(-1)))
	})

	It("returns -1 when InfiniBand HCA directory is missing", func() {
		deps.On("ReadFile", "/sys/class/net/ib0/speed").Return([]byte("-1\n"), nil)
		deps.On("LinkByName", "ib0").Return(&netlink.IPoIB{
			LinkAttrs: netlink.LinkAttrs{
				Name:      "ib0",
				EncapType: "infiniband",
			},
		}, nil)
		deps.On("ReadDir", "/sys/class/net/ib0/device/infiniband").Return(nil, errors.New("no such directory"))

		Expect(ni.SpeedMbps()).To(Equal(int64(-1)))
	})

	It("returns -1 when rate file is unreadable", func() {
		deps.On("ReadFile", "/sys/class/net/ib0/speed").Return([]byte("-1\n"), nil)
		deps.On("LinkByName", "ib0").Return(&netlink.IPoIB{
			LinkAttrs: netlink.LinkAttrs{
				Name:      "ib0",
				EncapType: "infiniband",
			},
		}, nil)
		deps.On("ReadDir", "/sys/class/net/ib0/device/infiniband").Return(
			[]fs.FileInfo{fakeFileInfo{name: "mlx5_0"}}, nil,
		)
		deps.On("ReadFile", "/sys/class/net/ib0/dev_port").Return([]byte("0\n"), nil)
		deps.On("ReadFile", "/sys/class/infiniband/mlx5_0/ports/1/rate").Return(nil, errors.New("permission denied"))

		Expect(ni.SpeedMbps()).To(Equal(int64(-1)))
	})

	It("returns -1 when rate format is unexpected", func() {
		deps.On("ReadFile", "/sys/class/net/ib0/speed").Return([]byte("-1\n"), nil)
		deps.On("LinkByName", "ib0").Return(&netlink.IPoIB{
			LinkAttrs: netlink.LinkAttrs{
				Name:      "ib0",
				EncapType: "infiniband",
			},
		}, nil)
		deps.On("ReadDir", "/sys/class/net/ib0/device/infiniband").Return(
			[]fs.FileInfo{fakeFileInfo{name: "mlx5_0"}}, nil,
		)
		deps.On("ReadFile", "/sys/class/net/ib0/dev_port").Return([]byte("0\n"), nil)
		deps.On("ReadFile", "/sys/class/infiniband/mlx5_0/ports/1/rate").Return([]byte("unknown\n"), nil)

		Expect(ni.SpeedMbps()).To(Equal(int64(-1)))
	})

	It("does not attempt fallback for positive speed", func() {
		deps.On("ReadFile", "/sys/class/net/ib0/speed").Return([]byte("1000\n"), nil)

		Expect(ni.SpeedMbps()).To(Equal(int64(1000)))
	})
})
