# How to build
```
VERSION=2.13.11
docker run -it -v "$PWD:/app" -w /app krakend/builder:${VERSION}-linux-generic go build -buildmode=plugin -buildvcs=false -o krakend-body-modifier-request.${VERSION}.so
```

