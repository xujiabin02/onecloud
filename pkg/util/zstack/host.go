package zstack

import (
	"fmt"
	"strings"
	"time"

	"yunion.io/x/jsonutils"
	"yunion.io/x/log"
	"yunion.io/x/onecloud/pkg/cloudprovider"

	api "yunion.io/x/onecloud/pkg/apis/compute"
)

type SHost struct {
	zone *SZone

	ZStackBasic
	Username                string `json:"username"`
	SSHPort                 int    `json:"sshPort"`
	ZoneUUID                string `json:"zoneUuid"`
	ClusterUUID             string `json:"clusterUuid"`
	ManagementIP            string `json:"managementIp"`
	HypervisorType          string `json:"hypervisorType"`
	State                   string `json:"state"`
	Status                  string `json:"status"`
	TotalCPUCapacity        int    `json:"totalCpuCapacity"`
	AvailableCPUCapacity    int    `json:"availableCpuCapacity"`
	CPUSockets              int    `json:"cpuSockets"`
	TotalMemoryCapacity     int    `json:"totalMemoryCapacity"`
	AvailableMemoryCapacity int    `json:"availableMemoryCapacity"`
	CPUNum                  int    `json:"cpuNum"`
	ZStackTime
}

func (region *SRegion) GetHosts(zoneId string, hostId string) ([]SHost, error) {
	hosts := []SHost{}
	params := []string{}
	if len(zoneId) > 0 {
		params = append(params, "q=zone.uuid="+zoneId)
	}
	if len(hostId) > 0 {
		params = append(params, "q=uuid="+hostId)
	}
	if SkipEsxi {
		params = append(params, "q=hypervisorType!=ESX")
	}
	return hosts, region.client.listAll("hosts", params, &hosts)
}

func (region *SRegion) GetHost(hostId string) (*SHost, error) {
	host := &SHost{}
	err := region.client.getResource("hosts", hostId, host)
	if err != nil {
		return nil, err
	}
	zone, err := region.GetZone(host.ZoneUUID)
	if err != nil {
		return nil, err
	}
	host.zone = zone
	return host, nil
}

func (host *SHost) GetMetadata() *jsonutils.JSONDict {
	return nil
}

func (host *SHost) GetIWires() ([]cloudprovider.ICloudWire, error) {
	wires, err := host.zone.region.GetWires(host.ZoneUUID, "", host.ClusterUUID)
	if err != nil {
		return nil, err
	}
	iwires := []cloudprovider.ICloudWire{}
	for i := 0; i < len(wires); i++ {
		iwires = append(iwires, &wires[i])
	}
	return iwires, nil
}

func (host *SHost) GetIStorages() ([]cloudprovider.ICloudStorage, error) {
	storages, err := host.zone.region.GetStorages(host.zone.UUID, host.ClusterUUID, "")
	if err != nil {
		return nil, err
	}
	istorages := []cloudprovider.ICloudStorage{}
	for i := 0; i < len(storages); i++ {
		switch storages[i].Type {
		case StorageTypeLocal:
			localStorages, err := host.zone.region.getILocalStorages(storages[i].UUID, host.UUID)
			if err != nil {
				return nil, err
			}
			istorages = append(istorages, localStorages...)
		case StorageTypeCeph:
			istorages = append(istorages, &storages[i])
		case StorageTypeVCenter:
		}
	}
	return istorages, nil
}

func (host *SHost) GetIStorageById(id string) (cloudprovider.ICloudStorage, error) {
	return host.zone.GetIStorageById(id)
}

func (host *SHost) GetIVMs() ([]cloudprovider.ICloudVM, error) {
	instances, err := host.zone.region.GetInstances(host.UUID, "", "")
	if err != nil {
		return nil, err
	}
	iInstnace := []cloudprovider.ICloudVM{}
	for i := 0; i < len(instances); i++ {
		instances[i].host = host
		iInstnace = append(iInstnace, &instances[i])
	}
	return iInstnace, nil
}

func (host *SHost) GetIVMById(instanceId string) (cloudprovider.ICloudVM, error) {
	instance, err := host.zone.region.GetInstance(instanceId)
	if err != nil {
		return nil, err
	}
	instance.host = host
	return instance, nil
}

func (host *SHost) GetId() string {
	return host.UUID
}

func (host *SHost) GetName() string {
	return host.Name
}

