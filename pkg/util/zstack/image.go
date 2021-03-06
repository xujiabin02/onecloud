package zstack

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sort"
	"time"

	"yunion.io/x/jsonutils"
	"yunion.io/x/log"
	"yunion.io/x/onecloud/pkg/cloudprovider"
	"yunion.io/x/onecloud/pkg/util/httputils"

	api "yunion.io/x/onecloud/pkg/apis/compute"
)

type SBackupStorageRef struct {
	NackupStorageUUID string    `json:"backupStorageUuid"`
	CreateDate        time.Time `json:"createDate"`
	ImageUUID         string    `json:"ImageUuid"`
	InstallPath       string    `json:"installPath"`
	LastOpDate        time.Time `json:"lastOpDate"`
	Status            string    `json:"status"`
}

type SImage struct {
	storageCache *SStoragecache

	BackupStorageRefs []SBackupStorageRef `json:"backupStorageRefs"`
	ActualSize        int                 `json:"actualSize"`
	CreateDate        time.Time           `json:"createDate"`
	Description       string              `json:"description"`
	Format            string              `json:"format"`
	LastOpDate        time.Time           `json:"lastOpDate"`
	MD5Sum            string              `json:"md5sum"`
	MediaType         string              `json:"mediaType"`
	Name              string              `json:"name"`
	Platform          string              `json:"platform"`
	Size              int                 `json:"size"`
	State             string              `json:"state"`
	Status            string              `json:"Ready"`
	System            bool                `json:"system"`
	Type              string              `json:"type"`
	URL               string              `json:"url"`
	UUID              string              `json:"uuid"`
}

func (image *SImage) GetMinRamSizeMb() int {
	return 0
}

func (image *SImage) GetMetadata() *jsonutils.JSONDict {
	data := jsonutils.NewDict()
	return data
}

func (image *SImage) GetId() string {
	return image.UUID
}

func (image *SImage) GetName() string {
	return image.Name
}

func (image *SImage) IsEmulated() bool {
	return false
}

func (image *SImage) Delete(ctx context.Context) error {
	return cloudprovider.ErrNotImplemented
}

func (image *SImage) GetGlobalId() string {
	return image.UUID
}

func (image *SImage) GetIStoragecache() cloudprovider.ICloudStoragecache {
	return image.storageCache
}

func (image *SImage) GetStatus() string {
	switch image.Status {
	case "Ready":
		return api.CACHED_IMAGE_STATUS_READY
	case "Downloading":
		return api.CACHED_IMAGE_STATUS_CACHING
	default:
		log.Errorf("Unknown image status: %s", image.Status)
		return api.CACHED_IMAGE_STATUS_CACHE_FAILED
	}
}

func (image *SImage) GetImageStatus() string {
	switch image.Status {
	case "Ready":
		return cloudprovider.IMAGE_STATUS_ACTIVE
	default:
		return cloudprovider.IMAGE_STATUS_KILLED
	}
}

func (image *SImage) Refresh() error {
	new, err := image.storageCache.region.GetImage(image.UUID)
	if err != nil {
		return err
	}
	return jsonutils.Update(image, new)
}

func (image *SImage) GetImageType() string {
	if image.System {
		return cloudprovider.CachedImageTypeSystem
	}
	return cloudprovider.CachedImageTypeCustomized
}

func (image *SImage) GetSizeByte() int64 {
	return int64(image.Size)
}

func (image *SImage) GetOsType() string {
	return image.Platform
}

func (image *SImage) GetOsDist() string {
	return ""
}

func (image *SImage) GetOsVersion() string {
	return ""
}

func (image *SImage) GetOsArch() string {
	return ""
}

func (image *SImage) GetMinOsDiskSizeGb() int {
	return 10
}

func (image *SImage) GetImageFormat() string {
	return image.Format
}

func (image *SImage) GetCreateTime() time.Time {
	return image.CreateDate
}

func (region *SRegion) GetImage(imageId string) (*SImage, error) {
	image := &SImage{storageCache: region.getStorageCache()}
	return image, region.client.getResource("images", imageId, image)
}

func (region *SRegion) GetImages(imageId string) ([]SImage, error) {
	images := []SImage{}
	params := []string{}
	if len(imageId) > 0 {
		params = append(params, "q=uuid="+imageId)
	}
	if SkipEsxi {
		params = append(params, "q=type!=vmware")
	}
	return images, region.client.listAll("images", params, &images)
}

func (region *SRegion) GetBackupStorageUUID() ([]string, error) {
	imageServers, err := region.GetImageServers("")
	if err != nil {
		return nil, err
	}
	if len(imageServers) == 0 {
		return nil, fmt.Errorf("failed to found any image servers")
	}
	servers := ImageServers(imageServers)
	sort.Sort(servers)
	return []string{servers[0].UUID}, nil
}

func (region *SRegion) CreateImage(imageName, format, osType, desc string, reader io.Reader, size int64) (*SImage, error) {
	backupStorageUUIDs, err := region.GetBackupStorageUUID()
	if err != nil {
		return nil, err
	}
	platform := ""
	switch osType {
	case "linux":
		platform = "Linux"
	case "windows":
		platform = "Windows"
	default:
		platform = "Other"
	}
	parmas := map[string]interface{}{
		"params": map[string]interface{}{
			"name":               imageName,
			"url":                fmt.Sprintf("upload://%s", imageName),
			"description":        desc,
			"mediaType":          "RootVolumeTemplate",
			"system":             false,
			"format":             format,
			"platform":           platform,
			"backupStorageUuids": backupStorageUUIDs,
			"systemTags":         []string{"qemuga", "bootMode::Legacy"},
		},
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("", imageName)
	if err != nil {
		return nil, err
	}

	if reader == nil {
		return nil, fmt.Errorf("invalid reader")
	}

	if size == 0 {
		return nil, fmt.Errorf("invalid image size")
	}

	_, err = io.Copy(part, reader)
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	image := &SImage{storageCache: region.getStorageCache()}
	err = region.client.create("images", jsonutils.Marshal(parmas), image)
	if err != nil {
		return nil, err
	}

	if len(image.BackupStorageRefs) < 0 {
		return nil, fmt.Errorf("no InstallPath reture")
	}
	header := http.Header{}
	header.Add("X-IMAGE-UUID", image.UUID)
	header.Add("X-IMAGE-SIZE", fmt.Sprintf("%d", size))
	header.Add("Content-Type", writer.FormDataContentType())
	_, err = httputils.Request(httputils.GetDefaultClient(), context.Background(), "POST", image.BackupStorageRefs[0].InstallPath, header, body, false)
	return image, err
}
