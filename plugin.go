package plugin

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rs/zerolog/log"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Metric units
const (
	UnitFloat          = "float"
	UnitInteger        = "integer"
	UnitPercentage     = "percentage"
	UnitBytes          = "bytes"
	UnitBytesPerSecond = "bytes/sec"
	UnitIOPS           = "iops"
)

// PluginVersionRegex ex.) idpc-plugin-redis-metrics version 0.0.1 (rev dev) [windows amd64 go1.16.5]

var PluginVersionRegex = regexp.MustCompile(`^\s*idpc-plugin-(\w+)-(checker|metrics|metadata)\s+version\s+(\d{1,3}\.\d{1,3}\.\d{1,3})\s+\(rev\s+(\w+)\)\s+\[(\w+)\s+(\w+)\s+(.+)]`)

func ParseVersionCommand(s string) Meta {
	details := PluginVersionRegex.FindStringSubmatch(s)
	if len(details) != 8 {
		return Meta{}
	}
	version, err := ParseVersion(details[3])
	if err != nil {
		return Meta{}
	}
	return Meta{
		Key:       details[1],
		Type:      Type(details[2]),
		Version:   version,
		Revision:  details[4],
		GOOS:      details[5],
		GOARCH:    details[6],
		GOVersion: details[7],
	}
}

// Metrics represents definition of a metric
type Metrics struct {
	Name         string  `json:"name"`
	Label        string  `json:"label"`
	Diff         bool    `json:"-"`
	Type         string  `json:"-"`
	Stacked      bool    `json:"stacked"`
	Scale        float64 `json:"-"`
	AbsoluteName bool    `json:"-"`
}

// Graphs represents definition of a graph
type Graphs struct {
	Label   string    `json:"label"`
	Unit    string    `json:"unit"`
	Metrics []Metrics `json:"metrics"`
}

type PluginValues struct {
	Values    map[string]interface{}
	Timestamp time.Time
}

