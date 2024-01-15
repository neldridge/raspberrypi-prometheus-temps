package main

import (
	"fmt"
	"log/syslog"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	logrus "github.com/sirupsen/logrus"
	lSyslog "github.com/sirupsen/logrus/hooks/syslog"
)

var log logrus.Logger
var config Config
var (
	VERSION   string = "undefined"
	BUILDTIME string = "manual"
)

type Config struct {
	LogLevel       string
	LogDestination string
	LogFilename    string
}

func getGPUTemperature() (float64, error) {
	cmd := exec.Command("vcgencmd", "measure_temp")
	output, err := cmd.Output()
	// Check if cmd not found and set to 0
	if err != nil {
		return 0, err
	}
	if strings.Contains(string(output), "not found") {
		return 0, nil
	}
	temperature := strings.TrimPrefix(string(output), "temp=")
	temperature = strings.TrimSpace(strings.TrimSuffix(temperature, "'C\n"))
	// convert to int32
	temperatureInt, err := strconv.ParseFloat(temperature, 64)
	if err != nil {
		return 0, err
	}
	return temperatureInt, nil
}

func getCPUTemperature() (float64, error) {
	content, err := exec.Command("cat", "/sys/class/thermal/thermal_zone0/temp").Output()
	if err != nil {
		return 0, err
	}
	temperature := strings.TrimSpace(string(content))
	temperature = temperature[:len(temperature)-3] + "." + temperature[len(temperature)-3:]
	temperatureInt, err := strconv.ParseFloat(temperature, 64)
	if err != nil {
		return 0, err
	}
	return temperatureInt, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	gpuTemp, err := getGPUTemperature()
	if err != nil {
		// http.Error(w, fmt.Sprintf("Failed to get GPU temperature: %s", err), http.StatusInternalServerError)
		log.Errorf("Failed to get GPU temperature: %s", err)
		gpuTemp = 0
	}

	cpuTemp, err := getCPUTemperature()
	if err != nil {
		// http.Error(w, fmt.Sprintf("Failed to get CPU temperature: %s", err), http.StatusInternalServerError)
		log.Errorf("Failed to get CPU temperature: %s", err)
		cpuTemp = 0
	}

	fmt.Fprintf(w, "gpu_temperature{device=\"gpu\"} %f\ncpu_temperature{device=\"cpu\"} %f\nversion{app=\"%s\", build_time=\"%s\"}", gpuTemp, cpuTemp, VERSION, BUILDTIME)
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

	/*
		 _                   _
		| | ___   __ _  __ _(_)_ __   __ _
		| |/ _ \ / _` |/ _` | | '_ \ / _` |
		| | (_) | (_| | (_| | | | | | (_| |
		|_|\___/ \__, |\__, |_|_| |_|\__, |
		         |___/ |___/         |___/
	*/
	config.LogLevel = "debug"
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

	http.HandleFunc("/metrics", handler)
	log.Infof("Server listening on :8080")
	http.ListenAndServe(":8080", nil)
}
