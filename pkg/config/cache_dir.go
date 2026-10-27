package config

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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
	if localCache == "memory" {
		return localCache, gpfsCache
	}
	if inRootVolume(hostRoot + localCache) { // NVMe may be unmounted after the last probe
		klog.Warningf("Cache dir %q is on system disk, will use memory", localCache)
		return "memory", gpfsCache
	}
	pct, avail, ipct, err := diskFreePct(hostRoot + localCache)
	if err != nil { // e.g. EIO on a failing disk
		klog.Warningf("Cache dir %q is unavailable: %v, will use memory", localCache, err)
		return "memory", gpfsCache
	}
	if avail < 2*1024*1024*1024 {
		klog.Warningf("Cache dir %q is almost full, free pct=%.2f%%, avail=%s*, will use memory", localCache, pct*100, humanize.IBytes(avail))
		return "memory", gpfsCache
	}
	if (pct < nvmeLimit || ipct < nvmeLimit) && !cacheDirHasChunk(hostRoot+localCache) { // some volumes do not clean cache at exit
		klog.Warningf("Cache dir %q is almost full, free pct=%.2f%%*, ipct=%.2f%%, avail=%s, no chunk found, will use memory", localCache, pct*100, ipct*100, humanize.IBytes(avail))
		return "memory", gpfsCache
	}
	return localCache, gpfsCache
}

func init() {
	localCacheDir.Store("memory")
	probeFinished := false

	go func() {
		defer func() {
			probeFinished = true
		}()
		updateLocalCache()
		// if lastGpfsCache := findGPFSCache(""); lastGpfsCache != "" {
		// 	klog.Infof("GPFS cache path: %q available", lastGpfsCache)
		// }
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
	if d.Mountpoint == "" {
		return false
	}
	if inUT {
		return true
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

// OnSystemDisk compares st_dev since lsblk may report any bind mount of the system disk as mountpoint.
// Recurse because root may live in an LV under a partition.
func (d *Device) OnSystemDisk() bool {
	if inUT {
		return false
	}
	if d.Mountpoint != "" && inRootVolume(d.Mountpoint) {
		return true
	}
	for _, c := range d.Children {
		if c.OnSystemDisk() {
			return true
		}
	}
	return false
}

func (d *Device) StripSysRoot() string {
	return strings.TrimPrefix(d.Mountpoint, hostRoot)
}

type LsblkOutput struct {
	Blockdevices []Device `json:"blockdevices"`
}

func findNVMeMountpoint(output []byte) string {
	hostname, _ := os.Hostname()
	return findNVMeMountpointForHost(output, hostname)
}

func findNVMeMountpointForHost(output []byte, hostname string) string {
	var lsblkOutput LsblkOutput
	if err := json.Unmarshal(output, &lsblkOutput); err != nil {
		klog.Warningf("Error parsing lsblk output: %v", err)
		return ""
	}

	var candidate Device
	var visit func(*Device)
	visit = func(device *Device) {
		if device.IsDir() && preferCacheDirCandidate(hostname, candidate, *device) {
			candidate = *device
		}
		for i := range device.Children {
			visit(&device.Children[i])
		}
	}
	for i := range lsblkOutput.Blockdevices {
		device := &lsblkOutput.Blockdevices[i]
		if device.Tran != "nvme" || device.OnSystemDisk() {
			continue
		}
		visit(device)
	}
	return candidate.StripSysRoot()
}

func preferCacheDirCandidate(hostname string, cur, next Device) bool {
	if cur.Mountpoint == "" {
		return true
	}

	curMp, nextMp := cur.StripSysRoot(), next.StripSysRoot()
	curAvoid := avoidCacheDirOnHost(hostname, curMp)
	nextAvoid := avoidCacheDirOnHost(hostname, nextMp)
	if curAvoid && !nextAvoid {
		klog.Infof("Prefer cache dir %q over avoided cache dir %q on host %q", nextMp, curMp, hostname)
		return true
	}
	if !curAvoid && nextAvoid {
		klog.Infof("Skip avoided cache dir %q on host %q", nextMp, hostname)
		return false
	}

	if curShare, nextShare := curMp == "/mnt/share", nextMp == "/mnt/share"; curShare != nextShare {
		return nextShare // always prefer /mnt/share regardless of size
	}
	if next.ByteSize() != cur.ByteSize() {
		return next.ByteSize() > cur.ByteSize()
	}
	return nextMp > curMp // Prefer the last one, if capacity is the same
}

func avoidCacheDirOnHost(hostname, mountpoint string) bool {
	switch mountpoint {
	case "/mnt/nvme1n1":
		return strings.Contains(hostname, "gpu-h200") ||
			strings.Contains(hostname, "gpu-h100") ||
			strings.Contains(hostname, "gpu-h800")
	case "/mnt/nvme2n1":
		return strings.Contains(hostname, "gpu-h100") ||
			strings.Contains(hostname, "gpu-h800") ||
			strings.Contains(hostname, "gpu-b300") ||
			strings.Contains(hostname, "mc-gpu-a800")
	}
	return false
}

func updateLocalCache() {
	output, err := exec.Command("lsblk", "--json", "-o", "NAME,TYPE,SIZE,TRAN,MOUNTPOINT").Output()
	if err != nil {
		klog.Warningf("Error executing lsblk, output: %q, err: %v", output, err)
		return // keep the last probed value
	}
	cacheDir := "memory"
	if mp := findNVMeMountpoint(output); mp != "" {
		cacheDir = mp + "/jfsCache"
		klog.Infof("Found NVMe mountpoint: %q", mp)
	} else {
		klog.Infof("NVMe mountpoint not found from lsblk output: %q", output)
	}
	localCacheDir.Store(cacheDir)
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
	if _, existing := deviceOfPath(path); existing != "" {
		path = existing // walk up to an existing dir; on stat error keep path so Statfs reports the real errno
	}
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
