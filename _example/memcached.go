package main

import (
	"bufio"
	"flag"
	"fmt"
	plugin "github.com/gorpher/go-idpc-plugin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
)

var graphdef map[string]plugin.Graphs = map[string]plugin.Graphs{
	"memcached.connections": {
		Label: "Memcached Connections",
		Unit:  "integer",
		Metrics: []plugin.Metrics{
			{Name: "curr_connections", Label: "Connections", Diff: false},
		},
	},
	"memcached.cmd": {
		Label: "Memcached Command",
		Unit:  "integer",
		Metrics: []plugin.Metrics{
			{Name: "cmd_get", Label: "Get", Diff: true},
			{Name: "cmd_set", Label: "Set", Diff: true},
			{Name: "cmd_flush", Label: "Flush", Diff: true},
			{Name: "cmd_touch", Label: "Touch", Diff: true},
		},
	},
	"memcached.hitmiss": {
		Label: "Memcached Hits/Misses",
		Unit:  "integer",
		Metrics: []plugin.Metrics{
			{Name: "get_hits", Label: "Get Hits", Diff: true},
			{Name: "get_misses", Label: "Get Misses", Diff: true},
			{Name: "delete_hits", Label: "Delete Hits", Diff: true},
			{Name: "delete_misses", Label: "Delete Misses", Diff: true},
			{Name: "incr_hits", Label: "Incr Hits", Diff: true},
			{Name: "incr_misses", Label: "Incr Misses", Diff: true},
			{Name: "cas_hits", Label: "Cas Hits", Diff: true},
			{Name: "cas_misses", Label: "Cas Misses", Diff: true},
			{Name: "touch_hits", Label: "Touch Hits", Diff: true},
			{Name: "touch_misses", Label: "Touch Misses", Diff: true},
		},
	},
	"memcached.evictions": {
		Label: "Memcached Evictions",
		Unit:  "integer",
		Metrics: []plugin.Metrics{
			{Name: "evictions", Label: "Evictions", Diff: true},
		},
	},
	"memcached.unfetched": {
		Label: "Memcached Unfetched",
		Unit:  "integer",
		Metrics: []plugin.Metrics{
			{Name: "expired_unfetched", Label: "Expired unfetched", Diff: true},
			{Name: "evicted_unfetched", Label: "Evicted unfetched", Diff: true},
		},
	},
	"memcached.rusage": {
		Label: "Memcached Resouce Usage",
		Unit:  "float",
		Metrics: []plugin.Metrics{
			{Name: "rusage_user", Label: "User", Diff: true},
			{Name: "rusage_system", Label: "System", Diff: true},
		},
	},
	"memcached.bytes": {
		Label: "Memcached Traffics",
		Unit:  "bytes",
		Metrics: []plugin.Metrics{
			{Name: "bytes_read", Label: "Read", Diff: true},
			{Name: "bytes_written", Label: "Write", Diff: true},
		},
	},
}

type MemcachedPlugin struct {
	plugin.MetricsPlugin
	Key      string
	Target   string
	TempFile string
}

var (
	Revision  = "untracked"
	Version   = "v1.0.0"
	GOARCH    = runtime.GOARCH
	GOOS      = runtime.GOOS
	GOVersion = runtime.Version()
)

func (m MemcachedPlugin) Meta() plugin.Meta {
	if m.Key == "" {
		m.Key = "memcached"
	}
	version, _ := plugin.ParseVersion(Version)
	return plugin.Meta{
		Key:       m.Key,
		Type:      plugin.TypeMetrics,
		Version:   version,
		Revision:  Revision,
		GOARCH:    GOARCH,
		GOOS:      GOOS,
		GOVersion: GOVersion,
	}
}

func (m MemcachedPlugin) Metrics() (map[string]interface{}, error) {
	conn, err := net.Dial("tcp", m.Target)
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(conn, "stats")
	scanner := bufio.NewScanner(conn)
	stat := make(map[string]interface{})

	for scanner.Scan() {
		line := scanner.Text()
		s := string(line)
		if s == "END" {
			return stat, nil
		}

		res := strings.Split(s, " ")
		if res[0] == "STAT" {
			stat[res[1]], err = strconv.ParseFloat(res[2], 64)
			if err != nil {
				log.Error().Err(err).Msg("FetchMetrics:")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return stat, err
	}
	return nil, nil
}

func (m MemcachedPlugin) GraphDefinition() map[string]plugin.Graphs {
	return graphdef
}

func main() {
	optHost := flag.String("host", "localhost", "Hostname")
	optPort := flag.String("port", "11211", "Port")
	optTempFile := flag.String("tempFile", "", "Temp file name")
	v := flag.Bool("v", false, "version")
	if os.Getenv(plugin.PLUGIN_PREFIX+"DEBUG") != "" {
		log.Logger.Level(zerolog.DebugLevel)
	} else {
		log.Logger.Level(zerolog.FatalLevel)
	}
	flag.Parse()

	var memcached MemcachedPlugin

	memcached.Target = fmt.Sprintf("%s:%s", *optHost, *optPort)
	helper := plugin.NewIdpcPlugin(memcached)
	helper.TempFile = *optTempFile
	if *v {
		fmt.Println(helper.Version())
		return
	}
	helper.Run()
}
