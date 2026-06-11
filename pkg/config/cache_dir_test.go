package config

import "testing"

func init() {
	inUT = true
}
func Test_no_nvme(t *testing.T) {
	LsblkJson := `{
   "blockdevices": [
      {"name":"sda", "type":"disk", "size":"1.8T", "tran":"sata", "mountpoint":null,
         "children": [
            {"name":"sda1", "type":"part", "size":"1M", "tran":null, "mountpoint":null},
            {"name":"sda2", "type":"part", "size":"100M", "tran":null, "mountpoint":"/boot/efi"},
            {"name":"sda3", "type":"part", "size":"1G", "tran":null, "mountpoint":"/boot"},
            {"name":"sda4", "type":"part", "size":"1.8T", "tran":null, "mountpoint":"/"}
         ]
      }
   ]
}`
	if findNVMeMountpoint([]byte(LsblkJson)) != "" {
		t.Errorf("findNVMeMountpoint() = %v, want %v", findNVMeMountpoint([]byte(LsblkJson)), "")
	}
}

func Test_raid_nvme(t *testing.T) {
	LsblkJson := `{
   "blockdevices": [
      {"name":"sda", "type":"disk", "size":"447.1G", "tran":"sas", "mountpoint":null,
         "children": [
            {"name":"sda1", "type":"part", "size":"1M", "tran":null, "mountpoint":null},
            {"name":"sda2", "type":"part", "size":"100M", "tran":null, "mountpoint":"/boot/efi"},
            {"name":"sda3", "type":"part", "size":"1G", "tran":null, "mountpoint":"/boot"},
            {"name":"sda4", "type":"part", "size":"446G", "tran":null, "mountpoint":"/"}
         ]
      },
      {"name":"nvme0n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"md127", "type":"raid0", "size":"41.9T", "tran":null, "mountpoint":"/data"}
         ]
      },
      {"name":"nvme1n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"md127", "type":"raid0", "size":"41.9T", "tran":null, "mountpoint":"/data"}
         ]
      },
      {"name":"nvme10n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"md127", "type":"raid0", "size":"41.9T", "tran":null, "mountpoint":"/data"}
         ]
      },
      {"name":"nvme4n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"md127", "type":"raid0", "size":"41.9T", "tran":null, "mountpoint":"/data"}
         ]
      },
      {"name":"nvme6n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"md127", "type":"raid0", "size":"41.9T", "tran":null, "mountpoint":"/data"}
         ]
      },
      {"name":"nvme9n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"md127", "type":"raid0", "size":"41.9T", "tran":null, "mountpoint":"/data"}
         ]
      },
      {"name":"nvme8n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"md127", "type":"raid0", "size":"41.9T", "tran":null, "mountpoint":"/data"}
         ]
      },
      {"name":"nvme2n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"md127", "type":"raid0", "size":"41.9T", "tran":null, "mountpoint":"/data"}
         ]
      },
      {"name":"nvme11n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"md127", "type":"raid0", "size":"41.9T", "tran":null, "mountpoint":"/data"}
         ]
      },
      {"name":"nvme7n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"md127", "type":"raid0", "size":"41.9T", "tran":null, "mountpoint":"/data"}
         ]
      },
      {"name":"nvme3n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"md127", "type":"raid0", "size":"41.9T", "tran":null, "mountpoint":"/data"}
         ]
      },
      {"name":"nvme5n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"md127", "type":"raid0", "size":"41.9T", "tran":null, "mountpoint":"/data"}
         ]
      }
   ]
}`
	if findNVMeMountpoint([]byte(LsblkJson)) != "/data" {
		t.Errorf("findNVMeMountpoint() = %v, want %v", findNVMeMountpoint([]byte(LsblkJson)), "/data")
	}
}

func Test_container_nvme(t *testing.T) {
	LsblkJson := `{
   "blockdevices": [
      {
         "name": "nvme0n1",
         "type": "disk",
         "size": "1.7T",
         "tran": "nvme",
         "mountpoint": "/.host/data"
      }
   ]
}`
	if findNVMeMountpoint([]byte(LsblkJson)) != "/data" {
		t.Errorf("findNVMeMountpoint() = %v, want %v", findNVMeMountpoint([]byte(LsblkJson)), "/data")
	}
}

func Test_findNVMeMountpoint_prefers_larger_disk(t *testing.T) {
	LsblkJson := `{
   "blockdevices": [
      {"name":"nvme1n1", "type":"disk", "size":"1T", "tran":"nvme", "mountpoint":"/.host/mnt/nvme9n1"},
      {"name":"nvme2n1", "type":"disk", "size":"2T", "tran":"nvme", "mountpoint":"/.host/mnt/nvme2n1"}
   ]
}`
	if findNVMeMountpointForHost([]byte(LsblkJson), "worker") != "/mnt/nvme2n1" {
		t.Errorf("findNVMeMountpointForHost() = %v, want %v", findNVMeMountpointForHost([]byte(LsblkJson), "worker"), "/mnt/nvme2n1")
	}
}

func Test_findNVMeMountpoint_prefers_later_mountpoint_when_same_size(t *testing.T) {
	LsblkJson := `{
   "blockdevices": [
      {"name":"nvme1n1", "type":"disk", "size":"2T", "tran":"nvme", "mountpoint":"/.host/mnt/nvme3n1"},
      {"name":"nvme2n1", "type":"disk", "size":"2T", "tran":"nvme", "mountpoint":"/.host/mnt/nvme2n1"}
   ]
}`
	if findNVMeMountpointForHost([]byte(LsblkJson), "worker") != "/mnt/nvme3n1" {
		t.Errorf("findNVMeMountpointForHost() = %v, want %v", findNVMeMountpointForHost([]byte(LsblkJson), "worker"), "/mnt/nvme3n1")
	}
}

