package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	versionRegexp = regexp.MustCompile(`^v[\d]+`)
)

type rawGVK struct {
	KeepLabels       []string `mapstructure:"keep-labels"`
	KeepAnnotations  []string `mapstructure:"keep-annotations"`
	KeepPaths        []string `mapstructure:"keep-paths"`
	IgnorePaths      []string `mapstructure:"ignore-paths"`
	IgnoreNames      []string `mapstructure:"ignore-names"`
	PathValueFilters []string `mapstructure:"path-value-filters"`
}

type GVK struct {
	KeepLabels       []*regexp.Regexp
	KeepAnnotations  []*regexp.Regexp
	KeepPaths        []string
	IgnorePaths      []string
	IgnoreNames      []*regexp.Regexp
	PathValueFilters map[string]*regexp.Regexp
	GroupVersionKind schema.GroupVersionKind
}

func keyOrDie(key string) {
	if !viper.IsSet(key) {
		panic(fmt.Sprintf("%q is required", key))
	}
}

func ReadString(key, def string) string {
	s := viper.GetString(key)
	if s != "" {
		return s
	}
	return def
}

func ReadStringOrDie(key string) string {
	keyOrDie(key)
	return viper.GetString(key)
}

func ReadStringSliceOrDie(key string) []string {
	keyOrDie(key)
	return viper.GetStringSlice(key)
}

func ReadGVKOrDie() map[string]*GVK {
	const key = "gvk"
	rawGVKConfigs := map[string]*rawGVK{}
	err := viper.UnmarshalKey(key, &rawGVKConfigs)
	if err != nil {
		panic(errors.Wrapf(err, "failed to unmarshal rawGVK"))
	}
	if len(rawGVKConfigs) == 0 {
		panic(fmt.Sprintf("%q is required", key))
	}
	gvkConfigs := map[string]*GVK{}
	for k, v := range rawGVKConfigs {
		gvkConfig := &GVK{
			KeepAnnotations:  readRawRegexesOrDie(v.KeepAnnotations),
			KeepLabels:       readRawRegexesOrDie(v.KeepLabels),
			IgnoreNames:      readRawRegexesOrDie(v.IgnoreNames),
			KeepPaths:        v.KeepPaths,
			IgnorePaths:      v.IgnorePaths,
			PathValueFilters: readPathValueFiltersOrDie(v.PathValueFilters),
		}
		group, version, kind := parseGVKString(k)
		gvkConfig.GroupVersionKind = schema.GroupVersionKind{group, version, kind}
		gvkConfigs[k] = gvkConfig
	}
	return gvkConfigs
}

func parseGVKString(gvkString string) (string, string, string) {
	// kind.version.group
	tokens := strings.SplitN(gvkString, ".", 2)
	if len(tokens) == 1 {
		// kind only
		return "", "", tokens[0]
	}
	vkTokens := strings.SplitN(tokens[1], ".", 2)
	if len(vkTokens) == 1 {
		return tokens[1], "", tokens[0]
	}
	if versionRegexp.MatchString(vkTokens[0]) {
		return vkTokens[1], vkTokens[0], tokens[0]
	}
	// doesn't look like a version; assume the whole thing is a group
	return tokens[1], "", tokens[0]
}

func readPathValueFiltersOrDie(raw []string) map[string]*regexp.Regexp {
	m := make(map[string]*regexp.Regexp)
	for _, rawPathValueFilter := range raw {
		tokens := strings.Split(rawPathValueFilter, "=")
		if len(tokens) != 2 {
			panic(fmt.Sprintf("unexpected number of tokens in %q", rawPathValueFilter))
		}
		m[tokens[0]] = regexp.MustCompile(tokens[1])
	}
	return m
}

func readRawRegexesOrDie(rawRegexes []string) []*regexp.Regexp {
	var regexes []*regexp.Regexp
	for _, rawRegexes := range rawRegexes {
		regexes = append(regexes, regexp.MustCompile(rawRegexes))
	}
	return regexes
}
