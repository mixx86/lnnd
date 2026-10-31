package config

import (
	"bufio"
	"strings"
	"testing"
)

const sampleMountInfo = `
44 1 0:37 /root / rw shared:1 - btrfs /dev/mapper/luks-x rw
55 44 0:37 /home /home rw - btrfs /dev/mapper/luks-x rw
62 44 259:2 / /boot rw shared:84 - ext4 /dev/nvme0n1p2 rw
59 44 0:48 / /tmp rw shared:81 - tmpfs tmpfs rw
192 44 8:17 / /mnt/my\040disk rw - ext4 /dev/sdb1 rw
`

func TestMountSource(t *testing.T) {
	cases := []struct {
		path    string
		want    string
		wantErr bool
	}{
		{"/usr/bin/lnnd", "/dev/mapper/luks-x", false},
		{"/boot/lnnd", "/dev/nvme0n1p2", false},
		{"/bootx/lnnd", "/dev/mapper/luks-x", false},
		{"/mnt/my disk/lnnd", "/dev/sdb1", false},
		{"/tmp/lnnd", "", true},
	}
	for _, c := range cases {
		sc := bufio.NewScanner(strings.NewReader(sampleMountInfo))
		got, err := mountSource(sc, c.path)
		if (err != nil) != c.wantErr {
			t.Fatalf("%s: err = %v, wantErr %v", c.path, err, c.wantErr)
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.path, got, c.want)
		}
	}
}

func TestDiskDeviceFromEnv(t *testing.T) {
	t.Setenv("LNND_DISK_DEVICE", "/dev/sda")
	got, err := DiskDevice()
	if err != nil {
		t.Fatal(err)
	}
	if got != "sda" {
		t.Errorf("got %q, want %q", got, "sda")
	}
}
