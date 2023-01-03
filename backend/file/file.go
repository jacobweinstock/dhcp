// Package file watches a file for changes and updates the in memory DHCP data.
package file

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/ghodss/yaml"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/dhcp/data"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"inet.af/netaddr"
)

const tracerName = "github.com/tinkerbell/dhcp"

// Errors used by the file watcher.
var (
	// errFileFormat is returned when the file is not in the correct format, e.g. not valid YAML.
	errFileFormat     = fmt.Errorf("invalid file format")
	errRecordNotFound = fmt.Errorf("record not found")
	errParseIP        = fmt.Errorf("failed to parse IP from File")
	errParseSubnet    = fmt.Errorf("failed to parse subnet mask from File")
	errParseURL       = fmt.Errorf("failed to parse URL")
)

// netboot is the structure for the data expected in a file.
type netboot struct {
	AllowPXE      bool   `yaml:"allowPxe"`      // If true, the client will be provided netboot options in the DHCP offer/ack.
	IPXEScriptURL string `yaml:"ipxeScriptUrl"` // Overrides default value of that is passed into DHCP on startup.
	VLAN          string `yaml:"vlan"`          // DHCP option 43.116. Used to create VLAN interfaces in iPXE.
}

// dhcp is the structure for the data expected in a file.
type dhcp struct {
	MACAddress       net.HardwareAddr // The MAC address of the client.
	IPAddress        string           `yaml:"ipAddress"`        // yiaddr DHCP header.
	SubnetMask       string           `yaml:"subnetMask"`       // DHCP option 1.
	DefaultGateway   string           `yaml:"defaultGateway"`   // DHCP option 3.
	NameServers      []string         `yaml:"nameServers"`      // DHCP option 6.
	Hostname         string           `yaml:"hostname"`         // DHCP option 12.
	DomainName       string           `yaml:"domainName"`       // DHCP option 15.
	BroadcastAddress string           `yaml:"broadcastAddress"` // DHCP option 28.
	NTPServers       []string         `yaml:"ntpServers"`       // DHCP option 42.
	LeaseTime        int              `yaml:"leaseTime"`        // DHCP option 51.
	DomainSearch     []string         `yaml:"domainSearch"`     // DHCP option 119.
	Netboot          netboot          `yaml:"netboot"`
}

// Watcher represents the backend for watching a file for changes and updating the in memory DHCP data.
type Watcher struct {
	fileMu sync.RWMutex // protects FilePath for reads

	// FilePath is the path to the file to watch.
	FilePath string

	// Log is the logger to be used in the File backend.
	Log     logr.Logger
	dataMu  sync.RWMutex // protects data
	data    []byte       // data from file
	watcher *fsnotify.Watcher
}

// NewWatcher creates a new file watcher.
func NewWatcher(l logr.Logger, f string) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := watcher.Add(path.Dir(f)); err != nil {
		return nil, err
	}

	w := &Watcher{
		FilePath: path.Clean(f),
		watcher:  watcher,
		Log:      l,
	}

	w.fileMu.RLock()
	w.data, err = os.ReadFile(path.Clean(f))
	w.fileMu.RUnlock()
	if err != nil {
		return nil, err
	}

	return w, nil
}

// Read is the implementation of the Backend interface.
// It reads a given file from the in memory data (w.data).
func (w *Watcher) Read(ctx context.Context, mac net.HardwareAddr) (*data.DHCP, *data.Netboot, error) {
	tracer := otel.Tracer(tracerName)
	_, span := tracer.Start(ctx, "backend.file.Read")
	defer span.End()

	// get data from file, translate it, then pass it into setDHCPOpts and setNetworkBootOpts
	w.dataMu.RLock()
	d := w.data
	w.dataMu.RUnlock()
	r := make(map[string]dhcp)
	if err := yaml.Unmarshal(d, &r); err != nil {
		err := fmt.Errorf("%v: %w", err, errFileFormat)
		w.Log.Error(err, "failed to unmarshal file data")
		span.SetStatus(codes.Error, err.Error())

		return nil, nil, err
	}
	for k, v := range r {
		if strings.EqualFold(k, mac.String()) {
			// found a record for this mac address
			v.MACAddress = mac
			d, n, err := w.translate(v)
			if err != nil {
				span.SetStatus(codes.Error, err.Error())

				return nil, nil, err
			}
			span.SetAttributes(d.EncodeToAttributes()...)
			span.SetAttributes(n.EncodeToAttributes()...)
			span.SetStatus(codes.Ok, "")

			return d, n, nil
		}
	}

	err := fmt.Errorf("%w: %s", errRecordNotFound, mac.String())
	span.SetStatus(codes.Error, err.Error())

	return nil, nil, err
}

