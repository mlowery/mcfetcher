package dynamic

import (
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

type Client struct {
	restMapper meta.RESTMapper
	client     dynamic.Interface
}

func New(config *restclient.Config) (*Client, error) {
	config.Timeout = 5 * time.Minute
	restMapper, err := newDiscoveryRESTMapper(config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get discovery rest mapper")
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get create dynamic client")
	}
	return &Client{
		restMapper: restMapper,
		client:     dynamicClient,
	}, nil
}

func newDiscoveryRESTMapper(c *rest.Config) (meta.RESTMapper, error) {
	// Get a mapper
	dc, err := discovery.NewDiscoveryClientForConfig(c)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create discovery client")
	}
	gr, err := restmapper.GetAPIGroupResources(dc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get api group resources")
	}
	return restmapper.NewDiscoveryRESTMapper(gr), nil
}

func (c *Client) GetResourceInterface(gvk schema.GroupVersionKind, ns string) (dynamic.ResourceInterface, error) {
	mapping, err := c.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get rest mapping")
	}
	if mapping.Scope.Name() == meta.RESTScopeNameRoot {
		return c.client.Resource(mapping.Resource), nil
	}
	return c.client.Resource(mapping.Resource).Namespace(ns), nil
}