func (host *SHost) GetGlobalId() string {
	return host.GetId()
}

func (host *SHost) IsEmulated() bool {
	return false
}

func (host *SHost) GetStatus() string {
	if host.Status == "Connected" {
		return api.HOST_STATUS_RUNNING
	}
	return api.HOST_STATUS_UNKNOWN
}

func (host *SHost) Refresh() error {
	return nil
}

func (host *SHost) GetHostStatus() string {
	return api.HOST_ONLINE
}

func (host *SHost) GetEnabled() bool {
	return host.State == "Enabled"
}

func (host *SHost) GetAccessIp() string {
	return host.ManagementIP
}

func (host *SHost) GetAccessMac() string {
	return ""
}

func (host *SHost) GetSysInfo() jsonutils.JSONObject {
	info := jsonutils.NewDict()
	info.Add(jsonutils.NewString(CLOUD_PROVIDER_ZSTACK), "manufacture")
	return info
}

func (host *SHost) GetSN() string {
	return ""
}

func (host *SHost) GetCpuCount() int {
	return host.TotalCPUCapacity
}

func (host *SHost) GetNodeCount() int8 {
	return int8(host.CPUSockets)
}

func (host *SHost) GetCpuDesc() string {
	return ""
}

func (host *SHost) GetCpuMhz() int {
	return 0
}

func (host *SHost) GetMemSizeMB() int {
	return host.TotalMemoryCapacity / 1024 / 1024
}

func (host *SHost) GetStorageSizeMB() int {
	storages, err := host.zone.region.GetStorages(host.zone.UUID, host.ClusterUUID, "")
	if err != nil {
		return 0
	}
	totalStorage := 0
	for _, storage := range storages {
		if storage.Type == StorageTypeLocal {
			localStorages, err := host.zone.region.GetLocalStorages(storage.UUID, host.UUID)
			if err != nil {
				return 0
			}
			for i := 0; i < len(localStorages); i++ {
				totalStorage += int(localStorages[i].TotalCapacity)
			}
		}
	}
	return totalStorage / 1024 / 1024
}

func (host *SHost) GetStorageType() string {
	return api.DISK_TYPE_HYBRID
}

func (host *SHost) GetHostType() string {
	return api.HOST_TYPE_ZSTACK
}

func (region *SRegion) cleanDisks(diskIds []string) {
	for i := 0; i < len(diskIds); i++ {
		err := region.DeleteDisk(diskIds[i])
		if err != nil {
			log.Errorf("clean disk %s error: %v", diskIds[i], err)
		}
	}
}

func (region *SRegion) createDataDisks(name, hostId string, storages []cloudprovider.ICloudStorage, disks []cloudprovider.SDiskInfo) ([]string, error) {
	diskIds := []string{}
	for i := 0; i < len(disks); i++ {
		for j := 0; j < len(storages); j++ {
			poolName := ""
			if storages[i].GetStorageType() == disks[i].StorageType {
				switch disks[i].StorageType {
				case "localstorage":
					poolName = ""
				case "ceph":
					hostId = ""
					storage := storages[j].(*SStorage)
					poolName, _ := storage.GetDataPoolName()
					if len(poolName) == 0 {
						return []string{}, fmt.Errorf("failed to found data pool for ceph storage %s", storage.Name)
					}
				default:
					return diskIds, fmt.Errorf("not support storageType %s", disks[i].StorageType)
				}
				name := fmt.Sprintf("vdisk_%s_%d", name, time.Now().UnixNano())
				disk, err := region.CreateDisk(name, storages[j].GetId(), hostId, poolName, disks[i].SizeGB, "")
				if err != nil {
					return diskIds, err
				}
				diskIds = append(diskIds, disk.UUID)
			}
		}
	}
	return diskIds, nil
}

