package config

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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
	sysRootLimit = 0.5
	nvmeLimit    = 0.2 // nvme will be slow to read/write if usage exceeds 82%
)

type GpfsCache string

const (
	never       GpfsCache = "never"
	unspecified           = "unspecified"
	prefer                = "prefer"
)

func LocalCacheDir(volume string, preferGPFS GpfsCache) (localCache string, gpfsCache string) { // TODO: support multiple cache dirs?
	if preferGPFS == prefer {
		if gpfsCache = findGPFSCache(volume); gpfsCache != "" { // GPFS can be mounted/umounted dynamically, so probe every time
			klog.Infof("%q uses GPFS cache path: %q", volume, gpfsCache)
		}
	}
	// if strings.Contains(lastGpfsCache, icfsDir) {
	// 	return "memory", gpfsCache // force to use icfs, delete me later
	// }
	localCache = localCacheDir.Load().(string)
	freeLimit := nvmeLimit
	if inRootVolume(hostRoot + localCache) {
		freeLimit = sysRootLimit
	}
	pct, avail, ipct, _ := diskFreePct(hostRoot + localCache)
	if avail < 2*1024*1024*1024 {
		klog.Warningf("Cache dir %q is almost full, free pct=%.2f%%, avail=%s*, will use memory", localCache, pct*100, humanize.IBytes(avail))
		return "memory", gpfsCache
	}
	if (pct < freeLimit || ipct < freeLimit) && !cacheDirHasChunk(hostRoot+localCache) { // some volumes do not clean cache at exit
		klog.Warningf("Cache dir %q is almost full, free pct=%.2f%%*, ipct=%.2f%%, avail=%s, no chunk found, will use memory", localCache, pct*100, ipct*100, humanize.IBytes(avail))
		return "memory", gpfsCache
	}
	return localCacheDir.Load().(string), gpfsCache
}

func init() {
	localCacheDir.Store("/var/jfsCache")
	probeFinished := false

	go func() {
		defer func() {
			probeFinished = true
		}()
		updateLocalCache()
		if lastGpfsCache := findGPFSCache(""); lastGpfsCache != "" {
			klog.Infof("GPFS cache path: %q available", lastGpfsCache)
		}
	}()

	go func() {
		time.Sleep(time.Minute)
		if !probeFinished {
			klog.Warningf("NVMe probe doesnot finish in 1 minute, may be it hangs?")
		}
	}()

	go func() {
		for {
			time.Sleep(6 * time.Hour)
			updateLocalCache() // in case local disk fails
		}
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

func updateLocalCache() {
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
}

func findGPFSCache(volume string) string {
	resultChan := make(chan string, 1)
	probeFailed := false
	go func() { // Just in case GPFS hangs
		start := time.Now()
		resultChan <- doFindGPFSCache(volume)
		if probeFailed { // know how long it takes
			klog.Warningf("GPFS probe finally finished in %s", time.Since(start))
		} else {
			klog.Infof("GPFS probe finished in %s", time.Since(start))
		}
	}()

	timer := time.NewTimer(5 * time.Second) // Kubelet only waits 5 seconds(?)
	select {
	case result := <-resultChan:
		return result
	case <-timer.C:
		klog.Warningf("GPFS probe doesnot finish in 5s, may be it hangs?")
		probeFailed = true
		return ""
	}
}

func doFindGPFSCache(volume string) (cacheDir string) {
	// if strings.HasPrefix(hostname, "gpu-a910") {
	// 	klog.V(9).Infof("a910 in QB, skip")
	// 	return ""
	// }
	gpfsCacheDirs := []string{
		// "/gpfs/public-shared/fileset-groups/basemind-sys-jfs",
		// "/gpfs/public-shared/fileset-projects/mc-sys-jfs",
		// "/inspurfs/public-shared/fileset-projects/sys-jfs",
	}
	preference := map[string][]string{
		// "nextstep-project-jfs": {"/inspurfs/public-shared/fileset-projects/sys-jfs-large"},
		// "nextstep-du":          {"/inspurfs/public-shared/fileset-projects/sys-jfs-large"},
		// "step3-data": {"/gpfs/public-shared/fileset-groups/sys-jfs-pretrain2"},
	}
	gpfsCacheDirs = append(gpfsCacheDirs, preference[volume]...) // last one takes precedence
	for _, path := range gpfsCacheDirs {
		containerPath := hostRoot + path
		if st, err := os.Stat(containerPath); err != nil {
			if !os.IsNotExist(err) {
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
		cacheDir = path + "/jfsCache"
	}
	return
}

func inRootVolume(dir string) bool {
	dirDevice, _ := deviceOfPath(dir)
	rootDevice, _ := deviceOfPath(hostRoot)
	return dirDevice.Dev == rootDevice.Dev
}

func deviceOfPath(path string) (syscall.Stat_t, string) {
	for {
		stat, err := os.Stat(path)
		if err == nil {
			return *stat.Sys().(*syscall.Stat_t), path
		}
		if os.IsNotExist(err) {
			parent := filepath.Dir(path)
			if parent == path {
				// Reached root directory
				return syscall.Stat_t{}, ""
			}
			path = parent
		} else {
			klog.Warningf("Stat `%s`: %s", path, err.Error())
			return syscall.Stat_t{}, ""
		}
	}
}

func diskFreePct(path string) (float64, uint64, float64, error) {
	_, path = deviceOfPath(path) // avoid "no such file or directory"
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		klog.Warningf("Statfs `%s`: %s", path, err)
		return 0, 0, 0, err
	}
	if stat.Blocks == 0 || stat.Files == 0 {
		klog.Warningf("Statfs `%s`: zero blocks/files, stat=%+v", path, stat)
		return 0, 0, 0, nil
	}
	return float64(stat.Bavail) / float64(stat.Blocks), stat.Bavail * uint64(stat.Bsize), float64(stat.Ffree) / float64(stat.Files), nil
}

func cacheDirHasChunk(path string) bool {
	var total uint64
	start := time.Now()
	err := filepath.Walk(path, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			klog.Warningf("Error walking %s: %v", path, err)
			if os.IsNotExist(err) {
				err = nil
			}
			return err
		}
		if !info.IsDir() && strings.Count(filepath.Base(path), "_") == 2 { // maybe a cache file
			total += uint64(info.Size())
			if total > 100<<20 {
				return filepath.SkipAll
			}
		}
		if time.Since(start) > 10*time.Second {
			klog.Warningf("Cache dir %s has too many files, scaned %s, skipping", path, time.Since(start))
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return false
	}
	return total > 100<<20
}
