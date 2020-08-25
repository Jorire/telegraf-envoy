# Telegraf envoy/enphase input

This is an envoy/enphase input plugin for telegraf.
It is mean to be compiled separately from telegraf and used externally with telegraf's execd input plugin.

# Install instruction
`$ git clone git@github.com:`

build the binary

`$ go build`


# Configuration
Plugin can be called from telegraf now using execd using this kind of configuration:

```
[[inputs.execd]]
  command = ["/path/to/telegraf-envoy_binary", "-config", "/path/to/telegraf-envoy-plugin-config.conf", "-poll_interval", "5m]
  signal = "none"
  
# sample output: write metrics to stdout
[[outputs.file]]
  files = ["stdout"]
```

By default, pool_interval is 5m. 

# Plugin Configuration
Envoy plugin can be configured using a specific config file, like:
```
[[inputs.envoy]]
    base_url = "http://envoy/"
  	## Envoy Serial Number in order to get inverters detailled statistics 
  	## (see http://envoy/ to get it !)
  	serial_number = "xxxxxxxxxxxxx"
```

By default, base_url is *http://envoy/*. 