# mcfetcher: multi-cluster fetcher

Fetch, filter, and sanitize objects across Kubernetes clusters in parallel.

## Building

```sh
$ go build -o mcfetcher ./main.go
```

## Running

```sh
$ mcfetcher --config=config.toml --kubeconfig-contexts=cluster1,cluster2,cluster3 fetch
```