func (host *SHost) CreateVM(desc *cloudprovider.SManagedVMCreateConfig) (cloudprovider.ICloudVM, error) {
	storages, err := host.GetIStorages()
	if err != nil {
		return nil, err
	}
	diskIds, err := host.zone.region.createDataDisks(desc.Name, host.UUID, storages, desc.DataDisks)
	if err != nil {
		defer host.zone.region.cleanDisks(diskIds)
		return nil, err
	}
	rootStorageId := ""
	for i := 0; i < len(storages); i++ {
		if storages[i].GetStorageType() == desc.SysDisk.StorageType {
			rootStorageId = storages[i].GetId()
		}
	}
	if len(rootStorageId) == 0 {
		return nil, fmt.Errorf("failed to found appropriate storage for root disk")
	}
	instance, err := host.zone.region._createVM(desc, host.UUID, rootStorageId)
	if err != nil {
		defer host.zone.region.cleanDisks(diskIds)
		return nil, err
	}
	for i := 0; i < len(diskIds); i++ {
		err = host.zone.region.AttachDisk(instance.UUID, diskIds[i])
		if err != nil {
			log.Errorf("failed to attach disk %s into instance %s error: %v", diskIds[i], instance.Name, err)
		}
	}
	err = host.zone.region.AssignSecurityGroup(instance.UUID, desc.ExternalSecgroupId)
	if err != nil {
		return nil, err
	}
	return host.GetIVMById(instance.UUID)
}

func (region *SRegion) _createVM(desc *cloudprovider.SManagedVMCreateConfig, hostId string, rootStorageId string) (*SInstance, error) {
	l3Id := strings.Split(desc.ExternalNetworkId, "/")[0]
	if len(l3Id) == 0 {
		return nil, fmt.Errorf("invalid networkid: %s", desc.ExternalNetworkId)
	}
	_, err := region.GetL3Network(l3Id)
	if err != nil {
		log.Errorf("failed to found l3network %s error: %v", l3Id, err)
		return nil, err
	}
	offerings := map[string]string{}
	if len(desc.InstanceType) > 0 {
		offering, err := region.GetInstanceOfferingByType(desc.InstanceType)
		if err != nil {
			return nil, err
		}
		offerings[offering.Name] = offering.UUID
	} else {
		_offerings, err := region.GetInstanceOfferings("", "", desc.Cpu, desc.MemoryMB)
		if err != nil {
			return nil, err
		}
		for _, offering := range _offerings {
			offerings[offering.Name] = offering.UUID
		}
		if len(offerings) == 0 {
			return nil, fmt.Errorf("instance type %dC%dMB not avaiable", desc.Cpu, desc.MemoryMB)
		}
	}
	return region.CreateInstance(desc, l3Id, hostId, rootStorageId, offerings)
}

func (region *SRegion) CreateInstance(desc *cloudprovider.SManagedVMCreateConfig, l3Id, hostId, rootStorageId string, offerings map[string]string) (*SInstance, error) {
	instance := &SInstance{}
	systemTags := []string{
		"cdroms::Empty::None::None",
		"usbRedirect::false",
		fmt.Sprintf("staticIp::%s::%s", l3Id, desc.IpAddr),
		"vmConsoleMode::vnc",
		"cleanTraffic::false",
	}
	if len(desc.UserData) > 0 {
		systemTags = append(systemTags, "userdata::"+desc.UserData)
	}
	if len(desc.PublicKey) > 0 {
		systemTags = append(systemTags, "sshkey::"+desc.PublicKey)
	}
	var err error
	for offerName, offerId := range offerings {
		params := map[string]interface{}{
			"params": map[string]interface{}{
				"name":                 desc.Name,
				"description":          desc.Description,
				"instanceOfferingUuid": offerId,
				"imageUuid":            desc.ExternalImageId,
				"l3NetworkUuids": []string{
					l3Id,
				},
				"hostUuid":                        hostId,
				"dataVolumeSystemTags":            []string{},
				"rootVolumeSystemTags":            []string{},
				"vmMachineType":                   "",
				"tagUuids":                        []string{},
				"defaultL3NetworkUuid":            l3Id,
				"primaryStorageUuidForRootVolume": rootStorageId,
				"dataDiskOfferingUuids":           []string{},
				"systemTags":                      systemTags,
				"vmNicConfig":                     []string{},
			},
		}
		log.Debugf("Try instanceOffering : %s", offerName)
		err = region.client.create("vm-instances", jsonutils.Marshal(params), instance)
		if err == nil {
			return instance, nil
		}
		log.Errorf("create %s instance failed error: %v", offerName, err)
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("instance type %dC%dMB not avaiable", desc.Cpu, desc.MemoryMB)
}

func (host *SHost) GetIHostNics() ([]cloudprovider.ICloudHostNetInterface, error) {
	return nil, cloudprovider.ErrNotSupported
}

func (host *SHost) GetIsMaintenance() bool {
	return false
}

func (host *SHost) GetVersion() string {
	return ""
}
