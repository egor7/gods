// This programm collects some system information, formats it nicely and sets
// the X root windows name so it can be displayed in the dwm status bar.
//
// (https://github.com/schachmat/status-18), you should probably exchange them
// by something else ("CPU", "MEM", "|" for separators, …).
//
// For license information see the file LICENSE
package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	unpluggedSign = "BAT"
	pluggedSign   = "AC"

	cpuSign = "CPU"
	memSign = "MEM"
	netSign = "NET"

	floatSeparator = "."
	dateSeparator  = "|"
	fieldSeparator = " | "
)

var (
	netDevs = map[string]struct{}{
		"wlp1s0:": {},
	}
	cores = runtime.NumCPU() // count of cores to scale cpu usage
	rxOld = 0
	txOld = 0
)

// velocity calculates the speed using actual and old byte number
func velocity(vel int) string {
	velMB := vel / 1000000
	if velMB != 0 {
		return fmt.Sprintf("%3dM", velMB)
	}
	velKB := vel / 1000
	if velKB != 0 {
		return fmt.Sprintf("%3dK", velKB)
	}
	return fmt.Sprintf("%3dB", vel)
}

// updateNetUse reads current transfer rates of certain network interfaces
func updateNetUse() string {
	file, err := os.Open("/proc/net/dev")

	if err != nil {
		return netSign + " ERR"
	}

	defer file.Close()

	var void = 0 // target for unused values
	var dev, rx, tx, rxNow, txNow = "", 0, 0, 0, 0
	var scanner = bufio.NewScanner(file)

	for scanner.Scan() {
		_, err = fmt.Sscanf(
			scanner.Text(),
			"%s %d %d %d %d %d %d %d %d %d",
			&dev, &rx, &void, &void, &void, &void, &void, &void, &void, &tx,
		)

		if _, ok := netDevs[dev]; ok {
			rxNow += rx
			txNow += tx
		}
	}

	defer func() { rxOld, txOld = rxNow, txNow }()

	var download, upload string = " ", " "

	if rxNow-rxOld != 0.0 {
		download = "↓"
	}

	if txNow-txOld != 0.0 {
		upload = "↑"
	}

	return fmt.Sprintf("%s %s%s %s", netSign, upload, download, velocity(rxNow-rxOld+txNow-txOld))
}

// updatePower reads the current battery and power plug status
func updatePower() string {
	const powerSupply = "/sys/class/power_supply"
	var enFull, enNow, enPerc, curNow int = 0, 0, 0, 0
	var plugged, err = ioutil.ReadFile(powerSupply + "/AC/online")

	if err != nil {
		return "ERR"
	}

	batts, err := ioutil.ReadDir(powerSupply)

	if err != nil {
		return "ERR"
	}

	for _, batt := range batts {
		name := batt.Name()

		if !strings.HasPrefix(name, "BAT") {
			continue
		}

		batteryValues := parseFile(powerSupply + "/" + batt.Name() + "/uevent")

		enFull += batteryValues.SearchForInt([]string{"POWER_SUPPLY_ENERGY_FULL", "POWER_SUPPLY_CHARGE_FULL"})
		enNow += batteryValues.SearchForInt([]string{"POWER_SUPPLY_ENERGY_NOW", "POWER_SUPPLY_CHARGE_NOW"})
		curNow += batteryValues.SearchForInt([]string{"POWER_SUPPLY_CURRENT_NOW", "POWER_SUPPLY_POWER_NOW"})
	}

	if enFull == 0 { // Battery found but no readable full file.
		return "ERR"
	}

	enPerc = enNow * 100 / enFull
	icon := unpluggedSign
	timeRemaining := ""

	if plugged[0] == '1' {
		icon = pluggedSign
	} else if curNow != 0 {
		remaining := float32(enNow) / float32(curNow)
		time_in_min := int(remaining * 60)
		hours := time_in_min / 60
		time_in_min -= hours * 60

		timeRemaining = fmt.Sprintf(" [%2d:%02d]", hours, time_in_min)
	}

	return fmt.Sprintf("%s %3d%s", icon, enPerc, timeRemaining)
}

// updateCPUUse reads the last minute sysload and scales it to the core count
func updateCPUUse() string {
	var load float32
	var loadavg, err = ioutil.ReadFile("/proc/loadavg")

	if err != nil {
		return cpuSign + "ERR"
	}

	_, err = fmt.Sscanf(string(loadavg), "%f", &load)

	if err != nil {
		return cpuSign + "ERR"
	}
	return fmt.Sprintf("%s%3d", cpuSign, int(load*100.0/float32(cores)))
}

// updateMemUse reads the memory used by applications and scales to [0, 100]
func updateMemUse() string {
	var file, err = os.Open("/proc/meminfo")
	if err != nil {
		return memSign + "ERR"
	}
	defer file.Close()

	// done must equal the flag combination (0001 | 0010 | 0100 | 1000) = 15
	var total, used, done = 0, 0, 0

	for info := bufio.NewScanner(file); done != 15 && info.Scan(); {
		var prop, val = "", 0
		if _, err = fmt.Sscanf(info.Text(), "%s %d", &prop, &val); err != nil {
			return memSign + "ERR"
		}
		switch prop {
		case "MemTotal:":
			total = val
			used += val
			done |= 1
		case "MemFree:":
			used -= val
			done |= 2
		case "Buffers:":
			used -= val
			done |= 4
		case "Cached:":
			used -= val
			done |= 8
		}
	}
	return fmt.Sprintf("%s%3d", memSign, used*100/total)
}

func getHostname() (hostname string) {
	if tmp, err := ioutil.ReadFile("/etc/hostname"); err == nil {
		hostname = strings.TrimSpace(string(tmp))
	}

	return
}

// main updates the dwm statusbar every second
func main() {
	for {
		var status = []string{
			//getHostname(),
			updateCPUUse(),
			updateMemUse(),
			updatePower(),
			updateNetUse(),
			time.Now().Local().Format("Mon 02 " + dateSeparator + " 15:04:05"),
		}
		exec.Command("xsetroot", "-name", strings.Join(status, fieldSeparator)).Run()

		// sleep until beginning of next second
		var now = time.Now()

		time.Sleep(now.Truncate(time.Second).Add(time.Second).Sub(now))
	}
}
