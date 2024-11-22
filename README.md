# L3

## Concepts

## Building

You can run build and run the binary locally with

```sh
$ go build -o l3
$ l3 --controlled-by=least-latency
```

### Docker

To build the container image for L3, run

```sh
$ docker build -t oliviermichaelis/l3:latest .
```

## Installation

```sh
# To install the CustomResourceDefinitions of L3
make install

# To deploy L3 to Kubernetes
make deploy
```


