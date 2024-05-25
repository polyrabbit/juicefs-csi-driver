package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog"
)

var (
	hostRoot      = "/.host"
	localCacheDir atomic.Value
	inUT          = false
	CacheDirGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cache_dir_path",
		Help: "cache dir path on host",
	}, []string{"path"})
)

func LocalCacheDir() []string { // TODO: support multiple cache dirs?
	if path := findGPFSCache(); path != "" { // GPFS can be mounted/umounted dynamically, so probe every time
		gpfsCache := path + "/jfsCache"
		klog.Infof("Use GPFS cache path: %q", gpfsCache)
		return []string{gpfsCache}
	}
	return []string{localCacheDir.Load().(string)}
}

func init() {
	localCacheDir.Store("/var/jfsCache")
	probeFinished := false

	go func() {
		defer func() {
			probeFinished = true
		}()
		cmd := exec.Command("lsblk", "--json", "-o", "NAME,TYPE,SIZE,TRAN,MOUNTPOINT")
		output, err := cmd.Output()
		if err != nil {
			klog.Warningf("Error executing lsblk, output: %q, err: %v", output, err)
		} else {
			if mp := findNVMeMountpoint(output); mp != "" {
				localCacheDir.Store(mp + "/jfsCache")
				klog.Infof("Found NVMe mountpoint: %q", mp)
			} else {
				klog.Infof("NVMe mountpoint not found from lsblk output: %q", output)
			}
		}
		if path := findGPFSCache(); path != "" {
			klog.Infof("GPFS cache path: %q available", path)
		}
	}()

	go func() {
		time.Sleep(time.Minute)
		if !probeFinished {
			klog.Warningf("NVMe probe doesnot finish in 1 minute, may be it hangs?")
		}
		CacheDirGauge.WithLabelValues(LocalCacheDir()...).Set(1)
	}()
}

type Device struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Size       string   `json:"size"`
	Tran       string   `json:"tran"`
	Mountpoint string   `json:"mountpoint"`
	Children   []Device `json:"children"`
}

func (d *Device) ByteSize() uint64 {
	if d.Size == "" {
		return 0
	}
	bytes, err := humanize.ParseBytes(d.Size)
	if err != nil {
		klog.Warningf("Error parsing size: %q, err: %v", d.Size, err)
	}
	return bytes
}

func (d *Device) IsDir() bool {
	if inUT {
		return true
	}
	if d.Mountpoint == "" {
		return false
	}
	if st, err := os.Stat(d.Mountpoint); err != nil {
		klog.Warningf("Error stating mountpoint: %q, err: %v", d.Mountpoint, err)
		return false
	} else if !st.IsDir() {
		klog.Warningf("Mountpoint is not a directory: %q", d.Mountpoint)
		return false
	}
	return true
}

func (d *Device) StripSysRoot() string {
	return strings.TrimPrefix(d.Mountpoint, hostRoot)
}

func (d *Device) LargestMp() string {
	if d.Mountpoint != "" {
		return d.Mountpoint
	}
	var largestPart Device
	for _, device := range d.Children {
		if device.ByteSize() >= largestPart.ByteSize() && device.IsDir() {
			largestPart = device
		}
	}
	return largestPart.Mountpoint
}

type LsblkOutput struct {
	Blockdevices []Device `json:"blockdevices"`
}

func findNVMeMountpoint(output []byte) string {
	var lsblkOutput LsblkOutput
	if err := json.Unmarshal(output, &lsblkOutput); err != nil {
		klog.Warningf("Error parsing lsblk output: %v", err)
		return ""
	}

	devices := lsblkOutput.Blockdevices
	sort.Slice(devices, func(i, j int) bool { // Prefer last one
		return devices[i].LargestMp() < devices[j].LargestMp()
	})

	var candidate Device
	for _, device := range devices {
		if device.Tran != "nvme" {
			continue
		}
		// Prefer the last one, if capacity is the same
		if device.ByteSize() >= candidate.ByteSize() && device.IsDir() {
			candidate = device
		}
		for _, device = range device.Children { // Donot check tran for children
			if device.ByteSize() >= candidate.ByteSize() && device.IsDir() {
				candidate = device
			}
		}
	}
	return candidate.StripSysRoot()
}

func findGPFSCache() string {
	resultChan := make(chan string, 1)
	go func() { // Just in case GPFS hangs
		resultChan <- doFindGPFSCache()
	}()

	timer := time.NewTimer(30 * time.Second)
	select {
	case result := <-resultChan:
		return result
	case <-timer.C:
		klog.Warningf("GPFS probe doesnot finish in 30s, may be it hangs?")
		return ""
	}
}

func doFindGPFSCache() string {
	hostname, err := os.Hostname()
	if err != nil {
		klog.Warningf("Error getting hostname: %v", err)
		return ""
	}
	if !strings.HasPrefix(hostname, "gpu-") {
		klog.V(9).Infof("Hostname %q does not start with 'gpu-'", hostname)
		return ""
	}
	gpfsCacheDirs := map[string]string{
		"basemind":  "/gpfs/public-shared/fileset-groups/basemind-sys-jfs",
		"shaipower": "/inspurfs/public-shared/fileset-projects/sys-jfs",
	}
	for cluster, path := range gpfsCacheDirs {
		containerPath := hostRoot + path
		if st, err := os.Stat(containerPath); err != nil {
			if !os.IsNotExist(err) || strings.Contains(hostname, cluster) {
				klog.Warningf("Error stating path: %q, err: %v", containerPath, err)
			}
			continue
		} else if !st.IsDir() {
			klog.Warningf("Path is not a directory: %q", containerPath)
			continue
		}
		if inRootVolume(containerPath) {
			klog.Warningf("Path %q is in root volume", containerPath)
			continue
		}
		return path
	}
	return ""
}

func inRootVolume(dir string) bool {
	dstat, err := os.Stat(dir)
	if err != nil {
		klog.Warningf("Stat `%s`: %s", dir, err.Error())
		return false
	}
	rstat, err := os.Stat("/")
	if err != nil {
		klog.Warningf("Stat `/`: %s", err.Error())
		return false
	}
	return dstat.Sys().(*syscall.Stat_t).Dev == rstat.Sys().(*syscall.Stat_t).Dev
}
