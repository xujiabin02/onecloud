package zstack

import (
	"fmt"

	"yunion.io/x/jsonutils"

	api "yunion.io/x/onecloud/pkg/apis/compute"
)

type SInstanceOffering struct {
	region *SRegion

	ZStackBasic
	MemorySize        int    `json:"memorySize"`
	CPUNum            int    `json:"cpuNum"`
	CPUSpeed          int    `json:"cpuSpeed"`
	Type              string `json:"type"`
	AllocatorStrategy string `json:"allocatorStrategy"`
	State             string `json:"state"`

	ZStackTime
}

func (region *SRegion) GetInstanceOffering(offerId string) (*SInstanceOffering, error) {
	offer := &SInstanceOffering{region: region}
	return offer, region.client.getResource("instance-offerings", offerId, offer)
}

func (region *SRegion) GetInstanceOfferingByType(instanceType string) (*SInstanceOffering, error) {
	offerings, err := region.GetInstanceOfferings("", instanceType, 0, 0)
	if err != nil {
		return nil, err
	}
	if len(offerings) == 1 {
		return &offerings[0], nil
	}
	if len(offerings) == 0 {
		return nil, fmt.Errorf("instanceType %s not found", instanceType)
	}
	return nil, fmt.Errorf("duplicate instanceType %s", instanceType)
}

func (region *SRegion) GetInstanceOfferings(offerId string, name string, cpu int, memorySizeMb int) ([]SInstanceOffering, error) {
	offerings := []SInstanceOffering{}
	params := []string{"q=type=UserVM", "q=state=Enabled"}
	if len(offerId) > 0 {
		params = append(params, "q=uuid="+offerId)
	}
	if len(name) > 0 {
		params = append(params, "q=name="+name)
	}
	if cpu != 0 {
		params = append(params, fmt.Sprintf("q=cpuNum=%d", cpu))
	}
	if memorySizeMb != 0 {
		params = append(params, fmt.Sprintf("q=memorySize=%d", memorySizeMb*1024*1024))
	}
	if err := region.client.listAll("instance-offerings", params, &offerings); err != nil {
		return nil, err
	}
	for i := 0; i < len(offerings); i++ {
		offerings[i].region = region
	}
	return offerings, nil
}

func (offering *SInstanceOffering) GetMetadata() *jsonutils.JSONDict {
	return nil
}

func (offering *SInstanceOffering) IsEmulated() bool {
	return false
}

func (offering *SInstanceOffering) Refresh() error {
	new, err := offering.region.GetInstanceOffering(offering.UUID)
	if err != nil {
		return err
	}
	return jsonutils.Update(offering, new)
}

func (offering *SInstanceOffering) GetName() string {
	return offering.Name
}

func (offering *SInstanceOffering) GetStatus() string {
	switch offering.State {
	case "Enabled":
		return api.SkuStatusAvailable
	}
	return api.SkuStatusSoldout
}

func (offering *SInstanceOffering) GetId() string {
	return offering.UUID
}

func (offering *SInstanceOffering) GetGlobalId() string {
	return offering.UUID
}

func (offering *SInstanceOffering) GetInstanceTypeFamily() string {
	return offering.AllocatorStrategy
}

func (offering *SInstanceOffering) GetInstanceTypeCategory() string {
	return offering.AllocatorStrategy
}

func (offering *SInstanceOffering) GetPrepaidStatus() string {
	return api.SkuStatusSoldout
}

func (offering *SInstanceOffering) GetPostpaidStatus() string {
	return api.SkuStatusAvailable
}

func (offering *SInstanceOffering) GetCpuCoreCount() int {
	return offering.CPUNum
}

func (offering *SInstanceOffering) GetMemorySizeMB() int {
	return offering.MemorySize / 1024 / 1024
}

func (offering *SInstanceOffering) GetOsName() string {
	return "Any"
}

func (offering *SInstanceOffering) GetSysDiskResizable() bool {
	return true
}

func (offering *SInstanceOffering) GetSysDiskType() string {
	return ""
}

func (offering *SInstanceOffering) GetSysDiskMinSizeGB() int {
	return 0
}

func (offering *SInstanceOffering) GetSysDiskMaxSizeGB() int {
	return 0
}

func (offering *SInstanceOffering) GetAttachedDiskType() string {
	return ""
}

func (offering *SInstanceOffering) GetAttachedDiskSizeGB() int {
	return 0
}

func (offering *SInstanceOffering) GetAttachedDiskCount() int {
	return 6
}

func (offering *SInstanceOffering) GetDataDiskTypes() string {
	return ""
}

func (offering *SInstanceOffering) GetDataDiskMaxCount() int {
	return 6
}

func (offering *SInstanceOffering) GetNicType() string {
	return "vpc"
}

func (offering *SInstanceOffering) GetNicMaxCount() int {
	return 1
}

func (offering *SInstanceOffering) GetGpuAttachable() bool {
	return false
}

func (offering *SInstanceOffering) GetGpuSpec() string {
	return ""
}

func (offering *SInstanceOffering) GetGpuCount() int {
	return 0
}

func (offering *SInstanceOffering) GetGpuMaxCount() int {
	return 0
}
