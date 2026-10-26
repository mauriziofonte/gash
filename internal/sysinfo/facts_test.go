// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/platform"
)

// factsTree is a Linux host: /proc with a second disk, a bind mount of
// it, /boot, a read-only disk, a tmpfs and a Docker overlay.
var factsTree = tree{
	"etc/os-release": "PRETTY_NAME=\"Debian GNU/Linux 12 (bookworm)\"\n",
	"proc/uptime":    "93600.50 180000.00\n",
	"proc/loadavg":   "0.52 0.48 1.05 2/345 6789\n",
	"proc/cpuinfo":   "processor\t: 0\nmodel name\t: Intel(R) Xeon(R) CPU E5-2690 v4 @ 2.60GHz\n\nprocessor\t: 1\nmodel name\t: other\n",
	"proc/meminfo":   "MemTotal:       16384000 kB\nMemFree:         1024000 kB\nMemAvailable:    8192000 kB\n",
	"proc/self/mounts": "/dev/sda1 / ext4 rw,relatime 0 0\n" +
		"/dev/sdb1 /srv/data\\040disk xfs rw 0 0\n" +
		"/dev/sdb1 /mnt/bind xfs rw 0 0\n" +
		"/dev/sda2 /boot/efi vfat rw 0 0\n" +
		"/dev/sdc1 /mnt/ro ext4 ro,relatime 0 0\n" +
		"tmpfs /run tmpfs rw 0 0\n" +
		"overlay /var/lib/docker/overlay2/x/merged overlay rw 0 0\n" +
		"/dev/sdd1 /var/lib/docker ext4 rw 0 0\n",
}

func factsEnv(t *testing.T, tr tree) FactsEnv {
	root := t.TempDir()
	tr.write(t, root)
	sr, err := platform.NewSysroot(root)
	require.NoError(t, err)
	sizes := map[string]FS{
		"/":                 {Dev: "8:1", Total: 100 << 30, Free: 40 << 30, Avail: 35 << 30},
		"/srv/data disk":    {Dev: "8:17", Total: 2000 << 30, Free: 1500 << 30, Avail: 1500 << 30},
		"/home/user/proj":   {Dev: "8:1", Total: 100 << 30, Free: 40 << 30, Avail: 35 << 30},
		"/mnt/empty-statfs": {Dev: "0:9", Total: 0},
	}
	return FactsEnv{
		Sys:      sr,
		Hostname: func() (string, error) { return "web1", nil },
		Uname:    func() (string, string, string, bool) { return "Linux", "6.1.0-18-amd64", "x86_64", true },
		Statfs: func(p string) (FS, bool) {
			fs, ok := sizes[p]
			return fs, ok
		},
		NumCPU: func() int { return 8 },
		Now:    func() time.Time { return time.Unix(1761400000, 0) },
		Tool: func(name string, args ...string) ([]byte, bool) {
			if name == "nvidia-smi" {
				return []byte("NVIDIA A100-SXM4-40GB, 1024, 40960, 7\nbad line\n"), true
			}
			return nil, false
		},
	}
}

func TestCollectFactsLinux(t *testing.T) {
	f := CollectFacts(factsEnv(t, factsTree))
	b, err := json.Marshal(f)
	require.NoError(t, err)
	require.Equal(t, `{"host":"web1","os":"Debian GNU/Linux 12","kernel":"6.1.0-18-amd64","uptime_h":26,"load":[0.52,0.48,1.05],`+
		`"cpus":8,"cpu":"Intel Xeon E5-2690 v4 @ 2.60GHz","mem_mb":{"total":16000,"avail":8000},`+
		`"disks":[{"mount":"/","size_gb":100,"used_pct":64},{"mount":"/srv/data disk","size_gb":2000,"used_pct":25}],`+
		`"gpu":[{"name":"NVIDIA A100-SXM4-40GB","mem_mb":[1024,40960],"util_pct":7}]}`, string(b))
	// One disk and no GPU, the common case, stays within 250 bytes; each
	// further disk or GPU adds one short object.
	e := factsEnv(t, factsTree)
	e.Mounts, e.Tool = []string{"/"}, nil
	b, err = json.Marshal(CollectFacts(e))
	require.NoError(t, err)
	require.LessOrEqual(t, len(b), 250, string(b))

	// --mount: the named paths, one per filesystem, a path with no size
	// left out; no nvidia-smi, no GPU key; no MemAvailable, free+buffers+cached.
	e = factsEnv(t, tree{
		"proc/loadavg":             "1 2 3\n",
		"proc/meminfo":             "MemTotal: 2048000 kB\nMemFree: 512000 kB\nBuffers: 102400 kB\nCached: 409600 kB\n",
		"proc/sys/kernel/hostname": "fromproc\n",
	})
	e.Mounts = []string{"/", "/home/user/proj", "/mnt/empty-statfs", "/nowhere"}
	e.Tool = func(string, ...string) ([]byte, bool) { return nil, false }
	e.Hostname = func() (string, error) { return "", errors.New("no") }
	b, err = json.Marshal(CollectFacts(e))
	require.NoError(t, err)
	require.Equal(t, `{"host":"fromproc","kernel":"6.1.0-18-amd64","uptime_h":0,"load":[1,2,3],"cpus":8,`+
		`"mem_mb":{"total":2000,"avail":1000},"disks":[{"mount":"/","size_gb":100,"used_pct":64}]}`, string(b))
}

func TestCollectFactsDarwin(t *testing.T) {
	e := factsEnv(t, tree{"etc/": ""})
	e.Uname = func() (string, string, string, bool) { return "Darwin", "24.1.0", "arm64", true }
	e.Tool = func(name string, args ...string) ([]byte, bool) {
		if name != "sysctl" {
			return nil, false
		}
		return []byte("{ 1.50 1.20 0.90 }\n{ sec = 1761392800, usec = 0 } Sat Oct 25 10:00:00 2025\n17179869184\nApple M2 Pro\n15.1\n"), true
	}
	b, err := json.Marshal(CollectFacts(e))
	require.NoError(t, err)
	require.Equal(t, `{"host":"web1","os":"macOS 15.1","kernel":"24.1.0","uptime_h":2,"load":[1.5,1.2,0.9],"cpus":8,`+
		`"cpu":"Apple M2 Pro","mem_mb":{"total":16384,"avail":0},"disks":[{"mount":"/","size_gb":100,"used_pct":64}]}`, string(b))

	// Nothing known: the keys an agent reads are still there.
	b, err = json.Marshal(CollectFacts(FactsEnv{Sys: e.Sys}))
	require.NoError(t, err)
	require.Equal(t, `{"host":"","uptime_h":0,"load":[],"cpus":0,"mem_mb":{"total":0,"avail":0},"disks":[]}`, string(b))
}

func TestFactsHelpers(t *testing.T) {
	require.Equal(t, "AMD Ryzen 9 7950X 16-Core", shortCPU("AMD Ryzen 9 7950X 16-Core Processor"))
	require.Equal(t, "Intel Core i7-8550U @ 1.80GHz", shortCPU("Intel(R) Core(TM) i7-8550U CPU @ 1.80GHz"))
	require.Equal(t, "/a b\\x", unescapeMount(`/a\040b\x`))
	require.Equal(t, "/plain", unescapeMount("/plain"))
	require.Empty(t, parseGPUs([]byte("")))
	require.Equal(t, []string{"/"}, autoMounts(FactsEnv{}, false))
}
