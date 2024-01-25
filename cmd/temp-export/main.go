package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log/syslog"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	logrus "github.com/sirupsen/logrus"
	lSyslog "github.com/sirupsen/logrus/hooks/syslog"
)

var (
	VERSION               string = "undefined"
	BUILDTIME             string = "manual"
	config                Config
	log                   logrus.Logger
	board                 string           = "unknown"
	customCommandsToCheck []CustomCommands = []CustomCommands{
		{ThermalType: "gpu", Command: "vcgencmd", Args: []string{"measure_temp"}, Regex: "temp=([0-9.]+)'C"},
	}
)

type Config struct {
	LogLevel        string `json:"log_level"`
	LogDestination  string `json:"log_destination"`
	LogFilename     string `json:"log_filename"`
	HttpBindAddress string `json:"http_bind_address"`
	HttpPort        int    `json:"http_port"`
}

type CustomCommands struct {
	Command     string
	Args        []string
	Regex       string
	ThermalType string
}

type Temperature struct {
	Device string
	Temp   float64
}

func checkExists(path string) bool {
	// log.Debugf("Checking if %s exists", path)
	// path should be split on space then be [0]
	path = strings.Split(path, " ")[0]
	path = strings.TrimSpace(path)
	// log.Debugf("Checking if %s exists (now split/trimmed)", path)

	// File may not be executable, we should check if the file exists at all
	_, err := os.Stat(path)

	// log.Debugf("Stat of %s: %s", path, stat)
	if err == nil {
		log.Debugf("Found %s", path)
		return true
	}

	// Check if "which <path>" in case of command
	// log.Debugf("Checking if %s exists (now checking which)", path)
	whichPath, err := exec.Command("which", path).Output()
	if err == nil {
		// log.Debugf("Found %s", string(whichPath))
		return checkExists(string(whichPath))
	}
	// log.Debugf("Failed to find %s", path)
	return false

}

func determineBoard() string {

	log.Debugf("Determining board type...")
	// Check if we're on a tegra
	if checkExists("jetson_release") {
		/*
			# jetson_release
			Software part of jetson-stats 4.2.3 - (c) 2023, Raffaello Bonghi
			Model: lanai-3636 - Jetpack 4.6.4 [L4T 32.7.4]
			NV Power Mode[3]: MAXP_CORE_ARM
			Serial Number: [XXX Show with: jetson_release -s XXX]
			Hardware:
			 - P-Number: p3636-0001
			 - Module: NVIDIA Jetson TX2 NX
			Platform:
			 - Distribution: Ubuntu 18.04 Bionic Beaver
			 - Release: 4.9.337-tegra
			jtop:
			 - Version: 4.2.3
			 - Service: Active
			Libraries:
			 - CUDA: 10.2.300
			 - cuDNN: 8.2.1.32
			 - TensorRT: 8.2
			 - VPI: 1.2.3
			 - Vulkan: 1.2.70
			 - OpenCV: 4.1.1 - with CUDA: NO
		*/
		log.Debugf("Found /usr/bin/jetson_release")
		return "tegra"
	} else if checkExists("/etc/nv_tegra_release") {
		log.Debugf("Found /etc/nv_tegra_release")
		return "tegra"
	} else if checkExists("/sys/firmware/devicetree/base/model") {
		// check if "Raspberry PI" is in the model

		// open the file with ioutil
		// check if it contains "Raspberry Pi"
		log.Debugf("Found /sys/firmware/devicetree/base/model")
		model, err := os.ReadFile("/sys/firmware/devicetree/base/model")
		if err == nil {
			if strings.Contains(string(model), "Raspberry Pi") {
				log.Debugf("Found Raspberry Pi in model: %s", string(model))
				return "raspberrypi"
			} else {
				log.Debugf("Did not find Raspberry Pi in model")
				log.Debugf("Model: %s", string(model))
			}
		}

	}
	log.Debugf("Failed to determine board type")
	return "unknown"

}
func extractTemperature(input string, regex string) (float64, error) {
	re := regexp.MustCompile(regex)
	match := re.FindStringSubmatch(input)
	if match == nil || len(match) < 2 {
		return 0, fmt.Errorf("Temperature not found in the input string")
	}
	// must be float
	temperature := strings.TrimSpace(match[1])
	temperatureInt, err := strconv.ParseFloat(temperature, 64)
	if err != nil {
		return 0, err
	}
	return temperatureInt, nil
}

