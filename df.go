package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type Mount struct {
	Device, Mountpoint string
	Total, Used, Avail uint64
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