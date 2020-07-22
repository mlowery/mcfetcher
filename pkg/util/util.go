package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func fileExists(p string) (bool, error) {
	if _, err := os.Stat(p); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

func matchesAny(s string, res []*regexp.Regexp) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func EnsureWorkDir(subDirs ...string) (string, error) {
	var err error
	workDir := viper.GetString("work-dir")
	if len(workDir) == 0 {
		return "", errors.Errorf("work-dir is required")
	}
	workDir = filepath.Join(append([]string{workDir}, subDirs...)...)
	workDir, err = filepath.Abs(workDir)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create absolute path")
	}
	err = ensureDir(workDir)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create directory")
	}
	return workDir, nil
}

func ensureDir(dir string) error {
	err := os.MkdirAll(dir, 0751)
	if err != nil {
		return errors.Wrapf(err, "failed to create directory %q", dir)
	}
	return nil
}

func ReadRawObjects(path string) ([]*unstructured.Unstructured, error) {
	exists, err := fileExists(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check for file existence")
	}
	if !exists {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open file")
	}
	defer file.Close()
	b, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read file")
	}
	var l []*unstructured.Unstructured
	err = json.Unmarshal(b, &l)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal existing result")
	}
	return l, nil
}

func WriteRawObjects(path string, rawObjects []*unstructured.Unstructured) error {
	jsonBytes, err := json.Marshal(rawObjects)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal result")
	}
	err = ioutil.WriteFile(path, jsonBytes, 0644)
	if err != nil {
		return errors.Wrapf(err, "failed to write results")
	}
	return nil
}

// NewLogger returns a *zap.SugaredLogger and a func that should be called with defer.
// Warnings and above go to stderr. Everything else goes to stdout. All lines are complete Bytes docs.
func NewLogger() (*zap.SugaredLogger, func()) {
	// copy and paste of zap.NewExample with parts from zap_test.Example_advancedConfiguration
	highPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.WarnLevel
	})
	lowPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl < zapcore.WarnLevel
	})
	consoleDebugging := zapcore.Lock(os.Stdout)
	consoleErrors := zapcore.Lock(os.Stderr)
	encoderCfg := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		NameKey:        "logger",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}

	consoleEncoder := zapcore.NewJSONEncoder((encoderCfg))
	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, consoleErrors, highPriority),
		zapcore.NewCore(consoleEncoder, consoleDebugging, lowPriority),
	)
	l := zap.New(core).WithOptions()

	logger := l.Sugar()
	f := func() {
		logger.Sync()
	}
	return logger, f
}

func genKey(obj *unstructured.Unstructured) string {
	if len(obj.GetNamespace()) > 0 {
		return obj.GetNamespace() + "/" + obj.GetName()
	}
	return obj.GetName()
}

func MkCacheFilename(workDir string, context, gvkString, ext string) (string, error) {
	d := filepath.Join(workDir, gvkString)
	err := ensureDir(d)
	if err != nil {
		return "", errors.Wrapf(err, "failed to ensure directory")
	}
	return filepath.Join(d, fmt.Sprintf("%s.%s", context, ext)), nil
}

func Sanitize(l *zap.SugaredLogger, obj *unstructured.Unstructured, ignoreNames []*regexp.Regexp, pathValueFilters map[string]*regexp.Regexp, keepAnnotations, keepLabels []*regexp.Regexp, keepPaths, ignorePaths []string, keepDeleted bool) (*unstructured.Unstructured, error) {
	key := genKey(obj)
	if matchesAny(key, ignoreNames) {
		return nil, nil
	}
	if obj.GetDeletionTimestamp() != nil && !keepDeleted {
		return nil, nil
	}
	for path, valueRegexp := range pathValueFilters {
		pathSegments := pathToSegments(path)
		v, found, err := unstructured.NestedString(obj.Object, pathSegments...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get nested map (key=%s) (path=%s)", key, path)
		}
		if found {
			if !valueRegexp.MatchString(v) {
				l.Debugf("dropping %s %s (%s=%s)", obj.GetObjectKind().GroupVersionKind(), obj.GetName(), path, v)
				return nil, nil
			}
		} else {
			l.Warnf("failed to find path (key=%s) (path=%s)", key, path)
		}
	}
	sanObj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	sanObj.SetName(obj.GetName())
	sanObj.SetNamespace(obj.GetNamespace())
	sanObj.SetAPIVersion(obj.GetAPIVersion())
	sanObj.SetKind(obj.GetKind())
	if obj.GetAnnotations() != nil {
		sanAnn := map[string]string{}
		for k, v := range obj.GetAnnotations() {
			if matchesAny(k, keepAnnotations) {
				sanAnn[k] = v
			}
		}
		if len(sanAnn) > 0 {
			sanObj.SetAnnotations(sanAnn)
		}
	}
	if obj.GetLabels() != nil {
		sanLab := map[string]string{}
		for k, v := range obj.GetLabels() {
			if matchesAny(k, keepLabels) {
				sanLab[k] = v
			}
		}
		if len(sanLab) > 0 {
			sanObj.SetLabels(sanLab)
		}
	}
	for _, path := range keepPaths {
		pathSegments := pathToSegments(path)
		found, err := copyNestedField(obj.Object, sanObj.Object, pathSegments...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to copy nested field")
		}
		if !found {
			l.Warnf("failed to find path (key=%s) (path=%s)", key, path)
		}
	}
	for _, path := range ignorePaths {
		pathSegments := strings.Split(strings.TrimPrefix(path, "/"), "/")
		unstructured.RemoveNestedField(sanObj.Object, pathSegments...)
	}
	return sanObj, nil
}

func copyNestedField(origObj, newObj map[string]interface{}, fields ...string) (bool, error) {
	v, found, err := unstructured.NestedFieldNoCopy(origObj, fields...)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get nested map")
	}
	if found {
		err = unstructured.SetNestedField(newObj, v, fields...)
		if err != nil {
			return false, errors.Wrapf(err, "failed to set nested map")
		}
	}
	return found, err
}

func pathToSegments(path string) []string {
	return strings.Split(strings.TrimPrefix(path, "/"), "/")
}

func InHomeDirOrDie(path string) string {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}
	return filepath.Join(usr.HomeDir, path)
}
