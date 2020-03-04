# mcfetcher: multi-cluster fetcher

Fetch, filter, and sanitize objects across Kubernetes clusters in parallel.

## Running

```sh
$ mcfetcher --config=config.toml --kubeconfig-contexts=cluster1,cluster2,cluster3 fetch
```