type Version struct {
	Major, Minor, Patch uint32
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func ParseVersion(s string) (Version, error) {
	versionSplit := strings.SplitN(s, ".", 3)
	if len(versionSplit) < 3 {
		return Version{}, fmt.Errorf("expected Major.Minor.Patch in %q", s)
	}
	ver := Version{}
	for i, v := range []*uint32{&ver.Major, &ver.Minor, &ver.Patch} {
		var n64 uint64
		var err error
		n64, err = strconv.ParseUint(versionSplit[i], 10, 32)
		if err != nil {
			return Version{}, err
		}
		*v = uint32(n64)
	}
	return ver, nil
}

// LessThan determines whether the version is older than another version.
func (v Version) LessThan(other Version) bool {
	switch {
	case v.Major < other.Major:
		return true
	case v.Major > other.Major:
		return false

	case v.Minor < other.Minor:
		return true
	case v.Minor > other.Minor:
		return false

	case v.Patch < other.Patch:
		return true
	case v.Patch > other.Patch:
		return false
	default:
		// this should only be reachable when versions are equal
		return false
	}
}

type Type string

const (
	TypeChecker  Type = "checker"
	TypeMetrics  Type = "metrics"
	TypeMetadata Type = "metadata"
)

const PLUGIN_PREFIX = "idpc-plugin"

type Meta struct {
	Key       string
	Type      Type
	Version   Version
	Revision  string
	GOOS      string
	GOARCH    string
	GOVersion string
}

func (b Meta) String() string {
	return fmt.Sprintf("%s-%s-%s version %s (rev %s) [%s %s %s]",
		PLUGIN_PREFIX, b.Key, b.Type, b.Version, b.Revision, b.GOOS, b.GOARCH, b.GOVersion)
}
func (b Meta) Name() string {
	return PLUGIN_PREFIX + "-" + b.Key + "-" + string(b.Type)
}

type Plugin interface {
	Meta() Meta
}

type MetricsPlugin interface {
	Plugin
	Metrics() (map[string]interface{}, error)
	GraphDefinition() map[string]Graphs
}

type CheckerPlugin interface {
	Plugin
	Checker() (message, status string)
}

type MetadataPlugin interface {
	Plugin
	Metadata() (map[string]interface{}, error)
}

type IdpcPlugin struct {
	Plugin
	PluginRunner
	TempFile string
}

type PluginRunner interface {
	Run()
	OutputMeta()
	OutputValues()
	OutputMetricsValues()
	OutputCheckerValues()
	OutputMetadataValues()
}

func NewIdpcPlugin(plugin Plugin) IdpcPlugin {
	mp := IdpcPlugin{Plugin: plugin}
	return mp
}

func (h *IdpcPlugin) printValue(w io.Writer, key string, value interface{}, now time.Time) {
	switch v := value.(type) {
	case uint32:
		fmt.Fprintf(w, "%s\t%d\t%d\n", key, v, now.Unix())
	case uint64:
		fmt.Fprintf(w, "%s\t%d\t%d\n", key, v, now.Unix())
	case float64:
		if math.IsNaN(value.(float64)) || math.IsInf(v, 0) {
			log.Printf("Invalid value: key = %s, value = %f\n", key, value)
		} else {
			fmt.Fprintf(w, "%s\t%f\t%d\n", key, v, now.Unix())
		}
	}
}

// LoadLastValues 从缓存文件中加载插件数据，插件数据为Metadata数据或者Metrics数据
func (h *IdpcPlugin) LoadLastValues() (values PluginValues, err error) {
	f, err := os.Open(h.tempFilename())
	if err != nil {
		if os.IsNotExist(err) {
			return values, nil
		}
		return
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	err = decoder.Decode(&values.Values)
	if err != nil {
		return
	}
	switch v := values.Values["_lastTime"].(type) {
	case float64:
		values.Timestamp = time.Unix(int64(v), 0)
	case int64:
		values.Timestamp = time.Unix(v, 0)
	}
	return
}

var errStateUpdated = errors.New("state was recently updated")

func (h *IdpcPlugin) loadLastValuesSafe(now time.Time) (m PluginValues, err error) {
	m, err = h.LoadLastValues()
	if err != nil {
		return m, err
	}
	if now.Sub(m.Timestamp) < time.Second {
		return m, errStateUpdated
	}
	return m, nil
}

// SaveValues 保存插件数据
func (h *IdpcPlugin) SaveValues(values PluginValues) error {
	f, err := os.Create(h.tempFilename())
	if err != nil {
		return err
	}
	defer f.Close()

	values.Values["_lastTime"] = values.Timestamp.Unix()
	encoder := json.NewEncoder(f)
	err = encoder.Encode(values.Values)
	if err != nil {
		return err
	}
	return nil
}

func (h *IdpcPlugin) calcDiff(value float64, now time.Time, lastValue float64, lastTime time.Time) (float64, error) {
	diffTime := now.Unix() - lastTime.Unix()
	if diffTime > 600 {
		return 0, errors.New("too long duration")
	}

	diff := (value - lastValue) * 60 / float64(diffTime)

	if lastValue <= value {
		return diff, nil
	}
	return 0.0, errors.New("counter seems to be reset")
}

func (h *IdpcPlugin) calcDiffUint32(value uint32, now time.Time, lastValue uint32, lastTime time.Time, lastDiff float64) (float64, error) {
	diffTime := now.Unix() - lastTime.Unix()
	if diffTime > 600 {
		return 0, errors.New("too long duration")
	}

	diff := float64((value-lastValue)*60) / float64(diffTime)

	if lastValue <= value || diff < lastDiff*10 {
		return diff, nil
	}
	return 0.0, errors.New("counter seems to be reset")

}

func (h *IdpcPlugin) calcDiffUint64(value uint64, now time.Time, lastValue uint64, lastTime time.Time, lastDiff float64) (float64, error) {
	diffTime := now.Unix() - lastTime.Unix()
	if diffTime > 600 {
		return 0, errors.New("too long duration")
	}

	diff := float64((value-lastValue)*60) / float64(diffTime)

	if lastValue <= value || diff < lastDiff*10 {
		return diff, nil
	}
	return 0.0, errors.New("counter seems to be reset")
}

func (h *IdpcPlugin) tempFilename() string {
	if h.TempFile == "" {
		args := os.Args
		meta := h.Plugin.Meta()
		filename := fmt.Sprintf(
			"%s-%s-%s-%x", PLUGIN_PREFIX, meta.Key, meta.Type,
			// When command-line options are different, mostly different metrics.
			// e.g. `-host` and `-port` options for mackerel-plugin-mysql
			sha1.Sum([]byte(strings.Join(args[1:], " "))),
		)
		h.TempFile = filepath.Join(PluginWorkDir(), filename)
	}
	return h.TempFile
}

const (
	metricTypeUint32 = "uint32"
	metricTypeUint64 = "uint64"
	// metricTypeFloat  = "float64"
)

func (h *IdpcPlugin) formatValues(prefix string, metric Metrics, metricValues PluginValues, lastMetricValues PluginValues) {
	name := metric.Name
	if metric.AbsoluteName && len(prefix) > 0 {
		name = prefix + "." + name
	}
	value, ok := metricValues.Values[name]
	if !ok || value == nil {
		return
	}

	var err error
	if v, ok := value.(string); ok {
		switch metric.Type {
		case metricTypeUint32:
			value, err = strconv.ParseUint(v, 10, 32)
		case metricTypeUint64:
			value, err = strconv.ParseUint(v, 10, 64)
		default:
			value, err = strconv.ParseFloat(v, 64)
		}
	}
	if err != nil {
		// For keeping compatibility, if each above statement occurred the error,
		// then the value is set to 0 and continue.
		log.Print("Parsing a value: ", err)
	}

	if metric.Diff {
		_, ok := lastMetricValues.Values[name]
		if ok {
			var lastDiff float64
			if lastMetricValues.Values[".last_diff."+name] != nil {
				lastDiff = toFloat64(lastMetricValues.Values[".last_diff."+name])
			}
			var err error
			switch metric.Type {
			case metricTypeUint32:
				value, err = h.calcDiffUint32(toUint32(value), metricValues.Timestamp, toUint32(lastMetricValues.Values[name]), lastMetricValues.Timestamp, lastDiff)
			case metricTypeUint64:
				value, err = h.calcDiffUint64(toUint64(value), metricValues.Timestamp, toUint64(lastMetricValues.Values[name]), lastMetricValues.Timestamp, lastDiff)
			default:
				value, err = h.calcDiff(toFloat64(value), metricValues.Timestamp, toFloat64(lastMetricValues.Values[name]), lastMetricValues.Timestamp)
			}
			if err != nil {
				log.Print("OutputValues: ", err)
				return
			}
			metricValues.Values[".last_diff."+name] = value
		} else {
			log.Printf("%s does not exist at last fetch\n", name)
			return
		}
	}

	if metric.Scale != 0 {
		switch metric.Type {
		case metricTypeUint32:
			value = toUint32(value) * uint32(metric.Scale)
		case metricTypeUint64:
			value = toUint64(value) * uint64(metric.Scale)
		default:
			value = toFloat64(value) * metric.Scale
		}
	}

	var metricNames []string
	metricNames = append(metricNames, h.Plugin.Meta().Key)
	if len(prefix) > 0 {
		metricNames = append(metricNames, prefix)
	}
	metricNames = append(metricNames, metric.Name)
	h.printValue(os.Stdout, strings.Join(metricNames, "."), value, metricValues.Timestamp)
}

func (h *IdpcPlugin) formatValuesWithWildcard(prefix string, metric Metrics, metricValues PluginValues, lastMetricValues PluginValues) {
	regexpStr := `\A` + prefix + "." + metric.Name
	regexpStr = strings.Replace(regexpStr, ".", "\\.", -1)
	regexpStr = strings.Replace(regexpStr, "*", "[-a-zA-Z0-9_]+", -1)
	regexpStr = strings.Replace(regexpStr, "#", "[-a-zA-Z0-9_]+", -1)
	re, err := regexp.Compile(regexpStr)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to compile regexp: ")
	}
	for k := range metricValues.Values {
		if re.MatchString(k) {
			metricEach := metric
			metricEach.Name = k
			h.formatValues("", metricEach, metricValues, lastMetricValues)
		}
	}
}

var PLUGIN_META_ENV_VAR = strings.ReplaceAll(strings.ToUpper(PLUGIN_PREFIX), "-", "_") + "_META"

// Run the plugin
func (h *IdpcPlugin) Run() {
	if os.Getenv(PLUGIN_META_ENV_VAR) != "" {
		h.OutputMeta()
	} else {
		h.OutputValues()
	}
}

func (h *IdpcPlugin) Version() string {
	return h.Plugin.Meta().String()
}

// OutputValues output the metrics
func (h *IdpcPlugin) OutputValues() {
	meta := h.Plugin.Meta()
	switch meta.Type {
	case TypeChecker:
		h.OutputCheckerValues()
	case TypeMetrics:
		h.OutputMetricsValues()
	case TypeMetadata:
		h.OutputMetadataValues()
	}
	return
}

// GraphDef represents graph definitions
type GraphDef struct {
	Graphs map[string]Graphs `json:"graphs"`
}

// OutputMeta 打印输出插件meta信息
func (h *IdpcPlugin) OutputMeta() {
	builder := strings.Builder{}
	builder.WriteString(h.Meta().String())
	builder.WriteString("\n")
	if mp, ok := h.Plugin.(MetricsPlugin); ok {
		graphs := make(map[string]Graphs)
		for key, graph := range mp.GraphDefinition() {
			g := graph
			k := key
			prefix := h.Plugin.Meta().Key
			if k == "" {
				k = prefix
			} else {
				k = prefix + "." + k
			}
			if g.Label == "" {
				g.Label = title(k)
			}
			var metrics []Metrics
			for _, v := range g.Metrics {
				if v.Label == "" {
					v.Label = title(v.Name)
				}
				metrics = append(metrics, v)
			}
			g.Metrics = metrics
			graphs[k] = g
		}
		var graphdef GraphDef
		graphdef.Graphs = graphs
		b, err := json.Marshal(graphdef)
		if err != nil {
			log.Debug().Err(err).Msg("OutputDefinitions: ")
		}
		builder.Write(b)
	}
	fmt.Println(builder.String())
}

func (h *IdpcPlugin) OutputMetricsValues() {
	if mp, ok := h.Plugin.(MetricsPlugin); ok {
		stat, err := mp.Metrics()
		if err != nil {
			log.Fatal().Err(err).Msg("OutputValues: ")
		}
		metricValues := PluginValues{Values: stat, Timestamp: time.Now()}

		lastMetricValues, err := h.loadLastValuesSafe(metricValues.Timestamp)
		if err != nil {
			if err == errStateUpdated {
				log.Debug().Err(err).Msgf("OutputValues: ")
				return
			}
			log.Debug().Err(err).Msgf("FetchLastValues (ignore):")
		}

		for key, graph := range mp.GraphDefinition() {
			for _, metric := range graph.Metrics {
				if strings.ContainsAny(key+metric.Name, "*#") {
					h.formatValuesWithWildcard(key, metric, metricValues, lastMetricValues)
				} else {
					h.formatValues(key, metric, metricValues, lastMetricValues)
				}
			}
		}

		err = h.SaveValues(metricValues)
		if err != nil {
			log.Fatal().Err(err).Msgf("saveValues: ")
		}

	}
}

func (h *IdpcPlugin) OutputCheckerValues() {
	if mp, ok := h.Plugin.(CheckerPlugin); ok {
		mp.Checker()

	}
}

func (h *IdpcPlugin) OutputMetadataValues() {
	if mp, ok := h.Plugin.(MetadataPlugin); ok {
		now := time.Now()
		preMetadata, err := h.loadLastValuesSafe(now)
		if err != nil && errors.Is(err, errStateUpdated) {
			return
		}
		metadata, err := mp.Metadata()
		if err != nil {
			log.Fatal().Err(err).Send()
			return
		}
		err = json.NewEncoder(os.Stdout).Encode(metadata)
		if err != nil {
			log.Fatal().Err(err).Send()
			return
		}
		metadata["_lastTime"] = preMetadata.Values["_lastTime"]
		if !reflect.DeepEqual(preMetadata.Values, metadata) {
			h.SaveValues(PluginValues{
				Values:    metadata,
				Timestamp: now,
			})
		}

	}
}

func toUint32(value interface{}) uint32 {
	switch v := value.(type) {
	case uint32:
		return v
	case uint64:
		return uint32(v)
	case float64:
		return uint32(v)
	case string:
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return 0
		}
		return uint32(n)
	default:
		return 0
	}
}

func toUint64(value interface{}) uint64 {
	switch v := value.(type) {
	case uint32:
		return uint64(v)
	case uint64:
		return v
	case float64:
		return uint64(v)
	case string:
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

func toFloat64(value interface{}) float64 {
	switch v := value.(type) {
	case uint32:
		return float64(v)
	case uint64:
		return float64(v)
	case float64:
		return v
	case string:
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

func title(s string) string {
	r := strings.NewReplacer(".", " ", "_", " ")
	return strings.Title(r.Replace(s))
}