// Name returns the name of the backend.
func (w *Watcher) Name() string {
	return "file"
}

// Start starts watching a file for changes and updates the in memory data (w.data) on changes.
// Start is a blocking method. Use a context cancellation to exit.
func (w *Watcher) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			w.Log.Info("stopping watcher")
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				continue
			}
			if event.Name == w.FilePath && event.Op == fsnotify.Write {
				w.Log.Info("file changed, updating cache")
				w.fileMu.RLock()
				d, err := os.ReadFile(w.FilePath)
				w.fileMu.RUnlock()
				if err != nil {
					w.Log.Error(err, "failed to read file", "file", w.FilePath)
					continue
				}
				w.dataMu.Lock()
				w.data = d
				w.dataMu.Unlock()
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				continue
			}
			w.Log.Info("error watching file", "err", err)
		}
	}
}

// translate converts the data from the file into a data.DHCP and data.Netboot structs.
func (w *Watcher) translate(r dhcp) (*data.DHCP, *data.Netboot, error) {
	d := new(data.DHCP)
	n := new(data.Netboot)

	d.MACAddress = r.MACAddress
	// ip address, required
	ip, err := netaddr.ParseIP(r.IPAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("%v: %w", err, errParseIP)
	}
	d.IPAddress = ip

	// subnet mask, required
	mask := net.ParseIP(r.SubnetMask)
	if mask == nil {
		return nil, nil, fmt.Errorf("%v: %w", err, errParseSubnet)
	}

	d.SubnetMask = net.IPv4Mask(mask[12], mask[13], mask[14], mask[15])

	// default gateway, optional
	if dg, err := netaddr.ParseIP(r.DefaultGateway); err != nil {
		w.Log.Info("failed to parse default gateway", "defaultGateway", r.DefaultGateway, "err", err)
	} else {
		d.DefaultGateway = dg
	}

	// name servers, optional
	for _, s := range r.NameServers {
		ip := net.ParseIP(s)
		if ip == nil {
			w.Log.Info("failed to parse name server", "nameServer", s)
			break
		}
		d.NameServers = append(d.NameServers, ip)
	}

	// hostname, optional
	d.Hostname = r.Hostname

	// domain name, optional
	d.DomainName = r.DomainName

	// broadcast address, optional
	if ba, err := netaddr.ParseIP(r.BroadcastAddress); err != nil {
		w.Log.Info("failed to parse broadcast address", "broadcastAddress", r.BroadcastAddress, "err", err)
	} else {
		d.BroadcastAddress = ba
	}

	// ntp servers, optional
	for _, s := range r.NTPServers {
		ip := net.ParseIP(s)
		if ip == nil {
			w.Log.Info("failed to parse ntp server", "ntpServer", s)
			break
		}
		d.NTPServers = append(d.NTPServers, ip)
	}

	// lease time
	d.LeaseTime = uint32(r.LeaseTime)

	// domain search
	d.DomainSearch = r.DomainSearch

	// allow machine to netboot
	n.AllowNetboot = r.Netboot.AllowPXE

	// ipxe script url is optional but if provided, it must be a valid url
	if r.Netboot.IPXEScriptURL != "" {
		u, err := url.Parse(r.Netboot.IPXEScriptURL)
		if err != nil {
			return nil, nil, fmt.Errorf("%v: %w", err, errParseURL)
		}
		n.IPXEScriptURL = u
	}

	n.VLAN = r.Netboot.VLAN

	return d, n, nil
}
