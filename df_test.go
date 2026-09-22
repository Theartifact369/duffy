package main

import (
	"reflect"
	"testing"
)

func TestParseLSBLK(t *testing.T) {
	out := "/dev/nvme0n1p2  /  ext4\n" +
		"/dev/nvme0n1p3    ext4\n" +
		"/dev/sdb1    vfat\n" +
		"/dev/sr0    iso9660\n" +
		"/dev/loop0    squashfs\n" +
		"/dev/zram0    swap\n" +
		"/dev/nvme0n1    \n" // whole disk, no fs
	got := parseLSBLK(out, map[string]bool{"/dev/nvme0n1p2": true})
	want := []Mount{
		{Device: "/dev/nvme0n1p3", Mountpoint: "/dev/nvme0n1p3"},
		{Device: "/dev/sdb1", Mountpoint: "/dev/sdb1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLSBLK = %+v, want %+v", got, want)
	}
}