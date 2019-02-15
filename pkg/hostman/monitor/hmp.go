package monitor

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"yunion.io/x/jsonutils"
	"yunion.io/x/log"
)

type HmpMonitor struct {
	SBaseMonitor

	commandQueue  []string
	callbackQueue []StringCallback
}

func NewHmpMonitor(OnMonitorDisConnect, OnMonitorTimeout MonitorErrorFunc, OnMonitorConnected MonitorSuccFunc) *HmpMonitor {
	return &HmpMonitor{
		SBaseMonitor:  *NewBaseMonitor(OnMonitorConnected, OnMonitorDisConnect, OnMonitorTimeout),
		commandQueue:  make([]string, 0),
		callbackQueue: make([]StringCallback, 0),
	}
}

var hmpMark = []byte("(qemu) ")

func (m *HmpMonitor) hmpSplitFunc(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	index := bytes.Index(data, hmpMark)
	if index >= 0 {
		return index + len(hmpMark), data[0:index], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	// Request more data.
	return 0, nil, nil
}

func (m *HmpMonitor) actionResult(res string) string {
	return res
}

func (m *HmpMonitor) read(r io.Reader) {
	if !m.checkReading() {
		return
	}
	scanner := bufio.NewScanner(r)
	scanner.Split(m.hmpSplitFunc)
	for scanner.Scan() {
		res := scanner.Text()
		if len(res) == 0 {
			continue
		}
		if m.connected {
			go m.callBack(res)
		} else {
			// remove reader timeout
			m.connected = true
			m.rwc.SetReadDeadline(time.Time{})
		}
	}
	log.Errorln("Scan over ...")
	if err := scanner.Err(); err != nil {
		log.Errorln(err)
		if m.connected {
			m.connected = false
			m.OnMonitorDisConnect(err)
		} else {
			m.OnMonitorTimeout(err)
		}
	}
	m.reading = false
}

func (m *HmpMonitor) callBack(res string) {
	m.mutex.Lock()
	if len(m.callbackQueue) == 0 {
		return
	}
	cb := m.callbackQueue[0]
	m.callbackQueue = m.callbackQueue[1:]
	m.mutex.Unlock()
	if cb != nil {
		go cb(res)
	}
}

func (m *HmpMonitor) write(cmd []byte) error {
	cmd = append(cmd, '\n')
	length, index := len(cmd), 0
	for index < length {
		i, err := m.rwc.Write(cmd)
		if err != nil {
			return err
		}
		index += i
	}
	return nil
}

func (m *HmpMonitor) query() {
	if !m.checkWriting() {
		return
	}
	for {
		if len(m.commandQueue) == 0 {
			break
		}
		//pop
		m.mutex.Lock()
		cmd := m.commandQueue[0]
		m.commandQueue = m.commandQueue[1:]
		err := m.write([]byte(cmd))
		m.mutex.Unlock()
		if err != nil {
			log.Errorf("Write %s to monitor error: %s", cmd, err)
			break
		}
	}
	m.writing = false
}

func (m *HmpMonitor) Query(cmd string, cb StringCallback) {
	// push
	m.mutex.Lock()
	m.commandQueue = append(m.commandQueue, cmd)
	m.callbackQueue = append(m.callbackQueue, cb)
	m.mutex.Unlock()
	if m.connected {
		if !m.writing {
			go m.query()
		}
		if !m.reading {
			go m.read(m.rwc)
		}
	}

}

func (m *HmpMonitor) Connect(host string, port int) error {
	err := m.SBaseMonitor.Connect(host, port)
	if err != nil {
		return err
	}
	go m.read(m.rwc)
	return nil
}

func (m *HmpMonitor) QueryStatus(callback StringCallback) {
	m.Query("info status", m.parseStatus(callback))
}

func (m *HmpMonitor) parseStatus(callback StringCallback) StringCallback {
	return func(res string) {
		strs := strings.Split(res, "\r\n")
		for _, str := range strs {
			if strings.HasPrefix(str, "VM status:") {
				callback(strings.TrimSpace(str[len("VM status:"):]))
				return
			}
		}
	}
}

func (m *HmpMonitor) SimpleCommand(cmd string, callback StringCallback) {
	m.Query(cmd, callback)
}

func (m *HmpMonitor) HumanMonitorCommand(cmd string, callback StringCallback) {
	m.Query(cmd, callback)
}

func (m *HmpMonitor) GetVersion(callback StringCallback) {
	m.Query("info version", callback)
}

func (m *HmpMonitor) GetBlocks(callback func(*jsonutils.JSONArray)) {
	var cb = func(res string) {
		var lines = strings.Split(res, "\r\n")
		var mergedOutput = []string{}

		// merge output
		for _, line := range lines {
			parts := regexp.MustCompile(`\s+`).Split(line, -1)
			if parts[0][len(parts[0])-1] == ':' {
				mergedOutput = append(mergedOutput, "")
			} else if regexp.MustCompile(`\(#block\d+\):`).MatchString(line) {
				mergedOutput = append(mergedOutput, "")
			}
			mergedOutput[len(mergedOutput)-1] = mergedOutput[len(mergedOutput)-1] + " " + line
			mergedOutput[len(mergedOutput)-1] = strings.TrimSpace(mergedOutput[len(mergedOutput)-1])
		}

		// parse to json
		var outputJson = jsonutils.NewArray()
		for _, line := range mergedOutput {
			parts := regexp.MustCompile(`\s+`).Split(line, -1)
			if parts[0][len(parts[0])-1] == ':' ||
				regexp.MustCompile(`\(#block\d+\):`).MatchString(parts[1]) {

				drv := jsonutils.NewDict()
				drv.Set("device", jsonutils.NewString(parts[0]))
				if parts[0][len(parts[0])-1] == ':' {
					drv.Set("device", jsonutils.NewString(parts[0][:len(parts)-1]))
				}
				if regexp.MustCompile(`\(#block\d+\):`).MatchString(parts[1]) {
					inserted := jsonutils.NewDict()
					inserted.Set("file", jsonutils.NewString(parts[2]))
					for i := 0; i < len(parts)-2; i++ {
						if parts[i] == "Backing" && parts[i+1] == "file:" {
							inserted.Set("backing_file", jsonutils.NewString(parts[i+2]))
							break
						}
					}
					drv.Set("inserted", inserted)
				}
				outputJson.Add(drv)
			}
		}

		callback(outputJson)
	}

	m.Query("info block", cb)
}

func (m *HmpMonitor) EjectCdrom(dev string, callback StringCallback) {
	m.Query(fmt.Sprintf("eject -f %s", dev), callback)
}

func (m *HmpMonitor) ChangeCdrom(dev string, path string, callback StringCallback) {
	m.Query(fmt.Sprintf("change %s %s", dev, path), callback)
}

func (m *HmpMonitor) DriveDel(idstr string, callback StringCallback) {
	m.Query(fmt.Sprintf("drive_del %s", idstr), callback)
}

func (m *HmpMonitor) DeviceDel(idstr string, callback StringCallback) {
	m.Query(fmt.Sprintf("device_del %s", idstr), callback)
}

func (m *HmpMonitor) DriveAdd(bus string, params map[string]string, callback StringCallback) {
	var paramsKvs = []string{}
	for k, v := range params {
		paramsKvs = append(paramsKvs, fmt.Sprintf("%s=%s", k, v))
	}
	m.Query(fmt.Sprintf("drive_add %s %s", bus, strings.Join(paramsKvs, ",")), callback)
}

func (m *HmpMonitor) DeviceAdd(dev string, params map[string]interface{}, callback StringCallback) {
	var paramsKvs = []string{}
	for k, v := range params {
		paramsKvs = append(paramsKvs, fmt.Sprintf("%s=%v", k, v))
	}
	m.Query(fmt.Sprintf("device_add %s,%s", dev, strings.Join(paramsKvs, ",")), callback)
}

func (m *HmpMonitor) MigrateSetCapability(capability, state string, callback StringCallback) {
	m.Query(fmt.Sprintf("migrate_set_capability %s %s", capability, state), callback)
}

func (m *HmpMonitor) Migrate(
	destStr string, copyIncremental, copyFull bool, callback StringCallback,
) {
	cmd := "migrate -d"
	if copyIncremental {
		cmd += " -i"
	} else if copyFull {
		cmd += " -b"
	}
	cmd += " " + destStr
	m.Query(cmd, callback)
}

func (m *HmpMonitor) GetMigrateStatus(callback StringCallback) {
	cb := func(output string) {
		log.Infof("Query migrate status: %s", output)

		var status string
		for _, line := range strings.Split(output, "\n") {
			if strings.HasPrefix(line, "Migration status") {
				status = line[strings.LastIndex(line, " ")+1:]
				break
			}
		}
		callback(status)
	}

	m.Query("info migrate", cb)
}

func (m *HmpMonitor) GetBlockJobs(callback func(jobs int)) {
	cb := func(output string) {
		lines := strings.Split(output, "\n")
		if lines[0] == "No active jobs" {
			callback(0)
		} else {
			callback(len(lines))
		}
	}

	m.Query("info block-jobs", cb)
}

func (m *HmpMonitor) ReloadDiskBlkdev(device, path string, callback StringCallback) {
	m.Query(fmt.Sprintf("reload_disk_snapshot_blkdev -n %s %s", device, path), callback)
}

func (m *HmpMonitor) DriveMirror(callback StringCallback, drive, target, syncMode string, unmap bool) {
	cmd := "drive_mirror -n"
	if syncMode == "full" {
		cmd += " -f"
	}
	cmd += fmt.Sprintf(" %s %s", drive, target)
	m.Query(cmd, callback)
}

func (m *HmpMonitor) BlockStream(drive string, callback StringCallback) {
	var (
		speed = 30 // MB/s
		cmd   = fmt.Sprintf("block_stream %s %d", drive, speed)
	)
	m.Query(cmd, callback)
}

func (m *HmpMonitor) SetVncPassword(proto, password string, callback StringCallback) {
	if len(password) > 8 {
		password = password[:8]
	}
	m.Query(fmt.Sprintf("set_password %s %s", proto, password), callback)
}

func (m *HmpMonitor) StartNbdServer(port int, exportAllDevice, writable bool, callback StringCallback) {
	var cmd = "nbd_server_start"
	if exportAllDevice {
		cmd += " -a"
	}
	if writable {
		cmd += " -w"
	}
	cmd += fmt.Sprintf(" 0.0.0.0:%d", port)
	m.Query(cmd, callback)
}

func (m *HmpMonitor) ResizeDisk(driveName string, sizeMB int64, callback StringCallback) {
	cmd := fmt.Sprintf("block_resize %s %d", driveName, sizeMB)
	m.Query(cmd, callback)
}