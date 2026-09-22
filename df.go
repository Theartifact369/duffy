package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

type Mount struct {
	Device, Mountpoint string
	Total, Used, Avail uint64
	InoUsed, InoTotal  uint64
	Read, Write        float64 // bytes/s since the last refresh sample
	Mounted            bool
}

var pseudoFS = map[string]bool{
	"proc": true, "sysfs": true, "devtmpfs": true, "devpts": true, "tmpfs": true,
	"cgroup": true, "cgroup2": true, "securityfs": true, "debugfs": true,
	"mqueue": true, "hugetlbfs": true, "pstore": true, "fusectl": true,
	"configfs": true, "rpc_pipefs": true, "bpf": true, "tracefs": true,
	"autofs": true, "binfmt_misc": true, "efivarfs": true, "rootfs": true,
	"fuse": true, "fuse.gvfsd-fuse": true, "fuse.portal": true, "fuseblk": true,
}

// Mounts lists mounted filesystems from /proc/mounts, skipping pseudo
// filesystems unless all is set. Unreadable mounts are skipped. Unmounted
// block devices (from lsblk) are appended so they can be mounted.
func Mounts(all bool) []Mount {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []Mount
	mounted := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 {
			continue
		}
		dev, mp, fst := f[0], strings.ReplaceAll(f[1], `\040`, " "), f[2]
		if !all && pseudoFS[fst] {
			continue
		}
		var st syscall.Statfs_t
		if syscall.Statfs(mp, &st) != nil || st.Bsize <= 0 {
			continue
		}
		bs := uint64(st.Bsize)
		mounted[dev] = true
		out = append(out, Mount{
			Device: dev, Mountpoint: mp, Mounted: true,
			Total: st.Blocks * bs,
			Used:  (st.Blocks - st.Bfree) * bs,
			Avail: st.Bavail * bs,
			// inodes are tracked alongside bytes; a full-inode-but-empty
			// disk is the classic "can't create files" surprise
			InoTotal: st.Files,
			InoUsed:  st.Files - st.Ffree,
		})
	}
	if !all {
		out = append(out, unmountedDevices(mounted)...)
	}
	return out
}

// unmountedDevices lists block devices with a filesystem but no mountpoint,
// so the user can mount them. Uses lsblk's raw PATH,MOUNTPOINTS,FSTYPE.
func unmountedDevices(mounted map[string]bool) []Mount {
	out, err := exec.Command("lsblk", "-rno", "PATH,MOUNTPOINTS,FSTYPE").Output()
	if err != nil {
		return nil
	}
	return parseLSBLK(string(out), mounted)
}

// diskIO returns cumulative read/write bytes per block device name (bare,
// like "nvme0n1p2"), from /proc/diskstats. Fields: name, …, sectors read
// (5), …, sectors written (9). A sector is 512 bytes.
func diskIO() map[string][2]uint64 {
	m := map[string][2]uint64{}
	data, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 10 {
			continue
		}
		m[f[2]] = [2]uint64{atoi(f[5]) * 512, atoi(f[9]) * 512}
	}
	return m
}

// dmName maps block device names to their mapper alias it they have one
// (dm-0 -> root) by reading /sys/block/<name>/dm/name. Mounts show
// /dev/mapper/root, diskstats shows dm-0, so the detail box needs to join
// them. Lazy: only successive pairs are looked up, cost is one readdir.
func dmNames() map[string]string {
	al := map[string]string{}
	blocks, err := os.ReadDir("/sys/block")
	if err != nil {
		return al
	}
	for _, b := range blocks {
		name := b.Name()
		if data, err := os.ReadFile("/sys/block/" + name + "/dm/name"); err == nil {
			al[strings.TrimSpace(string(data))] = name
		}
	}
	return al
}

// attachIO fills each mount's Read/Write rates from the last two diskstats
// samples (refreshed every 2 s). The device is matched by basename; dm
// devices resolve their mapper alias first.
func attachIO(mounts []Mount, prev, cur map[string][2]uint64, dt float64) {
	dm := dmNames()
	for i := range mounts {
		base := filepath.Base(mounts[i].Device) // nvme0n1p2, dm-0, or "root" for /dev/mapper/root
		if name, ok := dm[base]; ok {
			base = name
		}
		p, okP := prev[base]
		c, okC := cur[base]
		if !okP || !okC || dt <= 0 || c[0] < p[0] || c[1] < p[1] {
			continue // first sample or counter reset
		}
		mounts[i].Read = float64(c[0]-p[0]) / dt
		mounts[i].Write = float64(c[1]-p[1]) / dt
	}
}

// atoi parses a uint64, returning 0 on garbage instead of panicking.
func atoi(s string) uint64 {
	var v uint64
	fmt.Sscanf(s, "%d", &v)
	return v
}

// parseLSBLK filters lsblk -rno PATH,MOUNTPOINTS,FSTYPE output down to
// unmountable devices: a filesystem, no mountpoint, not already mounted,
// and not a loop/zram/optical/removable oddity. lsblk omits empty
// mountpoints, so unmounted lines have 2 fields, mounted lines have 3.
func parseLSBLK(out string, mounted map[string]bool) []Mount {
	var devs []Mount
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || f[1] == "" {
			continue // mounted (3 fields) or whole disk (1 field)
		}
		dev := f[0]
		if mounted[dev] || !strings.HasPrefix(dev, "/dev/") {
			continue
		}
		base := strings.TrimPrefix(dev, "/dev/")
		if strings.Contains(base, "loop") || strings.Contains(base, "zram") ||
			strings.HasPrefix(base, "sr") || strings.HasPrefix(base, "fd") {
			continue
		}
		devs = append(devs, Mount{Device: dev, Mountpoint: dev})
	}
	return devs
}

// formatRate renders a byte-per-second rate: 12.3 M/s, 3.1 G/s, 512 B/s.
func formatRate(n float64) string {
	if n < 0 {
		n = 0
	}
	return formatSize(uint64(n)) + "/s"
}

// formatSize renders a byte count like duf: 121.0G, 1.8T, 512B.
func formatSize(n uint64) string {
	switch {
	case n >= 1<<50:
		return fmt.Sprintf("%.1fP", float64(n)/(1<<50))
	case n >= 1<<40:
		return fmt.Sprintf("%.1fT", float64(n)/(1<<40))
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