func getCustomCommands() ([]Temperature, []error) {
	tempsToReturn := []Temperature{}
	errors := []error{}

	for _, command := range customCommandsToCheck {
		if checkExists(command.Command) {
			cmd := exec.Command(command.Command, command.Args...)

			output, err := cmd.Output()
			// Check if cmd not found and set to 0
			if err != nil {
				errors = append(errors, err)
				tempsToReturn = append(tempsToReturn, Temperature{Device: command.ThermalType, Temp: 0})
				continue
			}
			temperature, err := extractTemperature(string(output), command.Regex)
			if err != nil {
				errors = append(errors, err)
				tempsToReturn = append(tempsToReturn, Temperature{Device: command.ThermalType, Temp: 0})
				continue
			}
			// Lowercase the thermaltype
			command.ThermalType = strings.ToLower(command.ThermalType)
			tempsToReturn = append(tempsToReturn, Temperature{Device: command.ThermalType, Temp: temperature})
		}
	}
	return tempsToReturn, errors
}

func getThermalZones() ([]Temperature, []error) {
	// We need to get `temp` and `type` from each thermal zone
	// We will iterate every /sys/class/thermal/thermal_zone#/ directory

	tempsToReturn := []Temperature{}
	errors := []error{}

	zones, err := os.ReadDir("/sys/class/thermal/")
	if err != nil {
		errors = append(errors, err)
		return tempsToReturn, errors
	}

	for _, zone := range zones {
		log.Debugf("Maybe found thermal zone: %s", zone.Name())

		// Check if the zone is a thermal zone
		if !strings.HasPrefix(zone.Name(), "thermal_zone") {
			log.Debugf("! HasPrefix(thermal_zone): %s", zone.Name())
			continue
		}

		// Check if the zone has a type
		typeFile, err := os.ReadFile(fmt.Sprintf("/sys/class/thermal/%s/type", zone.Name()))
		if err != nil {
			errors = append(errors, err)
			continue
		}

		deviceType := strings.TrimSpace(string(typeFile))
		// Lowercase the device type
		deviceType = strings.ToLower(deviceType)
		// Remove "-thermal" and "-therm" from the end of the string
		deviceType = strings.TrimSuffix(deviceType, "-thermal")
		deviceType = strings.TrimSuffix(deviceType, "-therm")

		// Check if the zone has a temp
		tempFile, err := ioutil.ReadFile(fmt.Sprintf("/sys/class/thermal/%s/temp", zone.Name()))
		if err != nil {
			errors = append(errors, err)
			continue
		}
		temperature := strings.TrimSpace(string(tempFile))
		temperature = temperature[:len(temperature)-3] + "." + temperature[len(temperature)-3:]
		temperatureInt, err := strconv.ParseFloat(temperature, 64)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		tempsToReturn = append(tempsToReturn, Temperature{Device: deviceType, Temp: temperatureInt})

	}

	return tempsToReturn, errors
}

