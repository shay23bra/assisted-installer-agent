package util

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

//go:generate mockery --name Interface --inpackage
type Interface interface {
	MTU() int
	Name() string
	HardwareAddr() net.HardwareAddr
	Flags() net.Flags
	Addrs() ([]net.Addr, error)
	IsPhysical() bool
	IsBonding() bool
	IsVlan() bool
	SpeedMbps() int64
	Type() (string, error)
}

type NetworkInterface struct {
	netInterface net.Interface
	dependencies IDependencies
}

func (n *NetworkInterface) MTU() int {
	return n.netInterface.MTU
}

func (n *NetworkInterface) Name() string {
	return n.netInterface.Name
}

func (n *NetworkInterface) HardwareAddr() net.HardwareAddr {
	return n.netInterface.HardwareAddr
}

func (n *NetworkInterface) Flags() net.Flags {
	return n.netInterface.Flags
}

func (n *NetworkInterface) Addrs() ([]net.Addr, error) {
	return n.netInterface.Addrs()
}

func (n *NetworkInterface) Type() (string, error) {
	if n.IsPhysical() {
		return "physical", nil
	}
	link, err := n.dependencies.LinkByName(n.netInterface.Name)
	if err != nil {
		return "", err
	}
	return link.Type(), nil
}

func (n *NetworkInterface) IsPhysical() bool {
	evaledPath, err := n.dependencies.EvalSymlinks(fmt.Sprintf("/sys/class/net/%s", n.netInterface.Name))
	if err != nil {
		logrus.WithError(err).Warnf("Could not determine if interface %s is physical", n.netInterface.Name)
		return true
	}
	return !strings.Contains(evaledPath, "/virtual/")
}

func (n *NetworkInterface) IsBonding() bool {
	link, err := n.dependencies.LinkByName(n.netInterface.Name)
	if err != nil {
		return false
	}
	return link.Type() == "bond"
}

func (n *NetworkInterface) IsVlan() bool {
	link, err := n.dependencies.LinkByName(n.netInterface.Name)
	if err != nil {
		return false
	}
	return link.Type() == "vlan"
}

func (n *NetworkInterface) isInfiniBand() bool {
	link, err := n.dependencies.LinkByName(n.netInterface.Name)
	if err != nil {
		logrus.WithError(err).Warnf("Could not get link for %s to check InfiniBand encap type", n.netInterface.Name)
		return false
	}
	encapType := link.Attrs().EncapType
	logrus.Debugf("Interface %s has encap type %q", n.netInterface.Name, encapType)
	return encapType == "infiniband"
}

func (n *NetworkInterface) infinibandSpeedMbps() int64 {
	ibDir := fmt.Sprintf("/sys/class/net/%s/device/infiniband", n.Name())
	entries, err := n.dependencies.ReadDir(ibDir)
	if err != nil || len(entries) == 0 {
		logrus.WithError(err).Warnf("Could not find InfiniBand HCA for %s", n.Name())
		return -1
	}
	hca := entries[0].Name()

	portBytes, err := n.dependencies.ReadFile(fmt.Sprintf("/sys/class/net/%s/dev_port", n.Name()))
	port := 1
	if err != nil {
		logrus.WithError(err).Debugf("Could not read dev_port for %s, defaulting to port 1", n.Name())
	} else if parsed, parseErr := strconv.Atoi(strings.TrimSpace(string(portBytes))); parseErr != nil {
		logrus.WithError(parseErr).Debugf("Could not parse dev_port for %s: %q, defaulting to port 1", n.Name(), strings.TrimSpace(string(portBytes)))
	} else {
		port = parsed + 1
	}

	rateFile := fmt.Sprintf("/sys/class/infiniband/%s/ports/%d/rate", hca, port)
	rateBytes, err := n.dependencies.ReadFile(rateFile)
	if err != nil {
		logrus.WithError(err).Warnf("Could not read InfiniBand rate for %s", n.Name())
		return -1
	}

	rateStr := strings.TrimSpace(string(rateBytes))
	parts := strings.Fields(rateStr)
	if len(parts) < 2 {
		logrus.Warnf("Unexpected InfiniBand rate format for %s: %q", n.Name(), rateStr)
		return -1
	}

	speed, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		logrus.WithError(err).Warnf("Could not parse InfiniBand rate for %s: %q", n.Name(), rateStr)
		return -1
	}

	speedMbps := int64(speed * 1000)
	logrus.Infof("InfiniBand interface %s: HCA=%s port=%d rate=%q speedMbps=%d", n.Name(), hca, port, rateStr, speedMbps)
	return speedMbps
}

func (n *NetworkInterface) SpeedMbps() int64 {
	b, err := n.dependencies.ReadFile(fmt.Sprintf("/sys/class/net/%s/speed", n.Name()))
	if err != nil {
		logrus.WithError(err).Warnf("Could not read %s speed", n.Name())
		return 0
	}
	ret, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 32)
	if err != nil {
		logrus.WithError(err).Warnf("Could not parse %s speed", n.Name())
	}
	// InfiniBand NICs report -1 via standard sysfs; try the InfiniBand-specific path
	if ret <= 0 && n.isInfiniBand() {
		return n.infinibandSpeedMbps()
	}
	return ret
}
