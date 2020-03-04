package fetch

import (
	ctx "context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/pager"

	dynamic2 "github.com/mlowery/mcfetcher/pkg/client/dynamic"
	"github.com/mlowery/mcfetcher/pkg/config"
	oerrors "github.com/mlowery/mcfetcher/pkg/errors"
	"github.com/mlowery/mcfetcher/pkg/util"
)

var (
	workDir    string
	gvkConfigs map[string]*config.GVK
)

var Cmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch, filter, and sanitize objects across Kubernetes clusters.",
	Run: func(cmd *cobra.Command, args []string) {
		logger, dFunc := util.NewLogger()
		defer dFunc()
		defer func(start time.Time) {
			logger.Infow("done", "totalDuration", time.Since(start))
		}(time.Now())

		gvkConfigs = config.ReadGVKOrDie()

		var err error
		workDir, err = util.EnsureWorkDir()
		if err != nil {
			logger.Fatalf("failed to ensure work dir: %v", err)
		}

		contexts := config.ReadStringSliceOrDie("kubeconfig-contexts")

		var wg sync.WaitGroup
		contextCh := make(chan string)
		errCh := make(chan error, len(contexts))
		var errorCount int

		go func() {
			for err := range errCh {
				errorCount++
				logger.Errorw(err.Error())
			}
		}()

		for w := 1; w <= 10; w++ {
			wg.Add(1)
			go worker(logger.With("worker", w), contextCh, errCh, &wg)
		}

		for i, context := range contexts {
			logger.Infow("queuing context", "context", context,
				"progress", fmt.Sprintf("%d/%d", i+1, len(contexts)))
			contextCh <- context
		}
		close(contextCh)

		logger.Infow("all contexts queued; waiting for workers to finish")
		wg.Wait()

		close(errCh)

		if errorCount > 0 {
			logger.Infof("failed with %d errors (written to stderr)", errorCount)
			os.Exit(1)
		}
	},
}

func worker(l *zap.SugaredLogger, contextCh <-chan string, errCh chan<- error, wg *sync.WaitGroup) {
	for context := range contextCh {
		logger := l.With("context", context)
		logger.Infow("processing context")

		for gvkString, gvkConfig := range gvkConfigs {
			logger = logger.With("gvk", gvkString)
			// if there is a file in the work-dir, don't call Kube since that is the most expensive part
			filename := util.MkCacheFilename(workDir, context, gvkString, "json")

			rawObjects, err := util.ReadRawObjects(filename)
			if err != nil {
				errCh <- oerrors.New(err, "failed to read cached records",
					"gvk", gvkString, "context", context, "filename", filename)
				continue
			}
			if rawObjects == nil {
				restConfig, err := getClientConfig(context, config.ReadString("kubeconfig", util.InHomeDirOrDie(".kube/config"))).ClientConfig()
				if err != nil {
					errCh <- oerrors.New(err, "failed to get rest config",
						"gvk", gvkString, "context", context)
					continue
				}
				client, err := dynamic2.New(restConfig)
				if err != nil {
					errCh <- oerrors.New(err, "failed to create client (is proxy configured correctly?)",
						"gvk", gvkString, "context", context)
					continue
				}

				logger.Infow("calling Kube to fetch all")
				// use pager to avoid: Stream error http2.StreamError{StreamID:0x5, Code:0x2, Cause:error(nil)} when
				// reading response body, may be caused by closed connection. Please retry.
				objPager := pager.New(pager.SimplePageFunc(func(opts metav1.ListOptions) (runtime.Object, error) {
					r, err := client.GetResourceInterface(gvkConfig.GroupVersionKind, metav1.NamespaceAll)
					if err != nil {
						return nil, errors.Wrapf(err, "failed to get resource interface")
					}
					return r.List(opts)
				}))
				start := time.Now()
				rawList, _, err := objPager.List(ctx.TODO(), metav1.ListOptions{})
				rtt := time.Since(start)
				if err != nil {
					oerrors.New(err, "failed to list",
						"gvk", gvkString, "context", context)
					continue
				}
				logger.Infow("called Kube", "duration", rtt)
				rawObjects, err := extractList(logger, errCh, rawList, gvkConfig)
				if err != nil {
					oerrors.New(err, "failed to extract list",
						"gvk", gvkString, "context", context)
					continue
				}
				logger.Infow("caching", "filename", filename, "objCount", len(rawObjects))
				err = util.WriteRawObjects(filename, rawObjects)
				if err != nil {
					oerrors.New(err, "failed to write records",
						"gvk", gvkString, "context", context)
					continue
				}

			} else {
				logger.Infow("using cached results (to skip cache, delete file)",
					"filename", filename, "objCount", len(rawObjects))
			}
		}
	}
	wg.Done()
}

func extractList(logger *zap.SugaredLogger, errCh chan<- error, object runtime.Object,
	gvkConfig *config.GVK) ([]*unstructured.Unstructured, error) {
	items, err := meta.ExtractList(object)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to extract list")
	}
	var l []*unstructured.Unstructured
	for _, item := range items {
		uObj := item.(*unstructured.Unstructured)
		sanObj, err := util.Sanitize(logger, uObj, gvkConfig.IgnoreNames, gvkConfig.PathValueFilters,
			gvkConfig.KeepAnnotations, gvkConfig.KeepLabels, gvkConfig.KeepPaths, gvkConfig.IgnorePaths)
		if err != nil {
			errCh <- oerrors.New(err, "failed to sanitize",
				"gvk", uObj.GetObjectKind().GroupVersionKind().String(), "name", uObj.GetName())
			continue
		}
		if sanObj != nil {
			l = append(l, sanObj)
		}
	}
	return l, nil
}

func getClientConfig(context, kubeconfig string) clientcmd.ClientConfig {
	pathOptions := clientcmd.NewDefaultPathOptions()
	loadingRules := *pathOptions.LoadingRules
	loadingRules.Precedence = pathOptions.GetLoadingPrecedence()
	loadingRules.ExplicitPath = kubeconfig
	overrides := &clientcmd.ConfigOverrides{
		CurrentContext: context,
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(&loadingRules, overrides)
}

func init() {
	Cmd.PersistentFlags().String("kubeconfig", "", "kubeconfig")
	viper.BindPFlag("kubeconfig", Cmd.PersistentFlags().Lookup("kubeconfig"))

	Cmd.PersistentFlags().StringSlice("kubeconfig-contexts", []string{}, "kubeconfig-contexts")
	viper.BindPFlag("kubeconfig-contexts", Cmd.PersistentFlags().Lookup("kubeconfig-contexts"))
}
