This Docker image can be built and used to run `fpm` and `reprepro` for testing on your development machine.

From the workspace directory `rackspace-monitoring-poller`, run the following to build the image

```bash
docker build -t poller-builder contrib/docker-builder
docker build -t poller-builder-centos -f contrib/docker-builder/Dockerfile.centos7 contrib/docker-builder
```

With that you can use the image to run `make`, etc using

```bash
docker run -it --rm -v $PWD:/home poller-builder
```

or for RPM/CentOS builds:

```bash
docker run -it --rm -v $PWD:/home poller-builder-centos
```

For example, to build the debian packages and populate the apt repo area without signing, run

```bash
make DONT_SIGN=1 reprepro-debs
```