func Test_findNVMeMountpoint_avoids_nvme1n1_and_nvme2n1_on_h100_h800_hosts(t *testing.T) {
	LsblkJson := `{
   "blockdevices": [
	{
         "name": "nbd7",
         "type": "disk",
         "size": "0B",
         "tran": null,
         "mountpoint": null
      },{
         "name": "nvme0n1",
         "type": "disk",
         "size": "3.5T",
         "tran": "nvme",
         "mountpoint": "/mnt/nvme0n1"
      },{
         "name": "nvme1n1",
         "type": "disk",
         "size": "3.5T",
         "tran": "nvme",
         "mountpoint": "/mnt/nvme1n1"
      },{
         "name": "nvme2n1",
         "type": "disk",
         "size": "3.5T",
         "tran": "nvme",
         "mountpoint": "/mnt/nvme2n1"
      }
	]
}`
	for _, hostname := range []string{"gpu-h100-0", "gpu-h800-0"} {
		if findNVMeMountpointForHost([]byte(LsblkJson), hostname) != "/mnt/nvme0n1" {
			t.Errorf("findNVMeMountpointForHost(%q) = %v, want %v", hostname, findNVMeMountpointForHost([]byte(LsblkJson), hostname), "/mnt/nvme0n1")
		}
	}
}

func Test_findNVMeMountpoint_avoids_nvme1n1_on_h200_hosts(t *testing.T) {
	LsblkJson := `{
   "blockdevices": [
      {"name":"nvme0n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":"/mnt/nvme0n1"},
      {"name":"nvme1n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":"/mnt/nvme1n1"}
   ]
}`
	if findNVMeMountpointForHost([]byte(LsblkJson), "gpu-h200-0") != "/mnt/nvme0n1" {
		t.Errorf("findNVMeMountpointForHost() = %v, want %v", findNVMeMountpointForHost([]byte(LsblkJson), "gpu-h200-0"), "/mnt/nvme0n1")
	}
}

func Test_findNVMeMountpoint_uses_nvme2n1_when_no_other_choice(t *testing.T) {
	LsblkJson := `{
   "blockdevices": [
      {"name":"nvme2n1", "type":"disk", "size":"2T", "tran":"nvme", "mountpoint":"/.host/mnt/nvme2n1"}
   ]
}`
	if findNVMeMountpointForHost([]byte(LsblkJson), "worker-gpu-h100-0") != "/mnt/nvme2n1" {
		t.Errorf("findNVMeMountpointForHost() = %v, want %v", findNVMeMountpointForHost([]byte(LsblkJson), "worker-gpu-h100-0"), "/mnt/nvme2n1")
	}
}

func Test_findNVMeMountpoint_uses_nvme1n1_when_no_other_choice(t *testing.T) {
	LsblkJson := `{
   "blockdevices": [
      {"name":"nvme1n1", "type":"disk", "size":"2T", "tran":"nvme", "mountpoint":"/mnt/nvme1n1"}
   ]
}`
	if findNVMeMountpointForHost([]byte(LsblkJson), "worker-gpu-h200-0") != "/mnt/nvme1n1" {
		t.Errorf("findNVMeMountpointForHost() = %v, want %v", findNVMeMountpointForHost([]byte(LsblkJson), "worker-gpu-h200-0"), "/mnt/nvme1n1")
	}
}

func Test_avoid_root_volume(t *testing.T) {
	LsblkJson := `{
   "blockdevices": [
      {"name":"nvme1n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":"/.host/mnt/nvme0n1"},
      {"name":"nvme3n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":"/.host/mnt/nvme3n1"},
      {"name":"nvme2n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":"/.host/mnt/nvme2n1"},
      {"name":"nvme4n1", "type":"disk", "size":"3.5T", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"nvme4n1p1", "type":"part", "size":"1M", "tran":"nvme", "mountpoint":null},
            {"name":"nvme4n1p2", "type":"part", "size":"100M", "tran":"nvme", "mountpoint":"/.host/boot/efi"},
            {"name":"nvme4n1p3", "type":"part", "size":"1G", "tran":"nvme", "mountpoint":"/.host/boot"},
            {"name":"nvme4n1p4", "type":"part", "size":"3.5T", "tran":"nvme", "mountpoint":"/.host"}
         ]
      },
      {"name":"nvme0n1", "type":"disk", "size":"447.1G", "tran":"nvme", "mountpoint":null,
         "children": [
            {"name":"nvme0n1p1", "type":"part", "size":"1M", "tran":"nvme", "mountpoint":null},
            {"name":"nvme0n1p2", "type":"part", "size":"100M", "tran":"nvme", "mountpoint":null},
            {"name":"nvme0n1p3", "type":"part", "size":"1G", "tran":"nvme", "mountpoint":null},
            {"name":"nvme0n1p4", "type":"part", "size":"446G", "tran":"nvme", "mountpoint":null}
         ]
      }
   ]
}`
	if findNVMeMountpoint([]byte(LsblkJson)) != "/mnt/nvme3n1" {
		t.Errorf("findNVMeMountpoint() = %v, want %v", findNVMeMountpoint([]byte(LsblkJson)), "/mnt/nvme3n1")
	}
}

func Test_root_volume_detect(t *testing.T) {
	if !inRootVolume("/var") {
		t.Errorf("inRootVolume() = %v, want %v", inRootVolume("/var"), true)
	}
	if !inRootVolume("/var/nonexistent") {
		t.Errorf("inRootVolume() = %v, want %v", inRootVolume("/nonexistent"), true)
	}
}
