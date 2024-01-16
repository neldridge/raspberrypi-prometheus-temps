# Temperature Exporter
The intent of this project is to provide GPU/CPU and any other relevant temperatures for devices to be exported on `/metrics` for consumption within Prometheus

# Raspberry PI OS
Tested with Raspberry PI 4/5 and CM4, GPU attained from `vcgencmd measure_temp` and any other temp (cpu) from `/sys/class/thermal/thermal_zone?/{temp,type}`

## Jetson TX Temperatures
Essentially, these should be covered by iterating every `/sys/class/thermal/thermal_zone?/{temp,type}`

Relevant info:
* https://docs.nvidia.com/jetson/archives/r34.1/DeveloperGuide/text/SD/PlatformPowerAndPerformance/JetsonOrinNxSeriesAndJetsonAgxOrinSeries.html#bsp-specific-thermal-zones
* https://forums.developer.nvidia.com/t/use-c-language-get-cpu-thermal-sensor-temperature-tx1/52275/3
* https://forums.developer.nvidia.com/t/thermal-zone4-reports-100-degree-celcius/44422/6