func handler(w http.ResponseWriter, r *http.Request) {

	tempsTmp, err := getCustomCommands()
	// check if err is an empty list
	if len(err) > 0 {
		for _, e := range err {
			log.Errorf("Failed to get custom commands: %s", e)
		}
	}
	for _, temp := range tempsTmp {
		log.Debugf("Found temperature: %s: %f", temp.Device, temp.Temp)

		device := strings.ToLower(temp.Device)
		device = strings.ReplaceAll(device, " ", "_")
		device = strings.ReplaceAll(device, "-", "_")

		fmt.Fprintf(w, "%s_temperature{device=\"%s\"} %f\n", device, temp.Device, temp.Temp)
	}

	tempsTmp, err = getThermalZones()
	if len(err) > 0 {
		for _, e := range err {
			log.Errorf("Failed to get custom commands: %s", e)
		}
	}
	for _, temp := range tempsTmp {
		log.Debugf("Found temperature: %s: %f", temp.Device, temp.Temp)

		device := strings.ToLower(temp.Device)
		device = strings.ReplaceAll(device, " ", "_")
		device = strings.ReplaceAll(device, "-", "_")

		fmt.Fprintf(w, "%s_temperature{device=\"%s\"} %f\n", device, temp.Device, temp.Temp)
	}

	fmt.Fprintf(w, "version{app=\"%s\", build_time=\"%s\"}", VERSION, BUILDTIME)
	log.Infof("%s metrics served successfully", r.RemoteAddr)
}

func main() {
	// get our logger going
	log = *logrus.New()
	log.SetLevel(logrus.InfoLevel)

	logrus_hook, err := lSyslog.NewSyslogHook("", "", syslog.LOG_EMERG|syslog.LOG_ALERT|syslog.LOG_CRIT|syslog.LOG_ERR|syslog.LOG_WARNING|syslog.LOG_NOTICE|syslog.LOG_INFO|syslog.LOG_DEBUG, "")
	if err == nil {
		// Only disable Timestamps for syslog
		log.SetFormatter(&logrus.TextFormatter{
			DisableTimestamp: true,
		})
		log.Hooks.Add(logrus_hook)
	} else {
		log.Error(fmt.Sprintf("failed to configure logger: %s", err.Error()))
	}

	config = Config{
		LogLevel:        "info",
		LogDestination:  "syslog",
		LogFilename:     "/var/log/temperature-exporter.log",
		HttpBindAddress: "0.0.0.0",
		HttpPort:        9101,
	}

	if checkExists("/etc/temperature-exporter/config.json") {
		log.Infof("Found config file at /etc/temperature-exporter/config.json")
		// Read the file
		file, err := os.ReadFile("/etc/temperature-exporter/config.json")
		if err != nil {
			log.Errorf("Failed to read config file: %s", err)
			syscall.Exit(1)
		}
		// Unmarshal the file
		err = json.Unmarshal(file, &config)
		if err != nil {
			log.Errorf("Failed to unmarshal config file: %s", err)
			syscall.Exit(2)
		}
	}

	/*
		 _                   _
		| | ___   __ _  __ _(_)_ __   __ _
		| |/ _ \ / _` |/ _` | | '_ \ / _` |
		| | (_) | (_| | (_| | | | | | (_| |
		|_|\___/ \__, |\__, |_|_| |_|\__, |
		         |___/ |___/         |___/
	*/
	switch config.LogLevel {
	case "trace":
		log.SetLevel(logrus.TraceLevel)
	case "debug":
		log.SetLevel(logrus.DebugLevel)
	case "info":
		log.SetLevel(logrus.InfoLevel)
	case "warn":
		log.SetLevel(logrus.WarnLevel)
	case "error":
		log.SetLevel(logrus.ErrorLevel)
	case "fatal":
		log.SetLevel(logrus.FatalLevel)
	case "panic":
		log.SetLevel(logrus.PanicLevel)
	default:
		log.SetLevel(logrus.WarnLevel)
	}

	log.Infof("Temperature Exporter github.com/neldridge/temperature-exporter v%s (built: %s)", VERSION, BUILDTIME)

	board = determineBoard()

	if board == "raspberrypi" {
		log.Infof("Detected Raspberry Pi")
	} else if board == "tegra" {
		log.Infof("Detected Tegra based board")
	} else {
		log.Warnf("Unknown board")
	}

	http.HandleFunc("/metrics", handler)
	log.Infof("Server listening on %s:%d", config.HttpBindAddress, config.HttpPort)
	err = http.ListenAndServe(fmt.Sprintf("%s:%d", config.HttpBindAddress, config.HttpPort), nil)
	if err != nil {
		log.Fatalf("Failed to start server: %s", err)
	}

}
